package godot

import (
	"context"
	"strings"
	"testing"
)

// The engine environment hook must reach the spawned process (a coverage build keys hit traces by PID).
func TestRunOperationAppliesEngineEnv(t *testing.T) {
	SetEngineEnv(func(env []string) []string {
		return append(env, "GDCOVER_FILE=keyed/hits.txt")
	})
	t.Cleanup(func() { SetEngineEnv(nil) })

	bin := writeExecutable(t, `printf '%s' "$GDCOVER_FILE"`)
	stdout, _, _, err := RunOperation(context.Background(), bin, "/proj", "/script.gd", "op", nil)
	if err != nil {
		t.Fatalf("RunOperation: %v", err)
	}
	if !strings.Contains(stdout, "keyed/hits.txt") {
		t.Errorf("stdout = %q, want the hook's GDCOVER_FILE", stdout)
	}
}
