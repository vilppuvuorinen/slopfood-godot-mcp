package daemonpath

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestProjectHashDeterministic(t *testing.T) {
	first := ProjectHash("/tmp/project")
	if again := ProjectHash("/tmp/project"); first != again {
		t.Errorf("ProjectHash not deterministic: %q, %q", first, again)
	}
	if dirty := ProjectHash("/tmp/project/../project"); dirty != first {
		t.Errorf("ProjectHash split on path cleanup: %q, %q", dirty, first)
	}
	if other := ProjectHash("/tmp/other"); other == first {
		t.Errorf("ProjectHash collided across projects: %q", first)
	}
	if len(first) != 16 {
		t.Errorf("ProjectHash length = %d, want 16 hex chars", len(first))
	}
}

func TestSocketAndLogPaths(t *testing.T) {
	runtimeDir := t.TempDir()
	t.Setenv("XDG_RUNTIME_DIR", runtimeDir)

	socket, err := SocketPath("/tmp/project")
	if err != nil {
		t.Fatalf("SocketPath: %v", err)
	}
	logFile, err := LogPath("/tmp/project")
	if err != nil {
		t.Fatalf("LogPath: %v", err)
	}

	wantDir := filepath.Join(runtimeDir, "slopfood-godot-mcp")
	if got := filepath.Dir(socket); got != wantDir {
		t.Errorf("socket dir = %q, want %q", got, wantDir)
	}
	if got := filepath.Dir(logFile); got != wantDir {
		t.Errorf("log dir = %q, want %q", got, wantDir)
	}
	if !strings.HasSuffix(socket, ".sock") {
		t.Errorf("socket path %q does not end in .sock", socket)
	}
	if !strings.HasSuffix(logFile, ".log") {
		t.Errorf("log path %q does not end in .log", logFile)
	}
	wantBase := filepath.Join(wantDir, ProjectHash("/tmp/project"))
	if socket != wantBase+".sock" {
		t.Errorf("socket path = %q, want %q", socket, wantBase+".sock")
	}
	if logFile != wantBase+".log" {
		t.Errorf("log path = %q, want %q", logFile, wantBase+".log")
	}
	if other, err := SocketPath("/tmp/other"); err != nil {
		t.Fatalf("SocketPath other: %v", err)
	} else if other == socket {
		t.Errorf("SocketPath collided across projects: %q", socket)
	}
}

func TestEngineSocketPath(t *testing.T) {
	runtimeDir := "/tmp/sg-runtime"
	t.Setenv("XDG_RUNTIME_DIR", runtimeDir)

	path, err := EngineSocketPath("/tmp/project")
	if err != nil {
		t.Fatalf("EngineSocketPath: %v", err)
	}
	want := filepath.Join(runtimeDir, "slopfood-godot-mcp", ProjectHash("/tmp/project")+".engine.sock")
	if path != want {
		t.Errorf("EngineSocketPath = %q, want %q", path, want)
	}
	if filepath.Dir(path) != filepath.Join(runtimeDir, "slopfood-godot-mcp") {
		t.Errorf("EngineSocketPath dir = %q", filepath.Dir(path))
	}
	if !strings.HasSuffix(path, ".engine.sock") {
		t.Errorf("path %q does not end in .engine.sock", path)
	}
	daemonPath, err := SocketPath("/tmp/project")
	if err != nil {
		t.Fatalf("SocketPath: %v", err)
	}
	if path == daemonPath {
		t.Errorf("EngineSocketPath collided with SocketPath: %q", path)
	}
	other, err := EngineSocketPath("/tmp/other")
	if err != nil {
		t.Fatalf("EngineSocketPath other: %v", err)
	}
	if other == path {
		t.Errorf("EngineSocketPath collided across projects: %q", path)
	}
	if again, err := EngineSocketPath("/tmp/project"); err != nil {
		t.Fatalf("EngineSocketPath again: %v", err)
	} else if again != path {
		t.Errorf("EngineSocketPath not deterministic: %q, %q", path, again)
	}
	// Godot's sun_path limit is 106 bytes; the derived path must fit.
	if len(path) > 106 {
		t.Errorf("EngineSocketPath length = %d, want <= 106", len(path))
	}
}

func TestRuntimeDirHonorsXDG(t *testing.T) {
	runtimeDir := t.TempDir()
	t.Setenv("XDG_RUNTIME_DIR", runtimeDir)

	got, err := RuntimeDir()
	if err != nil {
		t.Fatalf("RuntimeDir: %v", err)
	}
	if want := filepath.Join(runtimeDir, "slopfood-godot-mcp"); got != want {
		t.Errorf("RuntimeDir = %q, want %q", got, want)
	}
}

func TestRuntimeDirFallsBackToCache(t *testing.T) {
	cacheDir := t.TempDir()
	t.Setenv("XDG_RUNTIME_DIR", "")
	t.Setenv("XDG_CACHE_HOME", cacheDir)

	got, err := RuntimeDir()
	if err != nil {
		t.Fatalf("RuntimeDir: %v", err)
	}
	if want := filepath.Join(cacheDir, "slopfood-godot-mcp"); got != want {
		t.Errorf("RuntimeDir = %q, want %q", got, want)
	}
}
