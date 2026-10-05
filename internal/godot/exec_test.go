package godot

import (
	"context"
	"os"
	"path/filepath"
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

// A canceled context must kill the running engine; a killed process surfaces as a non-zero exit, not an error.
func TestRunEngineContextCancelStopsEngine(t *testing.T) {
	bin := writeExecutable(t, "exec sleep 30")

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	start := time.Now()
	_, _, exitCode, err := runEngine(ctx, bin)
	elapsed := time.Since(start)

	if elapsed >= 10*time.Second {
		t.Fatalf("runEngine took %s, want it to stop well before the 30s sleep", elapsed)
	}
	if err == nil && exitCode == 0 {
		t.Fatal("runEngine reported success, want the engine stopped by context cancellation")
	}
}
