package godot

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
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

	stdout, stderr, exitCode, err := RunOperation(context.Background(), bin, "/proj", "/script.gd", "create_scene",
		OperationParams{"scene_path": "main.tscn"})
	if err != nil {
		t.Fatalf("RunOperation: %v", err)
	}
	if exitCode != 0 {
		t.Errorf("exitCode = %d, want 0", exitCode)
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
	} {
		if !strings.Contains(stdout, want) {
			t.Errorf("stdout = %q, want it to contain %q", stdout, want)
		}
	}
}

func TestRunOperationCapturesOutputOnNonZeroExit(t *testing.T) {
	bin := writeExecutable(t, "echo out\necho 'Failed to create scene' >&2\nexit 1")

	stdout, stderr, exitCode, err := RunOperation(context.Background(), bin, "/proj", "/script.gd", "create_scene", nil)
	if err != nil {
		t.Fatalf("RunOperation: %v, want nil (the script reports its own errors)", err)
	}
	if exitCode != 1 {
		t.Errorf("exitCode = %d, want 1", exitCode)
	}
	if !strings.Contains(stdout, "out") {
		t.Errorf("stdout = %q", stdout)
	}
	if !strings.Contains(stderr, "Failed to create scene") {
		t.Errorf("stderr = %q", stderr)
	}
}

// TestRunOperationContextCancelStopsEngine proves the context reaches
// exec.CommandContext: a canceled context kills the running engine instead of
// letting it run to completion. RunOperation treats a killed process as an
// ordinary non-zero engine exit rather than a wrapper failure, so the proof is
// that the call returns well before the 30s sleep, not with a clean exit.
func TestRunOperationContextCancelStopsEngine(t *testing.T) {
	bin := writeExecutable(t, "exec sleep 30")

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	start := time.Now()
	_, _, exitCode, err := RunOperation(ctx, bin, "/proj", "/script.gd", "create_scene", nil)
	elapsed := time.Since(start)

	if elapsed >= 10*time.Second {
		t.Fatalf("RunOperation took %s, want it to stop well before the 30s sleep", elapsed)
	}
	if err == nil && exitCode == 0 {
		t.Fatal("RunOperation reported success, want the engine stopped by context cancellation")
	}
}

func TestRunOperationSpawnFailure(t *testing.T) {
	_, _, _, err := RunOperation(context.Background(), filepath.Join(t.TempDir(), "nope"), "/proj", "/script.gd", "create_scene", nil)
	if err == nil {
		t.Fatal("expected an error when the executable cannot be started")
	}
}
