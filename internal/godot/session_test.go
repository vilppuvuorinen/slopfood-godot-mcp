package godot

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// sessionFake is a fake daemon engine. It records one line per process start in
// starts, then loops reading request lines from stdin, parsing the id with sed
// and emitting a frame plus the matching stderr terminator. args records the
// arguments the session passed.
type sessionFake struct {
	bin    string
	starts string
	args   string
}

// newSessionFake writes a daemon fake driven by response, which is a shell
// snippet emitting a frame and its terminator after $id is set. It may ignore
// its args.
func newSessionFake(t *testing.T, response string) *sessionFake {
	t.Helper()
	dir := t.TempDir()
	f := &sessionFake{
		bin:    filepath.Join(dir, "godot"),
		starts: filepath.Join(dir, "starts"),
		args:   filepath.Join(dir, "args"),
	}
	body := `printf '%s\n' "$*" >> '` + f.args + `'
echo start >> '` + f.starts + `'
echo 'Godot Engine v4.7 fake'
echo '{"ready":true}'
while IFS= read -r line; do
  id=$(printf '%s\n' "$line" | sed -n 's/.*"id":\([0-9][0-9]*\).*/\1/p')
  [ -z "$id" ] && continue
` + response + `
done
`
	if err := os.WriteFile(f.bin, []byte("#!/bin/sh\n"+body), 0o755); err != nil { //nolint:gosec // test helper needs an executable script.
		t.Fatalf("write fake: %v", err)
	}
	return f
}

// defaultResponse succeeds with a fixed stdout and empty stderr.
const defaultResponse = `  printf '{"id":%s,"ok":true,"stdout":"out-%s","stderr":""}\n' "$id" "$id"
  printf 'GODOT_MCP_STDERR_END %s\n' "$id" >&2`

// startCount returns how many times the fake has been spawned.
func (f *sessionFake) startCount(t *testing.T) int {
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

func (f *sessionFake) argLines(t *testing.T) []string {
	t.Helper()
	data, err := os.ReadFile(f.args)
	if err != nil {
		t.Fatalf("read args: %v", err)
	}
	return strings.Split(strings.TrimSpace(string(data)), "\n")
}

func TestSessionRoundTripFraming(t *testing.T) {
	f := newSessionFake(t, defaultResponse)
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
	for _, want := range []string{"--headless", "--path " + project, "--script /ops.gd", "--gd-daemon"} {
		if !strings.Contains(lines[0], want) {
			t.Errorf("spawn args %q missing %q", lines[0], want)
		}
	}
}

// TestSessionFramesRecorded proves the request frames carry the assigned ids and
// the caller's operation/params, and that nil params marshal to {}. The fake
// copies each stdin line into a file.
func TestSessionFramesRecorded(t *testing.T) {
	reqs := filepath.Join(t.TempDir(), "reqs")
	f := newSessionFake(t, `  printf '%s\n' "$line" >> '`+reqs+`'
`+defaultResponse)
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

// TestSessionStderrMerge proves the process-stderr batch precedes the frame's
// own stderr and never includes the terminator.
func TestSessionStderrMerge(t *testing.T) {
	f := newSessionFake(t, `  printf 'engine diagnostic %s\n' "$id" >&2
  printf '{"id":%s,"ok":true,"stdout":"ok","stderr":"frame stderr line"}\n' "$id"
  printf 'GODOT_MCP_STDERR_END %s\n' "$id" >&2`)
	s := NewSession(f.bin, "/ops.gd")
	defer s.Close()

	_, stderr, exitCode, err := s.Run(context.Background(), t.TempDir(), "op", nil)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if exitCode != 0 {
		t.Fatalf("exitCode = %d", exitCode)
	}
	if strings.Contains(stderr, "GODOT_MCP_STDERR_END") {
		t.Errorf("stderr leaked the terminator: %q", stderr)
	}
	procAt := strings.Index(stderr, "engine diagnostic 1")
	frameAt := strings.Index(stderr, "frame stderr line")
	if procAt < 0 || frameAt < 0 {
		t.Fatalf("stderr = %q, want both lines", stderr)
	}
	if procAt > frameAt {
		t.Errorf("process stderr should precede frame stderr: %q", stderr)
	}
}

// TestSessionSkipsNonFrameStdout proves the banner and readiness line are not
// mistaken for a response.
func TestSessionSkipsNonFrameStdout(t *testing.T) {
	f := newSessionFake(t, defaultResponse)
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

func TestSessionInvalidateRestarts(t *testing.T) {
	f := newSessionFake(t, defaultResponse)
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
	if got := f.startCount(t); got != 2 {
		t.Errorf("starts = %d, want 2 after Invalidate", got)
	}
}

func TestSessionOperationCapRestarts(t *testing.T) {
	f := newSessionFake(t, defaultResponse)
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
	f := newSessionFake(t, defaultResponse)
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

func TestSessionFingerprintRestarts(t *testing.T) {
	f := newSessionFake(t, defaultResponse)
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
	if got := f.startCount(t); got != 2 {
		t.Errorf("starts = %d, want 2 after the fingerprint changed", got)
	}
}

// TestSessionEOFRespawn proves a process that exits after its first response is
// replaced by the next Run.
func TestSessionEOFRespawn(t *testing.T) {
	f := newSessionFake(t, defaultResponse+`
  exit 0`)
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

// TestSessionContextCancelKill proves a canceled request kills the wedged engine
// promptly and that the session respawns a healthy one afterwards.
func TestSessionContextCancelKill(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, "godot")
	body := `#!/bin/sh
echo '{"ready":true}'
while IFS= read -r line; do
  exec sleep 30
done
`
	if err := os.WriteFile(bin, []byte(body), 0o755); err != nil { //nolint:gosec // test helper.
		t.Fatalf("write fake: %v", err)
	}
	s := NewSession(bin, "/ops.gd")
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
		t.Fatalf("Run took %s, want it to stop well before the 30s sleep", elapsed)
	}

	healthy := newSessionFake(t, defaultResponse)
	s.executable = healthy.bin
	if _, _, _, err := s.Run(context.Background(), project, "op", nil); err != nil {
		t.Fatalf("Run after cancel: %v", err)
	}
}

func TestSessionCloseIdempotentAndFailsRuns(t *testing.T) {
	f := newSessionFake(t, defaultResponse)
	s := NewSession(f.bin, "/ops.gd")
	project := t.TempDir()

	if _, _, _, err := s.Run(context.Background(), project, "op", nil); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if err := s.Close(); err != nil {
		t.Fatalf("Close: %v", err)
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
	SetEngineEnv(func(env []string) []string {
		return append(env, "GDCOVER_FILE=keyed/hits.txt")
	})
	t.Cleanup(func() { SetEngineEnv(nil) })

	envFile := filepath.Join(t.TempDir(), "env")
	f := newSessionFake(t, `  printf '%s' "$GDCOVER_FILE" > '`+envFile+`'
`+defaultResponse)
	s := NewSession(f.bin, "/ops.gd")
	defer s.Close()

	if _, _, _, err := s.Run(context.Background(), t.TempDir(), "op", nil); err != nil {
		t.Fatalf("Run: %v", err)
	}
	data, err := os.ReadFile(envFile)
	if err != nil {
		t.Fatalf("read env: %v", err)
	}
	if string(data) != "keyed/hits.txt" {
		t.Errorf("GDCOVER_FILE = %q, want keyed/hits.txt", string(data))
	}
}
