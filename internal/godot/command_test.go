package godot

import (
	"context"
	"strings"
	"testing"
)

// The engine environment hook must reach the spawned process (a coverage build keys hit traces by PID).
func TestRunEngineAppliesEngineEnv(t *testing.T) {
	SetEngineEnv(func(env []string) []string {
		return append(env, "GDCOVER_FILE=keyed/hits.txt")
	})
	t.Cleanup(func() { SetEngineEnv(nil) })

	bin := writeExecutable(t, `printf '%s' "$GDCOVER_FILE"`)
	stdout, _, _, err := runEngine(context.Background(), bin, "op")
	if err != nil {
		t.Fatalf("runEngine: %v", err)
	}
	if !strings.Contains(stdout, "keyed/hits.txt") {
		t.Errorf("stdout = %q, want the hook's GDCOVER_FILE", stdout)
	}
}
