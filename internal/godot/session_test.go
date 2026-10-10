//go:build !windows

package godot

import (
	"context"
	"encoding/json"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Points XDG_RUNTIME_DIR at a short directory so the derived socket path stays under the sun_path limit.
func shortRuntimeDir(t *testing.T) {
	t.Helper()
	dir, err := os.MkdirTemp("", "sess-rt-")
	if err != nil {
		t.Fatalf("create runtime dir: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	t.Setenv("XDG_RUNTIME_DIR", dir)
}

type socketSessionFake struct {
	bin    string
	starts string
	args   string
	env    string
	quit   string
	ops    string
}

const defaultSocketResponse = `
        send(conn, req["id"], True, "out-" + str(req["id"]), "")`

// response is a Python snippet run per request with req, conn, and send() in scope.
func newSocketSessionFake(t *testing.T, response string) *socketSessionFake {
	t.Helper()
	return newSocketSessionFakeStderr(t, response, "")
}

// newSocketSessionFakeStderr additionally runs a shell snippet before the
// bridge starts, so tests can simulate engine startup diagnostics.
func newSocketSessionFakeStderr(t *testing.T, response, preStderr string) *socketSessionFake {
	t.Helper()
	dir := t.TempDir()
	f := &socketSessionFake{
		bin:    filepath.Join(dir, "godot"),
		starts: filepath.Join(dir, "starts"),
		args:   filepath.Join(dir, "args"),
		env:    filepath.Join(dir, "env"),
		quit:   filepath.Join(dir, "quit"),
		ops:    filepath.Join(dir, "ops"),
	}
	body := `#!/bin/sh
printf '%s\n' "$*" >> '` + f.args + `'
echo start >> '` + f.starts + `'
printf '%s' "$GDCOVER_FILE" > '` + f.env + `'
echo 'Godot Engine v4.7 fake'
` + preStderr + `
exec python3 - "$@" <<'PY'
import json, os, socket, sys
args = sys.argv[1:]
if "--gd-tcp" in args:
    host, port = args[args.index("--gd-tcp") + 1].rsplit(":", 1)
    srv = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
    srv.setsockopt(socket.SOL_SOCKET, socket.SO_REUSEADDR, 1)
    srv.bind((host, int(port)))
    srv.listen(1)
    bound_host, bound_port = srv.getsockname()
    print(json.dumps({"ready": True, "transport": "tcp", "host": bound_host, "port": bound_port, "token": "tok"}), flush=True)
else:
    path = args[args.index("--gd-socket") + 1]
    if os.path.exists(path):
        os.remove(path)
    srv = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM)
    srv.bind(path)
    srv.listen(1)
    os.chmod(path, 0o755)
    print(json.dumps({"ready": True, "transport": "uds", "path": path, "token": "tok"}), flush=True)
print("engine noise on stdout", flush=True)

def send(conn, i, ok, out, err):
    conn.sendall(json.dumps({"id": i, "ok": ok, "stdout": out, "stderr": err}).encode() + b"\n")

conn, _ = srv.accept()
buf = b""
while True:
    data = conn.recv(4096)
    if not data:
        break
    buf += data
    while b"\n" in buf:
        line, buf = buf.split(b"\n", 1)
        req = json.loads(line)
        open('` + f.ops + `', "a").write(line.decode() + "\n")
        if req.get("operation") == "quit":
            send(conn, req["id"], True, "", "")
            open('` + f.quit + `', "w").write("quit")
            sys.exit(0)
` + response + `
PY
`
	if err := os.WriteFile(f.bin, []byte(body), 0o755); err != nil { //nolint:gosec // test helper needs an executable script.
		t.Fatalf("write fake: %v", err)
	}
	return f
}

func (f *socketSessionFake) startCount(t *testing.T) int {
	t.Helper()
	data, err := os.ReadFile(f.starts)
	if err != nil {
		if os.IsNotExist(err) {
			return 0
		}
		t.Fatalf("read starts: %v", err)
	}
	return len(strings.Fields(string(data)))
}

func (f *socketSessionFake) argLines(t *testing.T) []string {
	t.Helper()
	data, err := os.ReadFile(f.args)
	if err != nil {
		t.Fatalf("read args: %v", err)
	}
	return strings.Split(strings.TrimSpace(string(data)), "\n")
}

// envValue is exercised by the gdcover-tagged session test, so it reads as
// unused in a normal build.
func (f *socketSessionFake) envValue(t *testing.T) string { //nolint:unused // used by the gdcover-tagged session test.
	t.Helper()
	data, err := os.ReadFile(f.env)
	if err != nil {
		t.Fatalf("read env: %v", err)
	}
	return string(data)
}

func (f *socketSessionFake) recordedOps(t *testing.T) []map[string]any {
	t.Helper()
	data, err := os.ReadFile(f.ops)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		t.Fatalf("read ops: %v", err)
	}
	var out []map[string]any
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		if line == "" {
			continue
		}
		var req map[string]any
		if err := json.Unmarshal([]byte(line), &req); err != nil {
			t.Fatalf("decode request %q: %v", line, err)
		}
		out = append(out, req)
	}
	return out
}

func (f *socketSessionFake) quitRecorded(t *testing.T) bool {
	t.Helper()
	_, err := os.Stat(f.quit)
	if err == nil {
		return true
	}
	if os.IsNotExist(err) {
		return false
	}
	t.Fatalf("stat quit: %v", err)
	return false
}

func TestSessionRoundTripFraming(t *testing.T) {
	shortRuntimeDir(t)
	f := newSocketSessionFake(t, defaultSocketResponse)
	s := NewSession(f.bin, "/ops.gd")
	defer s.Close()
	project := t.TempDir()

	run, err := s.Run(context.Background(), project, "run_script",
		map[string]any{"scenePath": "scenes/main.tscn"})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if run.ExitCode != 0 || run.Stderr != "" || run.Stdout != "out-1" {
		t.Fatalf("first run: stdout=%q stderr=%q exit=%d", run.Stdout, run.Stderr, run.ExitCode)
	}

	run, err = s.Run(context.Background(), project, "read_scene",
		map[string]any{"scenePath": "scenes/main.tscn"})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if run.ExitCode != 0 || run.Stderr != "" || run.Stdout != "out-2" {
		t.Fatalf("second run: stdout=%q stderr=%q exit=%d", run.Stdout, run.Stderr, run.ExitCode)
	}

	if got := f.startCount(t); got != 1 {
		t.Fatalf("starts = %d, want 1 (session reused the process)", got)
	}

	lines := f.argLines(t)
	if len(lines) != 1 {
		t.Fatalf("spawn arg lines = %d, want 1", len(lines))
	}
	for _, want := range []string{"--headless", "--editor", "--path " + project, "--script /ops.gd", "-- --gd-socket "} {
		if !strings.Contains(lines[0], want) {
			t.Errorf("spawn args %q missing %q", lines[0], want)
		}
	}
	if strings.Contains(lines[0], "--gd-daemon") {
		t.Errorf("spawn args still carry --gd-daemon: %q", lines[0])
	}
}

func TestSessionForcedTCPFraming(t *testing.T) {
	shortRuntimeDir(t)
	f := newSocketSessionFake(t, defaultSocketResponse)
	s := NewSessionTransport(f.bin, "/ops.gd", TransportTCP)
	defer s.Close()
	project := t.TempDir()

	run, err := s.Run(context.Background(), project, "run_script",
		map[string]any{"scenePath": "scenes/main.tscn"})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if run.ExitCode != 0 || run.Stderr != "" || run.Stdout != "out-1" {
		t.Fatalf("run: stdout=%q stderr=%q exit=%d", run.Stdout, run.Stderr, run.ExitCode)
	}

	lines := f.argLines(t)
	if len(lines) != 1 {
		t.Fatalf("spawn arg lines = %d, want 1", len(lines))
	}
	if !strings.Contains(lines[0], "-- --gd-tcp 127.0.0.1:0") {
		t.Errorf("spawn args %q missing the --gd-tcp user arg", lines[0])
	}
	if strings.Contains(lines[0], "--gd-socket") {
		t.Errorf("forced-TCP spawn args still carry --gd-socket: %q", lines[0])
	}
}

func TestSessionFramesRecorded(t *testing.T) {
	shortRuntimeDir(t)
	reqs := filepath.Join(t.TempDir(), "reqs")
	response := `
        req.pop("token", None)
        open('` + reqs + `', "a").write(json.dumps(req, separators=(",", ":")) + "\n")
        send(conn, req["id"], True, "out-" + str(req["id"]), "")`
	f := newSocketSessionFake(t, response)
	s := NewSession(f.bin, "/ops.gd")
	defer s.Close()
	project := t.TempDir()

	if _, err := s.Run(context.Background(), project, "run_script", map[string]any{"scenePath": "a.tscn"}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if _, err := s.Run(context.Background(), project, "read_scene", map[string]any{"scenePath": "b.tscn"}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if _, err := s.Run(context.Background(), project, "read_node", nil); err != nil {
		t.Fatalf("Run: %v", err)
	}

	data, err := os.ReadFile(reqs)
	if err != nil {
		t.Fatalf("read reqs: %v", err)
	}
	got := strings.Split(strings.TrimSpace(string(data)), "\n")
	want := []string{
		`{"id":1,"operation":"run_script","params":{"scenePath":"a.tscn"}}`,
		`{"id":2,"operation":"read_scene","params":{"scenePath":"b.tscn"}}`,
		`{"id":3,"operation":"read_node","params":{}}`,
	}
	if len(got) != len(want) {
		t.Fatalf("frames = %d, want %d: %q", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("frame %d = %q, want %q", i+1, got[i], want[i])
		}
	}
}

func TestSessionStderrDiagnosticsOnly(t *testing.T) {
	shortRuntimeDir(t)
	response := `
        print("engine diagnostic " + str(req["id"]), file=sys.stderr, flush=True)
        send(conn, req["id"], True, "ok", "frame stderr line")`
	f := newSocketSessionFake(t, response)
	s := NewSession(f.bin, "/ops.gd")
	defer s.Close()

	run, err := s.Run(context.Background(), t.TempDir(), "op", nil)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if run.ExitCode != 0 {
		t.Fatalf("exitCode = %d", run.ExitCode)
	}
	if run.Stderr != "frame stderr line" {
		t.Errorf("stderr = %q, want only the frame's stderr", run.Stderr)
	}
	if strings.Contains(run.Stderr, "engine diagnostic") {
		t.Errorf("stderr merged process diagnostics: %q", run.Stderr)
	}
}

func TestSessionSkipsNonFrameStdout(t *testing.T) {
	shortRuntimeDir(t)
	f := newSocketSessionFake(t, defaultSocketResponse)
	s := NewSession(f.bin, "/ops.gd")
	defer s.Close()

	run, err := s.Run(context.Background(), t.TempDir(), "op", nil)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if run.Stdout != "out-1" {
		t.Errorf("stdout = %q, want the frame's stdout only", run.Stdout)
	}
}

func TestSessionRescanProjectDrivesScan(t *testing.T) {
	shortRuntimeDir(t)
	f := newSocketSessionFake(t, defaultSocketResponse)
	s := NewSession(f.bin, "/ops.gd")
	defer s.Close()
	project := t.TempDir()

	if _, err := s.Run(context.Background(), project, "rescan_project", nil); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got := f.startCount(t); got != 1 {
		t.Fatalf("starts = %d, want 1", got)
	}
	reqs := f.recordedOps(t)
	if len(reqs) != 1 {
		t.Fatalf("requests = %+v, want one", reqs)
	}
	if reqs[0]["operation"] != "rescan_project" {
		t.Errorf("operation = %v, want rescan_project", reqs[0]["operation"])
	}
}

func TestSessionOperationCapRestarts(t *testing.T) {
	shortRuntimeDir(t)
	f := newSocketSessionFake(t, defaultSocketResponse)
	s := NewSession(f.bin, "/ops.gd")
	s.operationCap = 2
	defer s.Close()
	project := t.TempDir()

	for i := 0; i < 2; i++ {
		if _, err := s.Run(context.Background(), project, "op", nil); err != nil {
			t.Fatalf("Run %d: %v", i, err)
		}
	}
	if got := f.startCount(t); got != 1 {
		t.Fatalf("starts = %d, want 1 after two operations", got)
	}
	if _, err := s.Run(context.Background(), project, "op", nil); err != nil {
		t.Fatalf("third Run: %v", err)
	}
	if got := f.startCount(t); got != 2 {
		t.Errorf("starts = %d, want 2 once the cap is reached", got)
	}
}

func TestSessionProjectChangeRestarts(t *testing.T) {
	shortRuntimeDir(t)
	f := newSocketSessionFake(t, defaultSocketResponse)
	s := NewSession(f.bin, "/ops.gd")
	defer s.Close()

	if _, err := s.Run(context.Background(), t.TempDir(), "op", nil); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if _, err := s.Run(context.Background(), t.TempDir(), "op", nil); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got := f.startCount(t); got != 2 {
		t.Errorf("starts = %d, want 2 for two projects", got)
	}
}

func TestSessionProjectGodotChangeRestarts(t *testing.T) {
	shortRuntimeDir(t)
	ops := filepath.Join(t.TempDir(), "ops")
	response := `
        open('` + ops + `', "a").write(req["operation"] + "\n")
        send(conn, req["id"], True, "out-" + str(req["id"]), "")`
	f := newSocketSessionFake(t, response)
	s := NewSession(f.bin, "/ops.gd")
	defer s.Close()
	project := t.TempDir()

	projectFile := filepath.Join(project, "project.godot")
	if err := os.WriteFile(projectFile, []byte("config_version=5\n"), 0o644); err != nil { //nolint:gosec // test fixture.
		t.Fatalf("write project.godot: %v", err)
	}
	if _, err := s.Run(context.Background(), project, "op", nil); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got := f.startCount(t); got != 1 {
		t.Fatalf("starts = %d, want 1", got)
	}

	// A different size changes the stamp even if the mtime lands in the same clock tick.
	if err := os.WriteFile(projectFile, []byte("config_version=5\n\n[application]\n"), 0o644); err != nil { //nolint:gosec // test fixture.
		t.Fatalf("rewrite project.godot: %v", err)
	}
	if _, err := s.Run(context.Background(), project, "op", nil); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got := f.startCount(t); got != 2 {
		t.Errorf("starts = %d, want 2: a project.godot change must respawn", got)
	}

	data, err := os.ReadFile(ops)
	if err != nil {
		t.Fatalf("read ops: %v", err)
	}
	got := strings.Fields(string(data))
	want := []string{"op", "op"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("operations = %v, want %v (no rescan for project.godot)", got, want)
	}
}

// A class cache change is still reconciled by a rescan in the warm editor.
func TestSessionClassCacheRescans(t *testing.T) {
	shortRuntimeDir(t)
	ops := filepath.Join(t.TempDir(), "ops")
	response := `
        open('` + ops + `', "a").write(req["operation"] + "\n")
        send(conn, req["id"], True, "out-" + str(req["id"]), "")`
	f := newSocketSessionFake(t, response)
	s := NewSession(f.bin, "/ops.gd")
	defer s.Close()
	project := t.TempDir()

	cacheDir := filepath.Join(project, ".godot")
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		t.Fatalf("mkdir .godot: %v", err)
	}
	cacheFile := filepath.Join(cacheDir, "global_script_class_cache.cfg")
	if err := os.WriteFile(cacheFile, []byte("config_version=5\n"), 0o644); err != nil { //nolint:gosec // test fixture.
		t.Fatalf("write class cache: %v", err)
	}
	if _, err := s.Run(context.Background(), project, "op", nil); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if err := os.WriteFile(cacheFile, []byte("config_version=5\n[class]\n"), 0o644); err != nil { //nolint:gosec // test fixture.
		t.Fatalf("rewrite class cache: %v", err)
	}
	if _, err := s.Run(context.Background(), project, "op", nil); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got := f.startCount(t); got != 1 {
		t.Errorf("starts = %d, want 1: a class cache change must rescan, not respawn", got)
	}

	data, err := os.ReadFile(ops)
	if err != nil {
		t.Fatalf("read ops: %v", err)
	}
	got := strings.Fields(string(data))
	want := []string{"op", "rescan_project", "op"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("operations = %v, want %v", got, want)
	}
}

// A broken project.godot leaves the session unusable: every call reports the
// parse error without respawning again until the file changes.
func TestSessionBrokenProjectGodotFails(t *testing.T) {
	shortRuntimeDir(t)
	const parseErr = `echo "ERROR: Error parsing '/proj/project.godot' at line 2" >&2`
	f := newSocketSessionFakeStderr(t, defaultSocketResponse, parseErr)
	s := NewSession(f.bin, "/ops.gd")
	defer s.Close()
	project := t.TempDir()

	projectFile := filepath.Join(project, "project.godot")
	if err := os.WriteFile(projectFile, []byte("config_version=5\n"), 0o644); err != nil { //nolint:gosec // test fixture.
		t.Fatalf("write project.godot: %v", err)
	}

	_, err := s.Run(context.Background(), project, "op", nil)
	if err == nil {
		t.Fatal("Run succeeded on a broken project.godot, want an error")
	}
	if !strings.Contains(err.Error(), "project.godot failed to load") || !strings.Contains(err.Error(), "Error parsing") {
		t.Errorf("err = %v, want the project.godot parse diagnostic", err)
	}
	starts := f.startCount(t)

	if _, err := s.Run(context.Background(), project, "op", nil); err == nil {
		t.Fatal("second Run succeeded, want the remembered parse error")
	}
	if got := f.startCount(t); got != starts {
		t.Errorf("starts = %d, want %d: the broken project must not respawn on every call", got, starts)
	}
}

func TestSessionEOFRespawn(t *testing.T) {
	shortRuntimeDir(t)
	f := newSocketSessionFake(t, defaultSocketResponse+"\n        sys.exit(0)")
	s := NewSession(f.bin, "/ops.gd")
	defer s.Close()
	project := t.TempDir()

	if _, err := s.Run(context.Background(), project, "op", nil); err != nil {
		t.Fatalf("first Run: %v", err)
	}
	time.Sleep(50 * time.Millisecond)
	if _, err := s.Run(context.Background(), project, "op", nil); err != nil {
		t.Fatalf("second Run: %v", err)
	}
	if got := f.startCount(t); got != 2 {
		t.Errorf("starts = %d, want 2 after EOF", got)
	}
}

// A first-write failure (process died between the liveness check and the write) respawns once and retries.
func TestSessionRetriesOnceOnWriteFailure(t *testing.T) {
	shortRuntimeDir(t)
	f := newSocketSessionFake(t, defaultSocketResponse)
	s := NewSession(f.bin, "/ops.gd")
	defer s.Close()
	project := t.TempDir()

	conn, peer := net.Pipe()
	_ = peer.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s.proc = &sessionProc{
		cmd:     exec.CommandContext(ctx, f.bin), //nolint:gosec // test-controlled fake.
		cancel:  cancel,
		socket:  &engineSocket{conn: conn},
		done:    make(chan struct{}),
		quit:    make(chan struct{}),
		frames:  make(chan *responseFrame, 1),
		project: project,
	}

	run, err := s.Run(context.Background(), project, "op", nil)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if run.Stdout != "out-1" {
		t.Errorf("stdout = %q, want the retried response", run.Stdout)
	}
	if got := f.startCount(t); got != 1 {
		t.Errorf("starts = %d, want 1: the retry spawns a fresh process", got)
	}
}

func TestSessionResponseIDDesyncAborts(t *testing.T) {
	shortRuntimeDir(t)
	f := newSocketSessionFake(t, `
        send(conn, req["id"] + 100, True, "wrong", "")`)
	s := NewSession(f.bin, "/ops.gd")
	defer s.Close()
	project := t.TempDir()

	_, err := s.Run(context.Background(), project, "op", nil)
	if err == nil {
		t.Fatal("Run accepted a mismatched response id, want an error")
	}
	if !strings.Contains(err.Error(), "response id") {
		t.Errorf("err = %v, want a response-id desync", err)
	}

	healthy := newSocketSessionFake(t, defaultSocketResponse)
	s.executable = healthy.bin
	if _, err := s.Run(context.Background(), project, "op", nil); err != nil {
		t.Fatalf("Run after desync: %v", err)
	}
}

func TestSessionContextCancelKill(t *testing.T) {
	shortRuntimeDir(t)
	f := newSocketSessionFake(t, "\n        pass")
	s := NewSession(f.bin, "/ops.gd")
	defer s.Close()
	project := t.TempDir()

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	start := time.Now()
	_, err := s.Run(ctx, project, "op", nil)
	if err == nil {
		t.Fatal("Run reported success, want a context error")
	}
	if elapsed := time.Since(start); elapsed >= 10*time.Second {
		t.Fatalf("Run took %s, want it to stop well before a wedged engine", elapsed)
	}

	healthy := newSocketSessionFake(t, defaultSocketResponse)
	s.executable = healthy.bin
	if _, err := s.Run(context.Background(), project, "op", nil); err != nil {
		t.Fatalf("Run after cancel: %v", err)
	}
}

// A stuck operation holding the session lock must not keep Close from returning:
// past the grace period the engine is force-killed.
func TestSessionCloseBoundedWhenLocked(t *testing.T) {
	s := NewSession("godot", "/ops.gd")
	s.closeTimeout = 50 * time.Millisecond

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cmd := exec.CommandContext(ctx, "sleep", "60") //nolint:gosec // test-controlled fixed command.
	if err := cmd.Start(); err != nil {
		t.Skipf("start sleep: %v", err)
	}
	p := &sessionProc{cmd: cmd, cancel: cancel, done: make(chan struct{}), quit: make(chan struct{})}
	s.proc = p
	s.procRef.Store(p)

	waited := make(chan error, 1)
	go func() { waited <- cmd.Wait(); close(p.done) }()

	s.mu.Lock()
	start := time.Now()
	closed := make(chan struct{})
	go func() { _ = s.Close(); close(closed) }()

	select {
	case <-closed:
	case <-time.After(2 * time.Second):
		s.mu.Unlock()
		t.Fatal("Close blocked while an operation held the session lock")
	}
	if elapsed := time.Since(start); elapsed < s.closeTimeout {
		t.Errorf("Close returned after %s, want at least the %s grace", elapsed, s.closeTimeout)
	}

	select {
	case <-waited:
	case <-time.After(2 * time.Second):
		s.mu.Unlock()
		t.Fatal("the engine process was not killed")
	}
	s.mu.Unlock()
}

func TestSessionCloseQuitAndFailsRuns(t *testing.T) {
	shortRuntimeDir(t)
	f := newSocketSessionFake(t, defaultSocketResponse)
	s := NewSession(f.bin, "/ops.gd")
	project := t.TempDir()

	if _, err := s.Run(context.Background(), project, "op", nil); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if err := s.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if !f.quitRecorded(t) {
		t.Error("Close did not send the bridge a quit operation")
	}
	if err := s.Close(); err != nil {
		t.Fatalf("second Close: %v", err)
	}
	if _, err := s.Run(context.Background(), project, "op", nil); err == nil {
		t.Fatal("Run after Close succeeded, want an error")
	}
}
