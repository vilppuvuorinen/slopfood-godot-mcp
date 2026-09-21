package tools

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vilppuvuorinen/go-dot-mcp/internal/godot"
	"github.com/vilppuvuorinen/go-dot-mcp/internal/mcp"
)

func fakeGodot(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "godot")
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"+body+"\n"), 0o755); err != nil { //nolint:gosec // test helper needs an executable script.
		t.Fatalf("write fake godot: %v", err)
	}
	return path
}

func callTool(t *testing.T, tool mcp.Tool) mcp.ToolResult {
	t.Helper()
	out, err := tool.Handler(context.Background(), nil)
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	result, ok := out.(mcp.ToolResult)
	if !ok {
		t.Fatalf("result type %T, want mcp.ToolResult", out)
	}
	return result
}

func TestGetGodotVersionSchema(t *testing.T) {
	tool := getGodotVersion()
	if tool.Name != "get_godot_version" {
		t.Errorf("name = %q", tool.Name)
	}
	if tool.InputSchema["type"] != "object" {
		t.Errorf("schema type = %v", tool.InputSchema["type"])
	}
}

func TestGetGodotVersionSuccess(t *testing.T) {
	t.Setenv(godot.PathEnv, fakeGodot(t, "echo 4.7.2.stable.official.fake"))

	result := callTool(t, getGodotVersion())
	if result.IsError {
		t.Fatalf("unexpected error result: %+v", result.Content)
	}
	if len(result.Content) != 1 {
		t.Fatalf("content = %+v", result.Content)
	}
	if got := result.Content[0].Text; got != "4.7.2.stable.official.fake" {
		t.Fatalf("text = %q", got)
	}
}

func TestGetGodotVersionFailure(t *testing.T) {
	t.Setenv(godot.PathEnv, filepath.Join(t.TempDir(), "missing-godot"))

	result := callTool(t, getGodotVersion())
	if !result.IsError {
		t.Fatalf("expected an error result: %+v", result.Content)
	}
	if len(result.Content) != 2 {
		t.Fatalf("content = %+v", result.Content)
	}
	if !strings.Contains(result.Content[0].Text, "Failed to get Godot version") {
		t.Errorf("message = %q", result.Content[0].Text)
	}
	if !strings.HasPrefix(result.Content[1].Text, "Possible solutions:") {
		t.Errorf("solutions = %q", result.Content[1].Text)
	}
}
