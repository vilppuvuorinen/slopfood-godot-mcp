package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/vilppuvuorinen/slopfood-godot-mcp/internal/godot"
	"github.com/vilppuvuorinen/slopfood-godot-mcp/internal/godotops"
	"github.com/vilppuvuorinen/slopfood-godot-mcp/internal/mcp"
)

// makeProject creates dir with a minimal project.godot.
func makeProject(t *testing.T, dir string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", dir, err)
	}
	if err := os.WriteFile(filepath.Join(dir, "project.godot"), []byte("config_version=5\n"), 0o644); err != nil { //nolint:gosec // test fixture.
		t.Fatalf("write project.godot: %v", err)
	}
}

// seedFile writes content to path, creating parent directories as needed.
func seedFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil { //nolint:gosec // test fixture.
		t.Fatalf("write %s: %v", path, err)
	}
}

// oneShotSession is a test-only Session that reproduces the one-shot spawn the
// engine seam used before a warm session existed, so handler tests exercise the
// seam without a real engine. It keeps every fakeGodotOps shell fake working.
type oneShotSession struct{}

func (s *oneShotSession) Run(ctx context.Context, project, operation string, params map[string]any) (stdout, stderr string, exitCode int, err error) {
	scriptPath, err := godotops.OperationsScript()
	if err != nil {
		return "", "", 0, fmt.Errorf("materialize operations script: %w", err)
	}
	stdout, stderr, exitCode, err = godot.RunOperation(ctx, godot.Path(), project, scriptPath, operation, params)
	if err != nil {
		return stdout, stderr, exitCode, fmt.Errorf("run godot operation: %w", err)
	}
	return stdout, stderr, exitCode, nil
}

// callToolArgs invokes a tool handler with the bound project and raw JSON
// arguments and returns the ToolResult, failing on a transport-style error.
//
// The bound project is not a tool argument; it is lifted into the handler
// context, as Register does in production. An empty project leaves the project
// unbound. The context always carries the one-shot session double, so
// engine-reaching handlers exercise the seam.
func callToolArgs(t *testing.T, tool mcp.Tool, project, args string) mcp.ToolResult {
	t.Helper()
	ctx := context.Background()
	if project != "" {
		ctx = withProject(ctx, project)
	}
	ctx = WithEngine(ctx, &oneShotSession{})
	out, err := tool.Handler(ctx, json.RawMessage(args))
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	result, ok := out.(mcp.ToolResult)
	if !ok {
		t.Fatalf("result type %T, want mcp.ToolResult", out)
	}
	return result
}

// jsonString quotes a string for embedding in a JSON test argument.
func jsonString(s string) string {
	data, err := json.Marshal(s)
	if err != nil {
		panic(err)
	}
	return string(data)
}
