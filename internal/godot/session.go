package godot

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
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
	"github.com/vilppuvuorinen/slopfood-godot-mcp/internal/fsutil"
)

// Recycles the engine after a fixed number of operations so accumulated state cannot grow without bound.
const defaultOperationCap = 100

// Bounds the stderr lines retained for startup diagnostics so a chatty engine
// cannot grow the session without bound.
const startupStderrMaxLines = 200

// How long a fresh start waits for the engine to report a project.godot parse
// error before concluding the project loaded. The engine writes the diagnostic
// during startup, well before it is ready, so this is only a scheduling margin.
const startupParseGrace = 250 * time.Millisecond

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

	transport EngineTransport

	operationCap int

	proc   *sessionProc
	nextID int
	closed bool

	// The project.godot stamp last attempted, so a fresh start only probes for a
	// parse error when the file changed, and a broken project is remembered
	// until the file changes again.
	lastProjectStamp fileStamp
	brokenErr        error
	brokenProject    string
	brokenStamp      fileStamp

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

	// Startup stderr is retained so a project.godot parse error can be reported
	// back through the session error instead of being logged and dropped.
	stderrMu    sync.Mutex
	stderrLines []string

	stopOnce sync.Once

	project     string
	ops         int
	fingerprint projectFingerprint
}

// ID is a pointer because the startup readiness line has no id; Godot emits ids as JSON numbers.
type responseFrame struct {
	ID      *float64 `json:"id"`
	OK      bool     `json:"ok"`
	Stdout  string   `json:"stdout"`
	Stderr  string   `json:"stderr"`
	Payload string   `json:"payload"`
}

// RunResult is a bridge response: the handler's raw streams, the structured
// payload when the operation produced one, and the failure exit code.
type RunResult struct {
	Stdout   string
	Stderr   string
	Payload  string
	ExitCode int
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
	return NewSessionTransport(executable, script, EngineTransportAuto)
}

func NewSessionTransport(executable, script string, transport EngineTransport) *Session {
	return &Session{
		executable:   executable,
		script:       script,
		transport:    transport,
		operationCap: defaultOperationCap,
		closeTimeout: closeQuitTimeout + closeKillGrace,
	}
}

func (s *Session) Run(ctx context.Context, project, operation string, params map[string]any) (RunResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return RunResult{}, fmt.Errorf("engine session: %w", os.ErrClosed)
	}

	stamp := stampFile(filepath.Join(project, "project.godot"))

	// A project.godot that failed to load at this stamp keeps failing for every
	// session call until the file changes, so the model can investigate with the
	// validate tool instead of retrying blindly.
	if s.brokenErr != nil {
		if s.brokenProject == project && s.brokenStamp == stamp {
			return RunResult{}, s.brokenErr
		}
		s.brokenErr = nil
	}

	if s.restartNeeded(project) {
		s.stopLocked()
	}

	// Only probe for a startup parse error when project.godot changed since the
	// last start; an operation-cap recycle reuses the same valid file.
	checkStartup := stamp != s.lastProjectStamp
	p, err := s.startLocked(project, stamp, checkStartup)
	if err != nil {
		return RunResult{}, err
	}

	// A changed class cache is reconciled by a rescan in the warm editor; a
	// changed project.godot is handled by a restart in restartNeeded.
	if s.consumeRescan(project) {
		if rescanErr := s.runRescanLocked(ctx, p); rescanErr != nil {
			return RunResult{}, rescanErr
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
		return RunResult{}, fmt.Errorf("engine session: encode request: %w", err)
	}
	// A failed write means the process died; respawn once and retry before erroring.
	if err = p.socket.writeRequest(line); err != nil {
		s.stopLocked()
		p, err = s.startLocked(project, stamp, false)
		if err != nil {
			return RunResult{}, err
		}
		if err = p.socket.writeRequest(line); err != nil {
			s.stopLocked()
			return RunResult{}, err
		}
	}

	frame, err := s.recvFrameLocked(ctx, p, id)
	if err != nil {
		return RunResult{}, err
	}
	p.ops++
	// Restamp so the next call does not rescan for the same change.
	p.fingerprint = fingerprintProject(project)

	result := RunResult{
		Stdout:  frame.Stdout,
		Stderr:  frame.Stderr,
		Payload: frame.Payload,
	}
	if !frame.OK {
		result.ExitCode = 1
	}
	return result, nil
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

// A changed project root or project.godot needs a respawn: the editor binds the
// project settings at startup and a rescan cannot apply a broken file. A dirty
// class cache is reconciled by a rescan instead.
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
	if p.fingerprint.projectGodot != stampFile(filepath.Join(project, "project.godot")) {
		return true
	}
	return p.ops >= s.operationCap
}

// consumeRescan reports whether the class cache changed since the last scan.
// project.godot is deliberately excluded: a change there restarts the session
// (see restartNeeded) because a rescan cannot recover from a parse error.
func (s *Session) consumeRescan(project string) bool {
	if s.proc == nil {
		return false
	}
	fingerprint := fingerprintProject(project)
	if s.proc.fingerprint.classCache == fingerprint.classCache {
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
	}{ID: id, Operation: "rescan_project", Params: map[string]any{}}
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

// startLocked starts the engine if needed and records a broken project.godot so
// later calls fail fast with the same parse diagnostic until the file changes.
func (s *Session) startLocked(project string, stamp fileStamp, checkStartup bool) (*sessionProc, error) {
	if s.proc != nil {
		return s.proc, nil
	}
	p, err := startSessionProc(s.executable, s.script, project, s.transport, checkStartup)
	if err != nil {
		s.lastProjectStamp = stamp
		var broken *projectBrokenError
		if errors.As(err, &broken) {
			s.brokenProject = project
			s.brokenStamp = stamp
			s.brokenErr = err
		}
		return nil, err
	}
	s.lastProjectStamp = stamp
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

func engineLaunch(project string, transport EngineTransport) (args []string, socketPath string, err error) {
	switch ResolveTransport(transport) {
	case EngineTransportTCP:
		return []string{"--gd-tcp", "127.0.0.1:0"}, "", nil
	case EngineTransportUDS, EngineTransportAuto:
		socketPath, err = daemonpath.EngineSocketPath(project)
		if err != nil {
			return nil, "", fmt.Errorf("engine session: derive socket path: %w", err)
		}
		if err = fsutil.EnsurePrivateDir(filepath.Dir(socketPath)); err != nil {
			return nil, "", fmt.Errorf("engine session: prepare socket directory: %w", err)
		}
		if err = fsutil.RemoveIfExists(socketPath); err != nil {
			return nil, "", fmt.Errorf("engine session: remove stale socket: %w", err)
		}
		return []string{"--gd-socket", socketPath}, socketPath, nil
	}
	return nil, "", fmt.Errorf("engine session: unsupported transport %q", transport)
}

func startSessionProc(executable, script, project string, transport EngineTransport, checkStartup bool) (*sessionProc, error) {
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

	// Drain stderr from the start so a startup parse error is captured even
	// though the process is only declared broken after it is ready.
	p.wg.Add(1)
	go p.readStderr()

	info, err := readReadiness(p.stdoutReader, cancel)
	if err != nil {
		p.stop()
		return nil, startupError(err, p)
	}
	socket, dialPath, err := dialReady(ctx, info, socketPath)
	if err != nil {
		p.stop()
		return nil, startupError(err, p)
	}
	p.socket = socket
	p.socketPath = dialPath

	// A project.godot that fails to parse still lets the editor boot with
	// defaults; reject that session so every call reports the parse error.
	if checkStartup {
		if diagnostics := awaitProjectParseErrors(p); len(diagnostics) > 0 {
			p.stop()
			return nil, &projectBrokenError{diagnostics: diagnostics}
		}
	}

	p.wg.Add(2)
	go p.readStdout()
	go p.readSocketFrames()

	go func() {
		p.wg.Wait()
		p.doneOnce.Do(func() { close(p.done) })
	}()
	return p, nil
}

// startupError keeps the engine's startup stderr attached to the failure so a
// project.godot parse error is visible to the caller.
func startupError(err error, p *sessionProc) error {
	if diagnostics := ProjectParseErrors(p.stderrText()); len(diagnostics) > 0 {
		return &projectBrokenError{diagnostics: diagnostics}
	}
	if text := strings.TrimSpace(p.stderrText()); text != "" {
		return fmt.Errorf("%w: %s", err, text)
	}
	return err
}

// projectBrokenError reports that project.godot failed to load. It is returned
// for every session operation until the file changes.
type projectBrokenError struct {
	diagnostics []string
}

func (e *projectBrokenError) Error() string {
	return "engine session: project.godot failed to load: " + strings.Join(e.diagnostics, "; ")
}

// awaitProjectParseErrors polls the captured startup stderr for a parse
// diagnostic. The engine emits it before readiness, so the first poll usually
// finds it; the grace covers the reader goroutine's scheduling.
func awaitProjectParseErrors(p *sessionProc) []string {
	deadline := time.Now().Add(startupParseGrace)
	for {
		if diagnostics := ProjectParseErrors(p.stderrText()); len(diagnostics) > 0 {
			return diagnostics
		}
		if !time.Now().Before(deadline) {
			return nil
		}
		time.Sleep(10 * time.Millisecond)
	}
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

// Process stderr is diagnostics only: the frame carries the handler's stderr, and editor exit leak warnings are expected. It is still retained so a startup project.godot parse error can be surfaced through the session error.
func (p *sessionProc) readStderr() {
	defer p.wg.Done()
	r := bufio.NewReader(p.stderr)
	for {
		line, err := r.ReadString('\n')
		if text := strings.TrimRight(line, "\r\n"); text != "" {
			log.Printf("engine stderr: %s", text)
			p.appendStderr(text)
		}
		if err != nil {
			p.noteReadErr(err)
			return
		}
	}
}

func (p *sessionProc) appendStderr(line string) {
	p.stderrMu.Lock()
	defer p.stderrMu.Unlock()
	if len(p.stderrLines) >= startupStderrMaxLines {
		return
	}
	p.stderrLines = append(p.stderrLines, line)
}

func (p *sessionProc) stderrText() string {
	p.stderrMu.Lock()
	defer p.stderrMu.Unlock()
	return strings.Join(p.stderrLines, "\n")
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
