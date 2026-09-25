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

// callToolArgs invokes a tool handler with raw JSON arguments and returns the
// ToolResult, failing on a transport-style error.
//
// The bound project is not a tool argument. Tests still name it with the former
// "projectPath" key; this helper lifts that value into the handler context, as
// Register does in production, and passes the remaining arguments through.
func callToolArgs(t *testing.T, tool mcp.Tool, args string) mcp.ToolResult {
	t.Helper()
	raw, project := splitProjectPath([]byte(args))
	ctx := context.Background()
	if project != "" {
		ctx = withProject(ctx, project)
	}
	out, err := tool.Handler(ctx, raw)
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	result, ok := out.(mcp.ToolResult)
	if !ok {
		t.Fatalf("result type %T, want mcp.ToolResult", out)
	}
	return result
}

// splitProjectPath separates a test-supplied "projectPath" from the other tool
// arguments. It returns the arguments without that key and the project value.
func splitProjectPath(args []byte) (json.RawMessage, string) {
	if len(args) == 0 {
		return nil, ""
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(args, &fields); err != nil {
		return json.RawMessage(args), ""
	}
	rawProject, ok := fields["projectPath"]
	if !ok {
		return json.RawMessage(args), ""
	}
	var project string
	if err := json.Unmarshal(rawProject, &project); err != nil {
		return json.RawMessage(args), ""
	}
	delete(fields, "projectPath")
	rest, err := json.Marshal(fields)
	if err != nil {
		return json.RawMessage(args), project
	}
	return rest, project
}

// jsonString quotes a string for embedding in a JSON test argument.
func jsonString(s string) string {
	data, err := json.Marshal(s)
	if err != nil {
		panic(err)
	}
	return string(data)
}
