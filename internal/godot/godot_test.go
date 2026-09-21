package godot

import (
	"context"
	"os"
	"path/filepath"
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
