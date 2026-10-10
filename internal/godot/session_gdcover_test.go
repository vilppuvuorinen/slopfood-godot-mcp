//go:build gdcover

package godot

import (
	"context"
	"testing"
)

func TestSessionAppliesEngineEnv(t *testing.T) {
	shortRuntimeDir(t)
	EngineEnv = func(env []string) []string {
		return append(env, "GDCOVER_FILE=keyed/hits.txt")
	}
	t.Cleanup(func() { EngineEnv = nil })

	f := newSocketSessionFake(t, defaultSocketResponse)
	s := NewSessionTransport(f.bin, "/ops.gd", EngineTransportAuto)
	defer s.Close()

	if _, err := s.Run(context.Background(), t.TempDir(), "op", nil); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got := f.envValue(t); got != "keyed/hits.txt" {
		t.Errorf("GDCOVER_FILE = %q, want keyed/hits.txt", got)
	}
}
