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

func TestSessionDrivesRealBridge(t *testing.T) {
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

	session := godot.NewSession(engine, script)
	defer session.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	if _, stderr, exitCode, runErr := session.Run(ctx, project, "create_scene",
		map[string]any{"scenePath": "scenes/main.tscn"}); runErr != nil {
		t.Fatalf("create_scene: %v", runErr)
	} else if exitCode != 0 {
		t.Fatalf("create_scene exitCode = %d; stderr:\n%s", exitCode, stderr)
	}

	stdout, stderr, exitCode, err := session.Run(ctx, project, "read_scene",
		map[string]any{"scenePath": "scenes/main.tscn"})
	if err != nil {
		t.Fatalf("read_scene: %v", err)
	}
	if exitCode != 0 {
		t.Fatalf("read_scene exitCode = %d; stderr:\n%s", exitCode, stderr)
	}
	if !strings.Contains(stdout, `"name":"root"`) {
		t.Errorf("read_scene stdout = %q, want the root node payload", stdout)
	}
}
