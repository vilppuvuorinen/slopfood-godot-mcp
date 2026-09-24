package tools

import (
	"os"
	"path/filepath"
	"testing"
)

// fakeGodotOps writes an executable that stands in for Godot in scene and UID
// tool tests. Its body is a POSIX shell script; it is pointed at with
// GODOT_PATH so no real engine is needed.
func fakeGodotOps(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "godot")
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"+body+"\n"), 0o755); err != nil { //nolint:gosec // test helper needs an executable script.
		t.Fatalf("write fake godot: %v", err)
	}
	return path
}

// TestScriptFailed covers the shared failure check: the exit code and stderr
// are combined, so an unexpected stderr line fails even on exit 0.
func TestScriptFailed(t *testing.T) {
	tests := []struct {
		name     string
		exitCode int
		stderr   string
		want     bool
	}{
		{name: "clean success", exitCode: 0, stderr: "", want: false},
		{name: "failure exit", exitCode: 1, stderr: "", want: true},
		{name: "unexpected stderr on exit 0", exitCode: 0, stderr: "ERROR: boom\n", want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := scriptFailed(tt.exitCode, tt.stderr); got != tt.want {
				t.Errorf("scriptFailed(%d, %q) = %v, want %v", tt.exitCode, tt.stderr, got, tt.want)
			}
		})
	}
}
