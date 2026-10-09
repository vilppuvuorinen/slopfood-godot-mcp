//go:build gdcover

package godot

import (
	"context"
	"testing"
)

func TestSessionAppliesEngineEnv(t *testing.T) {
	shortRuntimeDir(t)
	SetEngineEnv(func(env []string) []string {
		return append(env, "GDCOVER_FILE=keyed/hits.txt")
	})
	t.Cleanup(func() { SetEngineEnv(nil) })

	f := newSocketSessionFake(t, defaultSocketResponse)
	s := NewSession(f.bin, "/ops.gd")
	defer s.Close()

	if _, _, _, err := s.Run(context.Background(), t.TempDir(), "op", nil); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got := f.envValue(t); got != "keyed/hits.txt" {
		t.Errorf("GDCOVER_FILE = %q, want keyed/hits.txt", got)
	}
}
