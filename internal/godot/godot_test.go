package godot

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func writeScript(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "godot")
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"+body+"\n"), 0o755); err != nil { //nolint:gosec // test helper needs an executable script.
		t.Fatalf("write script: %v", err)
	}
	return path
}

func writeFile(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte("binary"), 0o755); err != nil { //nolint:gosec // test helper needs an executable script.
		t.Fatalf("write binary: %v", err)
	}
}

func TestPathEnvOverride(t *testing.T) {
	t.Setenv(PathEnv, "/custom/godot")
	if got := Path(); got != "/custom/godot" {
		t.Fatalf("Path() = %q, want /custom/godot", got)
	}
}

func TestPathIgnoresBlankEnv(t *testing.T) {
	t.Setenv(PathEnv, "   ")
	if got := Path(); got == "   " {
		t.Fatalf("Path() = %q, want a resolved path", got)
	}
}

// TestPathEnvOverrideIsNotCached confirms the local-build cache never masks an
// explicit GODOT_PATH: the override is consulted on every call, so changing it
// after a lookup still takes effect and the cache only covers the filesystem
// walk.
func TestPathEnvOverrideIsNotCached(t *testing.T) {
	t.Setenv(PathEnv, "/first/godot")
	if got := Path(); got != "/first/godot" {
		t.Fatalf("Path() = %q, want /first/godot", got)
	}
	if err := os.Setenv(PathEnv, "/second/godot"); err != nil {
		t.Fatalf("setenv: %v", err)
	}
	if got := Path(); got != "/second/godot" {
		t.Fatalf("Path() = %q, want /second/godot", got)
	}
}

func TestFindInFindsMinorBuild(t *testing.T) {
	root := t.TempDir()
	binary := filepath.Join(root, ".godot", "4.7", "Godot_v4.7.2-stable_linux.x86_64")
	writeFile(t, binary)

	if got := findIn([]string{root}); got != binary {
		t.Fatalf("findIn() = %q, want %q", got, binary)
	}
}

func TestFindInWalksUp(t *testing.T) {
	root := t.TempDir()
	binary := filepath.Join(root, ".godot", "4.7", "Godot_v4.7.2-stable_linux.x86_64")
	writeFile(t, binary)

	nested := filepath.Join(root, "a", "b")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if got := findIn([]string{nested}); got != binary {
		t.Fatalf("findIn() = %q, want %q", got, binary)
	}
}

func TestFindInMissing(t *testing.T) {
	if got := findIn([]string{t.TempDir()}); got != "" {
		t.Fatalf("findIn() = %q, want empty", got)
	}
}

func TestEngineGlobs(t *testing.T) {
	tests := []struct {
		goos string
		want []string
	}{
		{"linux", []string{"Godot_v*_linux.*"}},
		{"windows", []string{"Godot_v*_win64_console.exe", "Godot_v*_win64.exe", "Godot_v*_win32.exe"}},
		{"darwin", []string{"Godot.app/Contents/MacOS/Godot", "Godot_v*_macos.*"}},
	}
	for _, tt := range tests {
		got := engineGlobs(tt.goos)
		if len(got) != len(tt.want) {
			t.Fatalf("engineGlobs(%q) = %q, want %q", tt.goos, got, tt.want)
		}
		for i := range got {
			if got[i] != tt.want[i] {
				t.Fatalf("engineGlobs(%q) = %q, want %q", tt.goos, got, tt.want)
			}
		}
	}
}

func TestFindInSkipsNonExecutable(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("execute bits are not meaningful on Windows")
	}
	root := t.TempDir()
	binary := filepath.Join(root, ".godot", "4.7", "Godot_v4.7.2-stable_linux.x86_64")
	if err := os.MkdirAll(filepath.Dir(binary), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(binary, []byte("binary"), 0o644); err != nil { //nolint:gosec // deliberately not executable.
		t.Fatalf("write binary: %v", err)
	}

	if got := findIn([]string{root}); got != "" {
		t.Fatalf("findIn() = %q, want empty for a non-executable match", got)
	}
}

func TestVersion(t *testing.T) {
	path := writeScript(t, "echo 4.7.2.stable.official.ed1daf0bf")
	got, err := Version(context.Background(), path)
	if err != nil {
		t.Fatalf("Version: %v", err)
	}
	if got != "4.7.2.stable.official.ed1daf0bf" {
		t.Fatalf("Version() = %q", got)
	}
}

func TestVersionTrimsWhitespace(t *testing.T) {
	path := writeScript(t, "printf '  4.7.2.stable  \\n\\n'")
	got, err := Version(context.Background(), path)
	if err != nil {
		t.Fatalf("Version: %v", err)
	}
	if got != "4.7.2.stable" {
		t.Fatalf("Version() = %q", got)
	}
}

func TestVersionError(t *testing.T) {
	path := writeScript(t, "echo boom >&2\nexit 1")
	_, err := Version(context.Background(), path)
	if err == nil {
		t.Fatal("expected an error")
	}
	if !strings.Contains(err.Error(), "boom") {
		t.Fatalf("error = %v, want it to include stderr", err)
	}
}

func TestVersionMissingBinary(t *testing.T) {
	_, err := Version(context.Background(), filepath.Join(t.TempDir(), "nope"))
	if err == nil {
		t.Fatal("expected an error")
	}
}
