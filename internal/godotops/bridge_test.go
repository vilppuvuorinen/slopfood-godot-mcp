package godotops

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/vilppuvuorinen/slopfood-godot-mcp/internal/godot"
)

// requireGodot returns a usable engine path or skips the test. The unit-test
// job can run without Godot, so a missing engine is a skip rather than a
// failure.
func requireGodot(t *testing.T) string {
	t.Helper()
	path := godot.Path()
	if _, err := godot.Version(context.Background(), path); err != nil {
		t.Skipf("Godot unavailable at %q: %v", path, err)
	}
	return path
}

// TestDispatchSurvivesHandlerAbort is the end-to-end guard for the bridge's
// recovery path: a handler that hits an unhandled engine error returns null
// instead of its Result, which would make the driver dereference null and leave
// the operation without a response frame. The operation must instead fail
// cleanly with exit code 1. read_scene is the vehicle: a non-string scenePath
// cannot call begins_with, so the handler aborts.
func TestDispatchSurvivesHandlerAbort(t *testing.T) {
	engine := requireGodot(t)
	withCache(t)
	script, err := materialize("godot_operations", operationsScript)
	if err != nil {
		t.Fatalf("materialize: %v", err)
	}
	project := t.TempDir()
	if writeErr := os.WriteFile(filepath.Join(project, "project.godot"), []byte("config_version=5\n"), 0o644); writeErr != nil { //nolint:gosec // test fixture.
		t.Fatalf("write project.godot: %v", writeErr)
	}

	sess := godot.NewSession(engine, script)
	defer func() { _ = sess.Close() }()
	// A bounded context keeps a regression from hanging the suite.
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	_, stderr, exitCode, err := sess.Run(ctx, project, "read_scene",
		godot.OperationParams{"scenePath": 123})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if exitCode != 1 {
		t.Fatalf("exitCode = %d, want 1; stderr:\n%s", exitCode, stderr)
	}
	if !strings.Contains(stderr, "unhandled engine error") {
		t.Errorf("stderr = %q, want the dispatcher's recovery message", stderr)
	}
}
