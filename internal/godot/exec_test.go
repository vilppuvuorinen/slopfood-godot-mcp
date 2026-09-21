package godot

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeExecutable(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "godot")
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"+body+"\n"), 0o755); err != nil { //nolint:gosec // test helper needs an executable script.
		t.Fatalf("write script: %v", err)
	}
	return path
}

func TestRunOperationArgsAndSuccess(t *testing.T) {
	bin := writeExecutable(t, `echo "$@"`)

	stdout, stderr, err := RunOperation(context.Background(), bin, "/proj", "/script.gd", "create_scene",
		OperationParams{"scene_path": "main.tscn"})
	if err != nil {
		t.Fatalf("RunOperation: %v", err)
	}
	if stderr != "" {
		t.Errorf("stderr = %q, want empty", stderr)
	}
	for _, want := range []string{
		"--headless",
		"--path /proj",
		"--script /script.gd",
		"create_scene",
		`{"scene_path":"main.tscn"}`,
		"--debug-godot",
	} {
		if !strings.Contains(stdout, want) {
			t.Errorf("stdout = %q, want it to contain %q", stdout, want)
		}
	}
}

func TestRunOperationCapturesOutputOnNonZeroExit(t *testing.T) {
	bin := writeExecutable(t, "echo out\necho 'Failed to create scene' >&2\nexit 1")

	stdout, stderr, err := RunOperation(context.Background(), bin, "/proj", "/script.gd", "create_scene", nil)
	if err != nil {
		t.Fatalf("RunOperation: %v, want nil (the script reports its own errors)", err)
	}
	if !strings.Contains(stdout, "out") {
		t.Errorf("stdout = %q", stdout)
	}
	if !strings.Contains(stderr, "Failed to create scene") {
		t.Errorf("stderr = %q", stderr)
	}
}

func TestRunOperationSpawnFailure(t *testing.T) {
	_, _, err := RunOperation(context.Background(), filepath.Join(t.TempDir(), "nope"), "/proj", "/script.gd", "create_scene", nil)
	if err == nil {
		t.Fatal("expected an error when the executable cannot be started")
	}
}
