package godot

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/vilppuvuorinen/slopfood-godot-mcp/internal/daemonpath"
)

// Recycles the engine after a fixed number of operations so accumulated state cannot grow without bound.
const defaultOperationCap = 100

// Bounds how long Close waits for a graceful quit before killing a wedged bridge.
const closeQuitTimeout = 5 * time.Second

// After the graceful quit window, a session whose operation will not return is
// force-killed so the daemon can still exit to be restarted.
const closeKillGrace = 2 * time.Second

// Contains no queue of its own: requests are serialized by the caller, so a second in-flight request
// would make the next frame unattributable.
type Session struct {
	mu         sync.Mutex
	executable string
	script     string

	transport Transport

	operationCap int

	proc   *sessionProc
	nextID int
	closed bool

	// procRef mirrors proc so Close can kill the engine without taking mu, which
	// a stuck operation may hold. closeTimeout is the production default, a
	// field so tests can shrink it.
	procRef      atomic.Pointer[sessionProc]
	closeTimeout time.Duration
}

// Its context is session-owned: the caller cancels a job's context as soon as the job returns, so a
// request-scoped context would kill the engine after every call.
type sessionProc struct {
	cmd    *exec.Cmd
	stdout io.ReadCloser
	stderr io.ReadCloser
	cancel context.CancelFunc

	// The single buffered reader over stdout, shared with the readiness read so buffered diagnostics drain here.
	stdoutReader *bufio.Reader

	// Requests and responses travel here; the stdout/stderr pipes are diagnostics only.
	socket     *engineSocket
	socketPath string

	// Closed once all readers stop, after them, so a frame delivered just before a reader stopped is not
	// masked by a sibling pipe closing first.
	done     chan struct{}
	doneOnce sync.Once
	quit     chan struct{}
	wg       sync.WaitGroup

	frames chan *responseFrame

	errOnce sync.Once
	errMu   sync.Mutex
	err     error

	stopOnce sync.Once

	project     string
	ops         int
	fingerprint projectFingerprint
}

// ID is a pointer because the startup readiness line has no id; Godot emits ids as JSON numbers.
type responseFrame struct {
	ID     *float64 `json:"id"`
	OK     bool     `json:"ok"`
	Stdout string   `json:"stdout"`
	Stderr string   `json:"stderr"`
}

// A missing file is the zero stamp, so both appearance and disappearance change the stamp.
type fileStamp struct {
	size int64
	mod  int64
}

// Engine-global file state cached at spawn; a change means the engine's loaded view is stale.
type projectFingerprint struct {
	projectGodot fileStamp
	classCache   fileStamp
}

func NewSession(executable, script string) *Session {
	return NewSessionTransport(executable, script, TransportAuto)
}

func NewSessionTransport(executable, script string, transport Transport) *Session {
	return &Session{
		executable:   executable,
		script:       script,
		transport:    transport,
		operationCap: defaultOperationCap,
		closeTimeout: closeQuitTimeout + closeKillGrace,
	}
}

func (s *Session) Run(ctx context.Context, project, operation string, params map[string]any) (stdout, stderr string, exitCode int, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return "", "", 0, fmt.Errorf("engine session: %w", os.ErrClosed)
	}

	if s.restartNeeded(project) {
		s.stopLocked()
	}

	p, err := s.ensureStartedLocked(project)
	if err != nil {
		return "", "", 0, err
	}

	// A changed fingerprint is reconciled by a rescan in the warm editor, not a respawn.
	if s.consumeRescan(project) {
		if rescanErr := s.runRescanLocked(ctx, p); rescanErr != nil {
			return "", "", 0, rescanErr
		}
	}

	s.nextID++
	id := s.nextID

	// nil params must marshal to {} so the bridge's validation never sees null.
	if params == nil {
		params = map[string]any{}
	}
	payload := struct {
		ID        int            `json:"id"`
		Operation string         `json:"operation"`
		Params    map[string]any `json:"params"`
	}{ID: id, Operation: operation, Params: params}
	line, err := json.Marshal(payload)
	if err != nil {
		return "", "", 0, fmt.Errorf("engine session: encode request: %w", err)
	}
	// A failed write means the process died; respawn once and retry before erroring.
	if err = p.socket.writeRequest(line); err != nil {
		s.stopLocked()
		p, err = s.ensureStartedLocked(project)
		if err != nil {
			return "", "", 0, err
		}
		if err = p.socket.writeRequest(line); err != nil {
			s.stopLocked()
			return "", "", 0, err
		}
	}

	frame, err := s.recvFrameLocked(ctx, p, id)
	if err != nil {
		return "", "", 0, err
	}
	p.ops++
	// Restamp so the next call does not rescan for the same change.
	p.fingerprint = fingerprintProject(project)

	if !frame.OK {
		exitCode = 1
	}
	return frame.Stdout, frame.Stderr, exitCode, nil
}

// Close stops the engine, waiting up to closeTimeout for the graceful path. If
// an operation holds the session lock and will not return (the queue's stuck-job
// path), the engine is killed out of band so the daemon can still exit.
func (s *Session) Close() error {
	done := make(chan struct{})
	go func() {
		defer close(done)
		s.mu.Lock()
		defer s.mu.Unlock()
		if s.closed {
			return
		}
		s.closed = true
		p := s.proc
		s.proc = nil
		s.procRef.Store(nil)
		if p != nil {
			p.shutdown()
		}
	}()
	select {
	case <-done:
	case <-time.After(s.closeTimeout):
		s.kill()
	}
	return nil
}

// kill force-stops the engine without taking the session lock, so a stuck
// operation cannot keep the daemon alive. The process kill is idempotent and
// safe alongside a concurrent p.stop.
func (s *Session) kill() {
	p := s.procRef.Load()
	if p == nil {
		return
	}
	p.cancel()
	if p.socket != nil {
		_ = p.socket.Close()
	}
	if p.cmd.Process != nil {
		_ = p.cmd.Process.Kill()
	}
}

// On any failure the process is killed and cleared, so a wedged engine is never reused.
func (s *Session) recvFrameLocked(ctx context.Context, p *sessionProc, id int) (*responseFrame, error) {
	for {
		select {
		case f := <-p.frames:
			if got, ok := frameID(f); !ok || got != id {
				s.stopLocked()
				return nil, fmt.Errorf("engine session: response id %v, want %d", *f.ID, id)
			}
			return f, nil
		case <-p.done:
			// Drain a buffered frame before treating the death as fatal.
			select {
			case f := <-p.frames:
				if got, ok := frameID(f); !ok || got != id {
					s.stopLocked()
					return nil, fmt.Errorf("engine session: response id %v, want %d", *f.ID, id)
				}
				return f, nil
			default:
				readErr := p.readErr()
				s.stopLocked()
				return nil, fmt.Errorf("engine session: engine exited before response: %w", readErr)
			}
		case <-ctx.Done():
			s.stopLocked()
			return nil, fmt.Errorf("engine session: %w", ctx.Err())
		}
	}
}

// A dirty project-global view is not a restart; it is reconciled by a rescan.
func (s *Session) restartNeeded(project string) bool {
	p := s.proc
	if p == nil {
		return true
	}
	select {
	case <-p.done:
		return true
	default:
	}
	if p.project != project {
		return true
	}
	return p.ops >= s.operationCap
}

func (s *Session) consumeRescan(project string) bool {
	if s.proc == nil {
		return false
	}
	fingerprint := fingerprintProject(project)
	if s.proc.fingerprint == fingerprint {
		return false
	}
	s.proc.fingerprint = fingerprint
	return true
}

func (s *Session) runRescanLocked(ctx context.Context, p *sessionProc) error {
	s.nextID++
	id := s.nextID
	payload := struct {
		ID        int            `json:"id"`
		Operation string         `json:"operation"`
		Params    map[string]any `json:"params"`
	}{ID: id, Operation: "reimport_project", Params: map[string]any{}}
	line, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("engine session: encode rescan request: %w", err)
	}
	if err = p.socket.writeRequest(line); err != nil {
		s.stopLocked()
		return fmt.Errorf("engine session: write rescan request: %w", err)
	}
	frame, err := s.recvFrameLocked(ctx, p, id)
	if err != nil {
		return err
	}
	p.ops++
	if !frame.OK {
		return fmt.Errorf("engine session: rescan failed: %s", strings.TrimSpace(frame.Stderr))
	}
	return nil
}

func (s *Session) ensureStartedLocked(project string) (*sessionProc, error) {
	if s.proc != nil {
		return s.proc, nil
	}
	p, err := startSessionProc(s.executable, s.script, project, s.transport)
	if err != nil {
		return nil, err
	}
	s.proc = p
	s.procRef.Store(p)
	return p, nil
}

func (s *Session) stopLocked() {
	p := s.proc
	if p == nil {
		return
	}
	s.proc = nil
	s.procRef.Store(nil)
	p.stop()
}

func engineLaunch(project string, transport Transport) (args []string, socketPath string, err error) {
	switch ResolveTransport(transport) {
	case TransportTCP:
		return []string{"--gd-tcp", "127.0.0.1:0"}, "", nil
	case TransportUDS, TransportAuto:
		socketPath, err = daemonpath.EngineSocketPath(project)
		if err != nil {
			return nil, "", fmt.Errorf("engine session: derive socket path: %w", err)
		}
		if err = prepareSocketDir(socketPath); err != nil {
			return nil, "", err
		}
		if err = removeStaleSocket(socketPath); err != nil {
			return nil, "", err
		}
		return []string{"--gd-socket", socketPath}, socketPath, nil
	}
	return nil, "", fmt.Errorf("engine session: unsupported transport %q", transport)
}

func startSessionProc(executable, script, project string, transport Transport) (*sessionProc, error) {
	ctx, cancel := context.WithCancel(context.Background())
	launchArgs, socketPath, err := engineLaunch(project, transport)
	if err != nil {
		cancel()
		return nil, err
	}
	// --editor makes EditorInterface and EditorFileSystem live; user args after -- reach OS.get_cmdline_user_args.
	args := append([]string{"--headless", "--editor", "--path", project, "--script", script, "--"}, launchArgs...)

	cmd := newCommand(ctx, executable, args...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		cancel()
		return nil, fmt.Errorf("engine session: stdout pipe: %w", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		cancel()
		return nil, fmt.Errorf("engine session: stderr pipe: %w", err)
	}
	if err = cmd.Start(); err != nil {
		cancel()
		return nil, fmt.Errorf("engine session: start engine: %w", err)
	}

	p := &sessionProc{
		cmd:          cmd,
		stdout:       stdout,
		stderr:       stderr,
		cancel:       cancel,
		stdoutReader: bufio.NewReader(stdout),
		done:         make(chan struct{}),
		quit:         make(chan struct{}),
		frames:       make(chan *responseFrame, 1),
		project:      project,
		fingerprint:  fingerprintProject(project),
	}

	info, err := readReadiness(p.stdoutReader, cancel)
	if err != nil {
		cancel()
		_ = cmd.Wait()
		return nil, err
	}
	socket, dialPath, err := dialReady(ctx, info, socketPath)
	if err != nil {
		cancel()
		_ = cmd.Wait()
		return nil, err
	}
	p.socket = socket
	p.socketPath = dialPath
	p.wg.Add(3)
	go p.readStdout()
	go p.readStderr()
	go p.readSocketFrames()

	go func() {
		p.wg.Wait()
		p.doneOnce.Do(func() { close(p.done) })
	}()
	return p, nil
}

func dialReady(ctx context.Context, info *readyInfo, socketPath string) (*engineSocket, string, error) {
	if info.Transport == "tcp" {
		address := net.JoinHostPort(info.Host, strconv.Itoa(info.Port))
		socket, err := dialEngineSocketNetwork(ctx, "tcp", address, info.Token)
		if err != nil {
			return nil, "", err
		}
		return socket, "", nil
	}
	dialPath := info.Path
	if dialPath == "" {
		dialPath = socketPath
	}
	socket, err := dialEngineSocket(ctx, dialPath, info.Token)
	if err != nil {
		return nil, "", err
	}
	if err := secureSocket(dialPath); err != nil {
		_ = socket.Close()
		return nil, "", err
	}
	return socket, dialPath, nil
}

// Closes quit before killing so a reader blocked on a full channel unblocks; only stop calls cmd.Wait, after all readers return.
func (p *sessionProc) stop() {
	p.stopOnce.Do(func() {
		close(p.quit)
		p.cancel()
		if p.socket != nil {
			_ = p.socket.Close()
		}
		p.wg.Wait()
		_ = p.cmd.Wait()
		if p.socketPath != "" {
			_ = os.Remove(p.socketPath)
		}
	})
}

func (p *sessionProc) shutdown() {
	if p.socket != nil {
		_ = p.socket.writeRequest([]byte(`{"id":0,"operation":"quit","params":{}}`))
	}
	timer := time.NewTimer(closeQuitTimeout)
	defer timer.Stop()
	select {
	case <-p.done:
	case <-timer.C:
	}
	p.stop()
}

// Editor exit prints fixed ObjectDB/RID leak ERROR lines here; they are diagnostics, never a result.
func (p *sessionProc) readStdout() {
	defer p.wg.Done()
	for {
		_, err := p.stdoutReader.ReadString('\n')
		if err != nil {
			p.noteReadErr(err)
			return
		}
	}
}

func (p *sessionProc) readSocketFrames() {
	defer p.wg.Done()
	for {
		f, err := p.socket.readFrame()
		if err != nil {
			p.noteReadErr(err)
			return
		}
		if !p.sendFrame(f) {
			return
		}
	}
}

// Process stderr is diagnostics only: the frame carries the handler's stderr, and editor exit leak warnings are expected.
func (p *sessionProc) readStderr() {
	defer p.wg.Done()
	r := bufio.NewReader(p.stderr)
	for {
		line, err := r.ReadString('\n')
		if text := strings.TrimRight(line, "\r\n"); text != "" {
			log.Printf("engine stderr: %s", text)
		}
		if err != nil {
			p.noteReadErr(err)
			return
		}
	}
}

func (p *sessionProc) sendFrame(f *responseFrame) bool {
	select {
	case p.frames <- f:
		return true
	case <-p.quit:
		return false
	}
}

// Records the first reader failure; done closes separately so a just-made delivery is not preempted.
func (p *sessionProc) noteReadErr(err error) {
	p.errOnce.Do(func() {
		p.errMu.Lock()
		p.err = err
		p.errMu.Unlock()
	})
}

func (p *sessionProc) readErr() error {
	p.errMu.Lock()
	defer p.errMu.Unlock()
	if p.err == nil {
		return io.EOF
	}
	return p.err
}

func parseFrame(line string) *responseFrame {
	trimmed := strings.TrimSpace(line)
	if !strings.HasPrefix(trimmed, "{") {
		return nil
	}
	var f responseFrame
	if err := json.Unmarshal([]byte(trimmed), &f); err != nil {
		return nil
	}
	if f.ID == nil {
		return nil
	}
	return &f
}

// frameID converts a frame's numeric id to an int, rejecting a non-integral or
// non-finite value; parseFrame guarantees the id is non-nil.
func frameID(f *responseFrame) (int, bool) {
	raw := *f.ID
	if math.IsNaN(raw) || math.IsInf(raw, 0) || raw != math.Trunc(raw) {
		return 0, false
	}
	return int(raw), true
}

func fingerprintProject(project string) projectFingerprint {
	return projectFingerprint{
		projectGodot: stampFile(filepath.Join(project, "project.godot")),
		classCache:   stampFile(filepath.Join(project, ".godot", "global_script_class_cache.cfg")),
	}
}

func stampFile(path string) fileStamp {
	info, err := os.Stat(path)
	if err != nil {
		return fileStamp{}
	}
	return fileStamp{size: info.Size(), mod: info.ModTime().UnixNano()}
}
