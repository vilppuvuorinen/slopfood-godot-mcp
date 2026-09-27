package daemonpath

import (
	"path/filepath"
	"strings"
	"testing"
)

// TestProjectHashDeterministic asserts the project hash is stable across calls
// and distinct between projects, including for an uncleaned path that cleans to
// the same root.
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

// TestSocketAndLogPaths asserts both paths share the runtime directory and
// project hash and carry the expected suffix.
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

// TestRuntimeDirHonorsXDG asserts XDG_RUNTIME_DIR wins when it is set.
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

// TestRuntimeDirFallsBackToCache asserts the user cache directory is used when
// XDG_RUNTIME_DIR is unset.
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
