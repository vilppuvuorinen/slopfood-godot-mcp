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

// shortRuntimeDir points XDG_RUNTIME_DIR at a short directory for one test, so
// the derived engine socket path stays under the Linux sun_path limit even
// though t.TempDir embeds a long test name.
func shortRuntimeDir(t *testing.T) {
	t.Helper()
	dir, err := os.MkdirTemp("", "sess-rt-")
	if err != nil {
		t.Fatalf("create runtime dir: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	t.Setenv("XDG_RUNTIME_DIR", dir)
}

// socketSessionFake is a fake engine process that speaks the bridge's
// newline-delimited Unix-socket protocol. It records one line per process start
// in starts, the spawn arguments in args, the engine environment hook value in
// env, and the quit operation in quit. It prints an engine banner and a
// non-JSON stdout line before the readiness object, binds the --gd-socket path
// it was handed, and runs the python snippet response for each request; a quit
// operation is always answered and recorded before the process exits.
type socketSessionFake struct {
	bin    string
	starts string
	args   string
	env    string
	quit   string
	// ops records each request line, so a test can assert the operations and
	// params the session sent without a transport-level response assertion.
	ops string
}

// defaultSocketResponse answers every request by echoing its id in stdout.
const defaultSocketResponse = `
        send(conn, req["id"], True, "out-" + str(req["id"]), "")`

// newSocketSessionFake writes the fake engine, running response for each
// request. response is a python snippet inserted in the receive loop with
// `req`, `conn`, and send() in scope; it must be indented to match the loop.
func newSocketSessionFake(t *testing.T, response string) *socketSessionFake {
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
exec python3 - "$@" <<'PY'
import json, os, socket, sys
args = sys.argv[1:]
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

// startCount returns how many times the fake has been spawned.
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

func (f *socketSessionFake) envValue(t *testing.T) string {
	t.Helper()
	data, err := os.ReadFile(f.env)
	if err != nil {
		t.Fatalf("read env: %v", err)
	}
	return string(data)
}

// recordedOps returns the decoded request objects the fake received, in order.
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

// TestSessionRoundTripFraming proves one socket engine process serves two
// sequential operations and that the spawn carries the socket args and never
// --gd-daemon.
func TestSessionRoundTripFraming(t *testing.T) {
	shortRuntimeDir(t)
	f := newSocketSessionFake(t, defaultSocketResponse)
	s := NewSession(f.bin, "/ops.gd")
	defer s.Close()
	project := t.TempDir()

	stdout, stderr, exitCode, err := s.Run(context.Background(), project, "create_scene",
		map[string]any{"scenePath": "scenes/main.tscn"})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if exitCode != 0 || stderr != "" || stdout != "out-1" {
		t.Fatalf("first run: stdout=%q stderr=%q exit=%d", stdout, stderr, exitCode)
	}

	stdout, stderr, exitCode, err = s.Run(context.Background(), project, "read_scene",
		map[string]any{"scenePath": "scenes/main.tscn"})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if exitCode != 0 || stderr != "" || stdout != "out-2" {
		t.Fatalf("second run: stdout=%q stderr=%q exit=%d", stdout, stderr, exitCode)
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

// TestSessionFramesRecorded proves the request frames carry the assigned ids and
// the caller's operation/params, and that nil params marshal to {}. The token
// the client adds to the first frame is stripped before comparison.
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

	if _, _, _, err := s.Run(context.Background(), project, "create_scene", map[string]any{"scenePath": "a.tscn"}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if _, _, _, err := s.Run(context.Background(), project, "read_scene", map[string]any{"scenePath": "b.tscn"}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if _, _, _, err := s.Run(context.Background(), project, "add_node", nil); err != nil {
		t.Fatalf("Run: %v", err)
	}

	data, err := os.ReadFile(reqs)
	if err != nil {
		t.Fatalf("read reqs: %v", err)
	}
	got := strings.Split(strings.TrimSpace(string(data)), "\n")
	want := []string{
		`{"id":1,"operation":"create_scene","params":{"scenePath":"a.tscn"}}`,
		`{"id":2,"operation":"read_scene","params":{"scenePath":"b.tscn"}}`,
		`{"id":3,"operation":"add_node","params":{}}`,
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

// TestSessionStderrDiagnosticsOnly proves the deliberate transport change: the
// process stderr pipe is diagnostics only, and the response frame's stderr is
// authoritative and returned alone.
func TestSessionStderrDiagnosticsOnly(t *testing.T) {
	shortRuntimeDir(t)
	response := `
        print("engine diagnostic " + str(req["id"]), file=sys.stderr, flush=True)
        send(conn, req["id"], True, "ok", "frame stderr line")`
	f := newSocketSessionFake(t, response)
	s := NewSession(f.bin, "/ops.gd")
	defer s.Close()

	_, stderr, exitCode, err := s.Run(context.Background(), t.TempDir(), "op", nil)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if exitCode != 0 {
		t.Fatalf("exitCode = %d", exitCode)
	}
	if stderr != "frame stderr line" {
		t.Errorf("stderr = %q, want only the frame's stderr", stderr)
	}
	if strings.Contains(stderr, "engine diagnostic") {
		t.Errorf("stderr merged process diagnostics: %q", stderr)
	}
}

// TestSessionSkipsNonFrameStdout proves the engine banner and a stray non-JSON
// stdout line are not mistaken for a response; the fake prints both.
func TestSessionSkipsNonFrameStdout(t *testing.T) {
	shortRuntimeDir(t)
	f := newSocketSessionFake(t, defaultSocketResponse)
	s := NewSession(f.bin, "/ops.gd")
	defer s.Close()

	stdout, _, _, err := s.Run(context.Background(), t.TempDir(), "op", nil)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if stdout != "out-1" {
		t.Errorf("stdout = %q, want the frame's stdout only", stdout)
	}
}

// TestSessionInvalidateRescans pins the new Invalidate contract: it asks the
// editor to rescan before the next Run instead of forcing a respawn, so the
// project-global change is reflected while the same process keeps serving.
func TestSessionInvalidateRescans(t *testing.T) {
	shortRuntimeDir(t)
	ops := filepath.Join(t.TempDir(), "ops")
	response := `
        open('` + ops + `', "a").write(req["operation"] + "\n")
        send(conn, req["id"], True, "out-" + str(req["id"]), "")`
	f := newSocketSessionFake(t, response)
	s := NewSession(f.bin, "/ops.gd")
	defer s.Close()
	project := t.TempDir()

	if _, _, _, err := s.Run(context.Background(), project, "op", nil); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got := f.startCount(t); got != 1 {
		t.Fatalf("starts = %d, want 1", got)
	}
	s.Invalidate()
	if _, _, _, err := s.Run(context.Background(), project, "op", nil); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got := f.startCount(t); got != 1 {
		t.Errorf("starts = %d, want 1: Invalidate must rescan, not respawn", got)
	}

	data, err := os.ReadFile(ops)
	if err != nil {
		t.Fatalf("read ops: %v", err)
	}
	got := strings.Fields(string(data))
	want := []string{"op", "reimport_project", "op"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("operations = %v, want %v", got, want)
	}
}

// TestSessionReimportProjectDrivesScan proves the session routes the
// reimport_project operation straight to the bridge without a respawn, so the
// warm editor filesystem, not a one-shot --import fork, drives the scan.
func TestSessionReimportProjectDrivesScan(t *testing.T) {
	shortRuntimeDir(t)
	f := newSocketSessionFake(t, defaultSocketResponse)
	s := NewSession(f.bin, "/ops.gd")
	defer s.Close()
	project := t.TempDir()

	if _, _, _, err := s.Run(context.Background(), project, "reimport_project", nil); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got := f.startCount(t); got != 1 {
		t.Fatalf("starts = %d, want 1", got)
	}
	reqs := f.recordedOps(t)
	if len(reqs) != 1 {
		t.Fatalf("requests = %+v, want one", reqs)
	}
	if reqs[0]["operation"] != "reimport_project" {
		t.Errorf("operation = %v, want reimport_project", reqs[0]["operation"])
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
		if _, _, _, err := s.Run(context.Background(), project, "op", nil); err != nil {
			t.Fatalf("Run %d: %v", i, err)
		}
	}
	if got := f.startCount(t); got != 1 {
		t.Fatalf("starts = %d, want 1 after two operations", got)
	}
	if _, _, _, err := s.Run(context.Background(), project, "op", nil); err != nil {
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

	if _, _, _, err := s.Run(context.Background(), t.TempDir(), "op", nil); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if _, _, _, err := s.Run(context.Background(), t.TempDir(), "op", nil); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got := f.startCount(t); got != 2 {
		t.Errorf("starts = %d, want 2 for two projects", got)
	}
}

// TestSessionFingerprintRescans pins the new fingerprint contract: a
// project.godot change is reconciled by a rescan before the next operation
// instead of restarting the engine, so the change is reflected while the same
// process keeps serving. The pre-editor guarantee — a project-global change is
// reflected before the next operation — is preserved either way.
func TestSessionFingerprintRescans(t *testing.T) {
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
	if _, _, _, err := s.Run(context.Background(), project, "op", nil); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got := f.startCount(t); got != 1 {
		t.Fatalf("starts = %d, want 1", got)
	}

	// A different size changes the stamp even if the mtime lands in the same
	// clock tick.
	if err := os.WriteFile(projectFile, []byte("config_version=5\n\n[application]\n"), 0o644); err != nil { //nolint:gosec // test fixture.
		t.Fatalf("rewrite project.godot: %v", err)
	}
	if _, _, _, err := s.Run(context.Background(), project, "op", nil); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got := f.startCount(t); got != 1 {
		t.Errorf("starts = %d, want 1: a fingerprint change must rescan, not respawn", got)
	}

	data, err := os.ReadFile(ops)
	if err != nil {
		t.Fatalf("read ops: %v", err)
	}
	got := strings.Fields(string(data))
	want := []string{"op", "reimport_project", "op"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("operations = %v, want %v", got, want)
	}
}

// TestSessionEOFRespawn proves a frame delivered just before the process exits
// is not lost and that the next Run replaces the dead process.
func TestSessionEOFRespawn(t *testing.T) {
	shortRuntimeDir(t)
	f := newSocketSessionFake(t, defaultSocketResponse+"\n        sys.exit(0)")
	s := NewSession(f.bin, "/ops.gd")
	defer s.Close()
	project := t.TempDir()

	if _, _, _, err := s.Run(context.Background(), project, "op", nil); err != nil {
		t.Fatalf("first Run: %v", err)
	}
	// Give the reader a moment to observe EOF so the death is visible to the
	// next restart decision.
	time.Sleep(50 * time.Millisecond)
	if _, _, _, err := s.Run(context.Background(), project, "op", nil); err != nil {
		t.Fatalf("second Run: %v", err)
	}
	if got := f.startCount(t); got != 2 {
		t.Errorf("starts = %d, want 2 after EOF", got)
	}
}

// TestSessionRetriesOnceOnWriteFailure proves the write-failure supervision: a
// request whose first socket write fails (the process died between the liveness
// check and the write) respawns the engine once and retries the same frame
// instead of surfacing the failure. The session is given a process whose socket
// is already closed, so the write fails deterministically with no live reader to
// race, and the retry then spawns the fake.
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

	stdout, _, _, err := s.Run(context.Background(), project, "op", nil)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if stdout != "out-1" {
		t.Errorf("stdout = %q, want the retried response", stdout)
	}
	if got := f.startCount(t); got != 1 {
		t.Errorf("starts = %d, want 1: the retry spawns a fresh process", got)
	}
}

// TestSessionResponseIDDesyncAborts proves a mismatched response id still kills
// the process instead of returning the wrong frame.
func TestSessionResponseIDDesyncAborts(t *testing.T) {
	shortRuntimeDir(t)
	f := newSocketSessionFake(t, `
        send(conn, req["id"] + 100, True, "wrong", "")`)
	s := NewSession(f.bin, "/ops.gd")
	defer s.Close()
	project := t.TempDir()

	_, _, _, err := s.Run(context.Background(), project, "op", nil)
	if err == nil {
		t.Fatal("Run accepted a mismatched response id, want an error")
	}
	if !strings.Contains(err.Error(), "response id") {
		t.Errorf("err = %v, want a response-id desync", err)
	}

	// The desync kills the process; the next Run respawns a healthy one.
	healthy := newSocketSessionFake(t, defaultSocketResponse)
	s.executable = healthy.bin
	if _, _, _, err := s.Run(context.Background(), project, "op", nil); err != nil {
		t.Fatalf("Run after desync: %v", err)
	}
}

// TestSessionContextCancelKill proves a canceled request kills the wedged engine
// promptly and that the session respawns a healthy one afterwards.
func TestSessionContextCancelKill(t *testing.T) {
	shortRuntimeDir(t)
	// The fake accepts the connection but never answers.
	f := newSocketSessionFake(t, "\n        pass")
	s := NewSession(f.bin, "/ops.gd")
	defer s.Close()
	project := t.TempDir()

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	start := time.Now()
	_, _, _, err := s.Run(ctx, project, "op", nil)
	if err == nil {
		t.Fatal("Run reported success, want a context error")
	}
	if elapsed := time.Since(start); elapsed >= 10*time.Second {
		t.Fatalf("Run took %s, want it to stop well before a wedged engine", elapsed)
	}

	healthy := newSocketSessionFake(t, defaultSocketResponse)
	s.executable = healthy.bin
	if _, _, _, err := s.Run(context.Background(), project, "op", nil); err != nil {
		t.Fatalf("Run after cancel: %v", err)
	}
}

// TestSessionCloseQuitAndFailsRuns proves Close asks the bridge to quit
// gracefully, is idempotent, and makes a later Run fail.
func TestSessionCloseQuitAndFailsRuns(t *testing.T) {
	shortRuntimeDir(t)
	f := newSocketSessionFake(t, defaultSocketResponse)
	s := NewSession(f.bin, "/ops.gd")
	project := t.TempDir()

	if _, _, _, err := s.Run(context.Background(), project, "op", nil); err != nil {
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
	s.Invalidate()
	if _, _, _, err := s.Run(context.Background(), project, "op", nil); err == nil {
		t.Fatal("Run after Close succeeded, want an error")
	}
}

// TestSessionAppliesEngineEnv proves the engine environment hook reaches the
// session-spawned child, mirroring TestRunOperationAppliesEngineEnv.
func TestSessionAppliesEngineEnv(t *testing.T) {
	shortRuntimeDir(t)
	SetEngineEnv(func(env []string) []string {
		return append(env, "GDCOVER_FILE=keyed/hits.txt")
	})
	t.Cleanup(func() { SetEngineEnv(nil) })

	f := newSocketSessionFake(t, defaultSocketResponse)
	s := NewSession(f.bin, "/ops.gd")
	defer s.Close()

	if _, _, _, err := s.Run(context.Background(), t.TempDir(), "op", nil); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got := f.envValue(t); got != "keyed/hits.txt" {
		t.Errorf("GDCOVER_FILE = %q, want keyed/hits.txt", got)
	}
}
