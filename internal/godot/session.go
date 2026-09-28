package godot

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/vilppuvuorinen/slopfood-godot-mcp/internal/daemonpath"
)

// defaultOperationCap bounds how many operations one engine process serves. A
// long-lived process accumulates state (loaded resources, engine globals), so
// recycling it after a fixed number of calls keeps that state from growing
// without bound; the cap is a field so tests can lower it.
const defaultOperationCap = 100

// closeQuitTimeout bounds how long Close waits for the bridge to exit after it
// is asked to quit over the socket. A bridge that ignores the request is then
// killed; the timeout is only a floor for a wedged process.
const closeQuitTimeout = 5 * time.Second

// Session owns at most one long-lived engine process and multiplexes one
// request at a time over its Unix socket transport. The mcp.Queue is the single
// serialization point for callers, so Session adds only a mutex to keep its own
// lifecycle consistent; it must not add another queue, because a second in-flight
// request would make it impossible to attribute the next frame to a request.
type Session struct {
	mu         sync.Mutex
	executable string
	script     string

	// operationCap restarts the engine after this many operations. It is a field
	// rather than a constant so same-package tests can set it low.
	operationCap int

	proc   *sessionProc
	nextID int
	dirty  bool
	closed bool
}

// sessionProc is one engine process plus the goroutines that read its pipes and
// socket. It deliberately owns a context independent of any request: the queue
// cancels a job's context as soon as the job returns, so a request-scoped
// command context would kill the engine after every call.
type sessionProc struct {
	cmd    *exec.Cmd
	stdout io.ReadCloser
	stderr io.ReadCloser
	cancel context.CancelFunc

	// stdoutReader is the single buffered reader over stdout. The readiness line
	// is read from it before the socket frame reader starts, so any buffered
	// diagnostics must be drained through the same reader.
	stdoutReader *bufio.Reader

	// socket is the Unix socket the bridge bound. Requests and responses travel
	// over it; the stdout/stderr pipes are diagnostics only.
	socket     *engineSocket
	socketPath string

	// done is closed once all readers have stopped, marking the process dead.
	// It closes after the readers, not on the first EOF, so a frame a reader
	// delivered just before it stopped is never masked by a sibling pipe
	// closing first. quit is closed by stop before it kills the process, so a
	// reader parked on a full delivery channel can leave and not block wg.Wait.
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

// responseFrame is one bridge response. ID is a pointer because the startup
// readiness line is a JSON object with no id, and Godot emits ids as JSON
// numbers that decode to float64.
type responseFrame struct {
	ID     *float64 `json:"id"`
	OK     bool     `json:"ok"`
	Stdout string   `json:"stdout"`
	Stderr string   `json:"stderr"`
}

// fileStamp identifies one project-global file by size and modification time. A
// missing file is the zero stamp, so appearance and disappearance both change
// the stamp and trigger a restart.
type fileStamp struct {
	size int64
	mod  int64
}

// projectFingerprint is the engine-global file state a running process cached at
// spawn. A change means the engine's loaded view is stale, so the next call
// restarts it rather than serving from that stale view.
type projectFingerprint struct {
	projectGodot fileStamp
	classCache   fileStamp
}

// NewSession returns a session that spawns executable's socket driver for the
// bridge at script. The engine is started lazily by the first Run.
func NewSession(executable, script string) *Session {
	return &Session{
		executable:   executable,
		script:       script,
		operationCap: defaultOperationCap,
	}
}

// Run executes one operation against the session's engine, starting or
// restarting it as needed. The id and frame are attributed under the lock, so
// the socket reader can assume only one response is ever in flight.
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

	s.nextID++
	id := s.nextID

	// nil params must marshal to {} so the bridge's request validation never
	// sees null and rejects an otherwise valid request.
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
	// The process may have died between the death check above and the write; a
	// failed write is the same death, so respawn once and retry before treating
	// it as an error.
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

	if !frame.OK {
		exitCode = 1
	}
	return frame.Stdout, frame.Stderr, exitCode, nil
}

// Invalidate marks the running engine's project-global view dirty so the next
// Run respawns it. It never touches the process itself, so it is cheap and safe
// to call after Close.
func (s *Session) Invalidate() {
	s.mu.Lock()
	s.dirty = true
	s.mu.Unlock()
}

// Close asks the engine to quit, then kills it and waits for its readers. It is
// idempotent; a Run after Close fails, and Invalidate remains safe.
func (s *Session) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil
	}
	s.closed = true
	p := s.proc
	s.proc = nil
	if p != nil {
		p.shutdown()
	}
	return nil
}

// recvFrameLocked waits for the response frame matching id. On any failure it
// kills and clears the process, so a wedged engine is never reused.
func (s *Session) recvFrameLocked(ctx context.Context, p *sessionProc, id int) (*responseFrame, error) {
	for {
		select {
		case f := <-p.frames:
			if int(*f.ID) != id {
				s.stopLocked()
				return nil, fmt.Errorf("engine session: response id %v, want %d", *f.ID, id)
			}
			return f, nil
		case <-p.done:
			// A frame already delivered is not lost when the reader then hits
			// EOF: drain the buffered frame before treating the death as fatal.
			select {
			case f := <-p.frames:
				if int(*f.ID) != id {
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

// restartNeeded reports whether the current process cannot serve project. Any
// doubt (death, dirty state, changed project or project-global files, or too
// many served operations) is resolved by a clean respawn rather than by trying
// to salvage the process.
func (s *Session) restartNeeded(project string) bool {
	if s.dirty {
		return true
	}
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
	if p.ops >= s.operationCap {
		return true
	}
	return p.fingerprint != fingerprintProject(project)
}

// ensureStartedLocked spawns the bridge for project when no process is running.
func (s *Session) ensureStartedLocked(project string) (*sessionProc, error) {
	if s.proc != nil {
		return s.proc, nil
	}
	p, err := startSessionProc(s.executable, s.script, project)
	if err != nil {
		return nil, err
	}
	s.proc = p
	s.dirty = false
	return p, nil
}

// stopLocked tears down the running process and clears it. It is safe to call
// with no process.
func (s *Session) stopLocked() {
	p := s.proc
	if p == nil {
		return
	}
	s.proc = nil
	p.stop()
}

// startSessionProc starts the bridge in socket mode and its reader goroutines.
// The context is session-owned (see sessionProc) and exists only so cancel can
// kill the child. It creates the runtime directory, removes a stale socket,
// passes the derived path after the -- separator so it reaches
// OS.get_cmdline_user_args, reads the readiness object from stdout, dials the
// socket, and narrows it to 0600.
func startSessionProc(executable, script, project string) (*sessionProc, error) {
	ctx, cancel := context.WithCancel(context.Background())
	socketPath, err := daemonpath.EngineSocketPath(project)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("engine session: derive socket path: %w", err)
	}
	if err = prepareSocketDir(socketPath); err != nil {
		cancel()
		return nil, err
	}
	if err = removeStaleSocket(socketPath); err != nil {
		cancel()
		return nil, err
	}
	args := []string{"--headless", "--path", project, "--script", script, "--", "--gd-socket", socketPath}

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
	dialPath := info.Path
	if dialPath == "" {
		dialPath = socketPath
	}
	socket, err := dialEngineSocket(ctx, dialPath, info.Token)
	if err != nil {
		cancel()
		_ = cmd.Wait()
		return nil, err
	}
	if err := secureSocket(dialPath); err != nil {
		_ = socket.Close()
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

// stop closes quit before killing the process, so a reader blocked sending on a
// full channel unblocks instead of holding wg.Wait forever. Only stop calls
// cmd.Wait, and only after every reader has returned, so Wait cannot close a
// pipe under an active reader. The socket is closed to unblock its reader and
// the bound path is unlinked so a respawn starts clean.
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

// shutdown asks the bridge to exit through the quit operation, waits a bounded
// time for the process to exit, then falls back to the hard stop. The quit frame
// is best-effort: a dead process or a failed write just falls through to stop.
func (p *sessionProc) shutdown() {
	_ = p.socket.writeRequest([]byte(`{"id":0,"operation":"quit","params":{}}`))
	timer := time.NewTimer(closeQuitTimeout)
	defer timer.Stop()
	select {
	case <-p.done:
	case <-timer.C:
	}
	p.stop()
}

// readStdout drains the engine process's stdout. The readiness line was already
// consumed and response frames travel over the socket, so after startup stdout
// is diagnostics only.
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

// readSocketFrames delivers response frames read from the socket, reusing the
// frame channel and its id-desync and buffered-drain supervision.
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

// readStderr drains the engine process's stderr. It is diagnostics only now: the
// response frame carries the handler's stderr, so process stderr is logged and
// never merged into a result or used as a protocol delimiter.
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

// noteReadErr records the first reader failure. The done channel is closed
// separately, once all readers stop, so a delivery made just before one reader
// stops is never preempted by another pipe closing.
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

// parseFrame returns the frame when line is a JSON object with an id, and nil
// for the banner, diagnostics, or any other non-frame output.
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

// fingerprintProject stamps the engine-global project files. Missing files are
// the zero stamp, so both appearance and disappearance compare unequal.
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
