package godot

import (
	"bufio"
	"context"
	"encoding/base64"
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestParseReadySkipsBanner(t *testing.T) {
	for _, line := range []string{
		"Godot Engine v4.7.2.stable.official.ed1daf0bf - https://godotengine.org\n",
		"some engine diagnostic\n",
		"{\"ready\":false}\n",
	} {
		info, err := parseReady(line)
		if err != nil {
			t.Errorf("parseReady(%q) error = %v, want nil", line, err)
		}
		if info != nil {
			t.Errorf("parseReady(%q) = %+v, want nil", line, info)
		}
	}
}

func TestParseReadyObject(t *testing.T) {
	line := `{"ready":true,"transport":"uds","path":"/tmp/p.engine.sock","token":"ab"}` + "\n"
	info, err := parseReady(line)
	if err != nil {
		t.Fatalf("parseReady: %v", err)
	}
	if info == nil {
		t.Fatal("parseReady returned nil, want the readiness object")
	}
	if !info.Ready || info.Transport != "uds" || info.Path != "/tmp/p.engine.sock" || info.Token != "ab" {
		t.Errorf("readiness = %+v", info)
	}
}

func TestParseReadyMalformed(t *testing.T) {
	if _, err := parseReady(`{"ready":` + "\n"); err == nil {
		t.Fatal("parseReady accepted malformed JSON, want an error")
	}
}

func TestReadReadinessSkipsBanner(t *testing.T) {
	lines := "Godot Engine v4.7.2\nready\n" + `{"ready":true,"transport":"uds","path":"p","token":"t"}` + "\n"
	info, err := readReadiness(bufio.NewReader(strings.NewReader(lines)), func() {})
	if err != nil {
		t.Fatalf("readReadiness: %v", err)
	}
	if info == nil || info.Token != "t" {
		t.Fatalf("readiness = %+v", info)
	}
}

func TestReadReadinessMalformed(t *testing.T) {
	if _, err := readReadiness(bufio.NewReader(strings.NewReader(`{"ready":`+"\n")), func() {}); err == nil {
		t.Fatal("readReadiness accepted a malformed readiness line, want an error")
	}
}

// fakeUDSServer is an in-process Unix-socket server that speaks the bridge's
// newline-delimited framing, so the client can be exercised without an engine.
type fakeUDSServer struct {
	t        *testing.T
	listener net.Listener
	path     string
	conn     net.Conn
	reader   *bufio.Reader
}

func newFakeUDSServer(t *testing.T) *fakeUDSServer {
	t.Helper()
	path := filepath.Join(t.TempDir(), "engine.sock")
	var lc net.ListenConfig
	listener, err := lc.Listen(context.Background(), "unix", path)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	server := &fakeUDSServer{t: t, listener: listener, path: path}
	t.Cleanup(server.close)
	return server
}

func (s *fakeUDSServer) accept() {
	s.t.Helper()
	conn, err := s.listener.Accept()
	if err != nil {
		s.t.Fatalf("accept: %v", err)
	}
	s.conn = conn
	s.reader = bufio.NewReader(conn)
}

func (s *fakeUDSServer) readLine() string {
	s.t.Helper()
	line, err := s.reader.ReadString('\n')
	if err != nil {
		s.t.Fatalf("read line: %v", err)
	}
	return strings.TrimRight(line, "\n")
}

func (s *fakeUDSServer) write(text string) {
	s.t.Helper()
	if _, err := s.conn.Write([]byte(text)); err != nil {
		s.t.Fatalf("write: %v", err)
	}
}

func (s *fakeUDSServer) close() {
	if s.conn != nil {
		_ = s.conn.Close()
	}
	if s.listener != nil {
		_ = s.listener.Close()
	}
}

// TestEngineSocketReadFrameReassemblesPartialReads proves the socket reader
// survives a frame split across writes and returns multiple frames delivered in
// one write, with no read-size cap or blank-line terminator.
func TestEngineSocketReadFrameReassemblesPartialReads(t *testing.T) {
	server := newFakeUDSServer(t)
	client, err := dialEngineSocket(context.Background(), server.path, "tok")
	if err != nil {
		t.Fatalf("dialEngineSocket: %v", err)
	}
	defer client.Close()
	server.accept()

	// Frame 1 arrives in three pieces; frames 2 and 3 share one write.
	server.write(`{"id":1,"ok":true,`)
	server.write(`"stdout":"one","stderr":""}`)
	server.write("\n")
	server.write(`{"id":2,"ok":true,"stdout":"two","stderr":""}` + "\n" +
		`{"id":3,"ok":false,"stdout":"","stderr":"bad"}` + "\n")

	want := []struct {
		id     float64
		ok     bool
		stdout string
		stderr string
	}{
		{1, true, "one", ""},
		{2, true, "two", ""},
		{3, false, "", "bad"},
	}
	for i, w := range want {
		frame, err := client.readFrame()
		if err != nil {
			t.Fatalf("readFrame %d: %v", i+1, err)
		}
		if frame.ID == nil || *frame.ID != w.id || frame.OK != w.ok || frame.Stdout != w.stdout || frame.Stderr != w.stderr {
			t.Fatalf("frame %d = %+v, want id=%v ok=%v stdout=%q stderr=%q", i+1, frame, w.id, w.ok, w.stdout, w.stderr)
		}
	}
}

// TestEngineSocketPresentsTokenOnce proves the client puts the token on the
// first request frame of a connection and omits it from later frames.
func TestEngineSocketPresentsTokenOnce(t *testing.T) {
	server := newFakeUDSServer(t)
	client, err := dialEngineSocket(context.Background(), server.path, "secret")
	if err != nil {
		t.Fatalf("dialEngineSocket: %v", err)
	}
	defer client.Close()
	server.accept()

	if err := client.writeRequest([]byte(`{"id":1,"operation":"ping","params":{}}`)); err != nil {
		t.Fatalf("first writeRequest: %v", err)
	}
	first := decodeLine(t, server.readLine())
	if first["token"] != "secret" {
		t.Errorf("first frame token = %v, want secret", first["token"])
	}
	server.write(`{"id":1,"ok":true,"stdout":"pong","stderr":""}` + "\n")
	if _, err := client.readFrame(); err != nil {
		t.Fatalf("readFrame: %v", err)
	}

	if err := client.writeRequest([]byte(`{"id":2,"operation":"ping","params":{}}`)); err != nil {
		t.Fatalf("second writeRequest: %v", err)
	}
	second := decodeLine(t, server.readLine())
	if _, present := second["token"]; present {
		t.Errorf("second frame repeated the token: %v", second)
	}
}

// TestEngineSocketTokenRejectClosesWithoutServing proves a server that rejects
// the token closes the connection and the client surfaces the failure without
// receiving a response.
func TestEngineSocketTokenRejectClosesWithoutServing(t *testing.T) {
	server := newFakeUDSServer(t)
	client, err := dialEngineSocket(context.Background(), server.path, "wrong")
	if err != nil {
		t.Fatalf("dialEngineSocket: %v", err)
	}
	defer client.Close()
	server.accept()

	served := make(chan struct{})
	go func() {
		line, err := server.reader.ReadString('\n')
		if err != nil {
			return
		}
		request := decodeLine(t, strings.TrimRight(line, "\n"))
		if request["token"] != "secret" {
			_ = server.conn.Close()
			return
		}
		close(served)
	}()

	if err := client.writeRequest([]byte(`{"id":1,"operation":"ping","params":{}}`)); err != nil {
		t.Fatalf("writeRequest: %v", err)
	}
	if _, err := client.readFrame(); err == nil {
		t.Fatal("readFrame succeeded after a token reject, want a closed-connection error")
	}
	select {
	case <-served:
		t.Fatal("server served an operation despite the rejected token")
	case <-time.After(100 * time.Millisecond):
	}
}

// TestEngineSocketMissingToken proves an empty token (the client never presents
// a matching one) is rejected by a server that requires it.
func TestEngineSocketMissingToken(t *testing.T) {
	server := newFakeUDSServer(t)
	client, err := dialEngineSocket(context.Background(), server.path, "")
	if err != nil {
		t.Fatalf("dialEngineSocket: %v", err)
	}
	defer client.Close()
	server.accept()

	go func() {
		line, readErr := server.reader.ReadString('\n')
		if readErr != nil {
			return
		}
		request := decodeLine(t, strings.TrimRight(line, "\n"))
		if request["token"] != "secret" {
			_ = server.conn.Close()
		}
	}()

	if err := client.writeRequest([]byte(`{"id":1,"operation":"ping","params":{}}`)); err != nil {
		t.Fatalf("writeRequest: %v", err)
	}
	if _, err := client.readFrame(); err == nil {
		t.Fatal("readFrame succeeded with a missing token, want a closed-connection error")
	}
}

func decodeLine(t *testing.T, line string) map[string]any {
	t.Helper()
	var decoded map[string]any
	if err := json.Unmarshal([]byte(line), &decoded); err != nil {
		t.Fatalf("decode frame %q: %v", line, err)
	}
	return decoded
}

// TestPrepareSocketDirAndSecureSocket proves the socket directory is created
// 0700 and a Godot-created 0755 socket is narrowed to 0600, with the stale
// unlink idempotent.
func TestPrepareSocketDirAndSecureSocket(t *testing.T) {
	socketPath := filepath.Join(t.TempDir(), "nested", "engine.sock")
	if err := prepareSocketDir(socketPath); err != nil {
		t.Fatalf("prepareSocketDir: %v", err)
	}
	dirInfo, statErr := os.Stat(filepath.Dir(socketPath))
	if statErr != nil {
		t.Fatalf("stat dir: %v", statErr)
	}
	if dirInfo.Mode().Perm() != 0o700 {
		t.Errorf("socket dir mode = %o, want 700", dirInfo.Mode().Perm())
	}

	// A regular file stands in for a Godot-created 0755 socket.
	if err := os.WriteFile(socketPath, nil, 0o600); err != nil { //nolint:gosec // the mode is widened on the next line to model Godot's socket.
		t.Fatalf("write socket file: %v", err)
	}
	if err := os.Chmod(socketPath, 0o755); err != nil {
		t.Fatalf("chmod socket file: %v", err)
	}
	if err := secureSocket(socketPath); err != nil {
		t.Fatalf("secureSocket: %v", err)
	}
	socketInfo, statErr := os.Stat(socketPath)
	if statErr != nil {
		t.Fatalf("stat socket: %v", statErr)
	}
	if socketInfo.Mode().Perm() != 0o600 {
		t.Errorf("socket mode = %o, want 600", socketInfo.Mode().Perm())
	}

	if err := removeStaleSocket(socketPath); err != nil {
		t.Fatalf("removeStaleSocket: %v", err)
	}
	if _, statErr := os.Stat(socketPath); !os.IsNotExist(statErr) {
		t.Errorf("socket still exists after removeStaleSocket: %v", statErr)
	}
	if err := removeStaleSocket(socketPath); err != nil {
		t.Errorf("removeStaleSocket on a missing file: %v", err)
	}
}

// requireSocketGodot returns a usable engine path or skips the test, mirroring
// the godotops requireGodot helper for the package's socket bridge test.
func requireSocketGodot(t *testing.T) string {
	t.Helper()
	path := Path()
	if _, err := Version(context.Background(), path); err != nil {
		t.Skipf("Godot unavailable at %q: %v", path, err)
	}
	return path
}

// TestSocketSessionDrivesRealBridge is the integration proof: the Go socket
// session boots the real bridge, reads readiness, secures the socket, serves a
// create_scene then a read_scene over the socket, and unlinks the socket on
// close.
func TestSocketSessionDrivesRealBridge(t *testing.T) {
	engine := requireSocketGodot(t)

	script, absErr := filepath.Abs(filepath.Join("..", "godotops", "godot_operations.gd"))
	if absErr != nil {
		t.Fatalf("resolve bridge script: %v", absErr)
	}
	if _, statErr := os.Stat(script); statErr != nil {
		t.Fatalf("bridge script: %v", statErr)
	}

	// A short runtime dir keeps the derived socket path under the sun_path
	// limit even when the test's temporary directories are long.
	runtimeDir, mkErr := os.MkdirTemp("", "sgrun")
	if mkErr != nil {
		t.Fatalf("mkdir runtime: %v", mkErr)
	}
	t.Cleanup(func() { _ = os.RemoveAll(runtimeDir) })
	t.Setenv("XDG_RUNTIME_DIR", runtimeDir)

	project := t.TempDir()
	if writeErr := os.WriteFile(filepath.Join(project, "project.godot"), []byte("config_version=5\n"), 0o600); writeErr != nil {
		t.Fatalf("write project.godot: %v", writeErr)
	}

	session := NewSession(engine, script)
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	if _, stderr, exitCode, runErr := session.Run(ctx, project, "create_scene",
		map[string]any{"scenePath": "scenes/main.tscn"}); runErr != nil {
		t.Fatalf("create_scene: %v", runErr)
	} else if exitCode != 0 {
		t.Fatalf("create_scene exitCode = %d; stderr:\n%s", exitCode, stderr)
	}

	// The socket exists, is secured 0600, and sits in the 0700 runtime dir while
	// the session is live.
	proc := session.proc
	if proc == nil || proc.socketPath == "" {
		t.Fatal("session has no live socket path")
	}
	if base := filepath.Base(proc.socketPath); !strings.HasSuffix(base, ".engine.sock") {
		t.Errorf("socket base = %q, want a .engine.sock suffix", base)
	}
	if info, statErr := os.Stat(proc.socketPath); statErr != nil {
		t.Errorf("stat socket: %v", statErr)
	} else if info.Mode().Perm() != 0o600 {
		t.Errorf("socket mode = %o, want 600", info.Mode().Perm())
	}
	if info, statErr := os.Stat(runtimeDir); statErr != nil {
		t.Errorf("stat runtime dir: %v", statErr)
	} else if info.Mode().Perm() != 0o700 {
		t.Errorf("runtime dir mode = %o, want 700", info.Mode().Perm())
	}

	stdout, stderr, exitCode, err := session.Run(ctx, project, "read_scene",
		map[string]any{"scenePath": "scenes/main.tscn"})
	if err != nil {
		t.Fatalf("read_scene: %v", err)
	}
	if exitCode != 0 {
		t.Fatalf("read_scene exitCode = %d; stderr:\n%s", exitCode, stderr)
	}
	if !strings.Contains(stdout, `"name":"root"`) {
		t.Errorf("read_scene stdout = %q, want the root node payload", stdout)
	}

	socketPath := proc.socketPath
	if err := session.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if _, err := os.Stat(socketPath); !os.IsNotExist(err) {
		t.Errorf("socket still exists after Close: %v", err)
	}
}

// minimalPNG is a 1x1 RGBA PNG, enough for the editor to import as a texture
// and write an .import sidecar.
const minimalPNG = "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAAC0lEQVR4nGNgAAIAAAUAAXpeqz8AAAAASUVORK5CYII="

// TestSocketSessionGatesEditorFilesystem is the Phase 5a validation: the real
// editor server withholds readiness until its initial scan completes, so the
// first operation is served only after the editor has materialized the sidecars
// for files already in the project (a script .uid and a texture .import).
func TestSocketSessionGatesEditorFilesystem(t *testing.T) {
	engine := requireSocketGodot(t)

	script, absErr := filepath.Abs(filepath.Join("..", "godotops", "godot_operations.gd"))
	if absErr != nil {
		t.Fatalf("resolve bridge script: %v", absErr)
	}
	if _, statErr := os.Stat(script); statErr != nil {
		t.Fatalf("bridge script: %v", statErr)
	}

	// A short runtime dir keeps the derived socket path under the sun_path
	// limit even when the test's temporary directories are long.
	runtimeDir, mkErr := os.MkdirTemp("", "sgfs")
	if mkErr != nil {
		t.Fatalf("mkdir runtime: %v", mkErr)
	}
	t.Cleanup(func() { _ = os.RemoveAll(runtimeDir) })
	t.Setenv("XDG_RUNTIME_DIR", runtimeDir)

	project := t.TempDir()
	if writeErr := os.WriteFile(filepath.Join(project, "project.godot"), []byte("config_version=5\n"), 0o600); writeErr != nil {
		t.Fatalf("write project.godot: %v", writeErr)
	}
	if mkErr := os.MkdirAll(filepath.Join(project, "scripts"), 0o755); mkErr != nil {
		t.Fatalf("mkdir scripts: %v", mkErr)
	}
	if writeErr := os.WriteFile(filepath.Join(project, "scripts", "foo.gd"), []byte("extends Node\nclass_name Foo\n"), 0o600); writeErr != nil {
		t.Fatalf("write foo.gd: %v", writeErr)
	}
	png, decodeErr := base64.StdEncoding.DecodeString(minimalPNG)
	if decodeErr != nil {
		t.Fatalf("decode png: %v", decodeErr)
	}
	if writeErr := os.WriteFile(filepath.Join(project, "icon.png"), png, 0o600); writeErr != nil {
		t.Fatalf("write icon.png: %v", writeErr)
	}

	session := NewSession(engine, script)
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	// ping is the first operation; the session can only serve it after the
	// editor reported readiness, which the bridge gates on the initial scan.
	stdout, stderr, exitCode, err := session.Run(ctx, project, "ping", nil)
	if err != nil {
		t.Fatalf("ping: %v", err)
	}
	if exitCode != 0 {
		t.Fatalf("ping exitCode = %d; stderr:\n%s", exitCode, stderr)
	}
	if !strings.Contains(stdout, "pong") {
		t.Errorf("ping stdout = %q, want pong", stdout)
	}

	if _, statErr := os.Stat(filepath.Join(project, "scripts", "foo.gd.uid")); statErr != nil {
		t.Errorf("script .uid sidecar missing after the gated scan: %v", statErr)
	}
	if _, statErr := os.Stat(filepath.Join(project, "icon.png.import")); statErr != nil {
		t.Errorf("texture .import sidecar missing after the gated scan: %v", statErr)
	}
}
