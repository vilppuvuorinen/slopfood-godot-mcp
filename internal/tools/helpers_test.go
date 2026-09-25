package tools

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/vilppuvuorinen/go-dot-mcp/internal/mcp"
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

// callToolArgs invokes a tool handler with the bound project and raw JSON
// arguments and returns the ToolResult, failing on a transport-style error.
//
// The bound project is not a tool argument; it is lifted into the handler
// context, as Register does in production. An empty project leaves the context
// unbound.
func callToolArgs(t *testing.T, tool mcp.Tool, project, args string) mcp.ToolResult {
	t.Helper()
	ctx := context.Background()
	if project != "" {
		ctx = withProject(ctx, project)
	}
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
