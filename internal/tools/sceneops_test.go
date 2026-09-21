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
