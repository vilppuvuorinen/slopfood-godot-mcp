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

// TestScriptFailed covers the shared failure check: the script's quit(1) is the
// authoritative and only signal.
func TestScriptFailed(t *testing.T) {
	tests := []struct {
		name     string
		exitCode int
		want     bool
	}{
		{name: "clean success", exitCode: 0, want: false},
		{name: "failure exit", exitCode: 1, want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := scriptFailed(tt.exitCode); got != tt.want {
				t.Errorf("scriptFailed(%d) = %v, want %v", tt.exitCode, got, tt.want)
			}
		})
	}
}
