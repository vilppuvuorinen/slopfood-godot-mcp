//go:build !windows

package e2e

import (
	"path/filepath"
	"testing"
)

func socketPath(t *testing.T, runtimeDir string) string {
	t.Helper()
	pattern := filepath.Join(runtimeDir, "slopfood-godot-mcp", "*.sock")
	matches, err := filepath.Glob(pattern)
	if err != nil {
		t.Fatalf("glob %s: %v", pattern, err)
	}
	if len(matches) != 1 {
		t.Fatalf("socket files = %v, want exactly one under %s", matches, runtimeDir)
	}
	return matches[0]
}
