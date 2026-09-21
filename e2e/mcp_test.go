// Package e2e is a standalone black-box test module. It knows nothing about
// the server's code: it only executes a pre-built binary and talks to it with
// the official MCP SDK, so the SDK never enters the server's dependency graph.
package e2e

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// binaryEnv is the path to the server binary under test. Build it before
// running these tests; the test itself never builds anything.
const binaryEnv = "GO_DOT_MCP_BIN"

func connect(t *testing.T) *mcp.ClientSession {
	t.Helper()
	return connectWithEnv(t)
}

func TestHelloWorldTool(t *testing.T) {
	ctx := context.Background()
	session := connect(t)

	tools, err := session.ListTools(ctx, nil)
	if err != nil {
		t.Fatalf("list tools: %v", err)
	}
	var listed bool
	for _, tool := range tools.Tools {
		if tool.Name == "hello" {
			listed = true
			break
		}
	}
	if !listed {
		t.Fatalf("hello tool not listed: %+v", tools.Tools)
	}

	tests := []struct {
		name string
		args map[string]any
		want string
	}{
		{name: "default", want: "Hello, World!"},
		{name: "named", args: map[string]any{"name": "MCP"}, want: "Hello, MCP!"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "hello", Arguments: tt.args})
			if err != nil {
				t.Fatalf("call tool: %v", err)
			}
			if res.IsError {
				t.Fatalf("tool reported an error: %+v", res.Content)
			}
			if len(res.Content) != 1 {
				t.Fatalf("content = %+v", res.Content)
			}
			text, ok := res.Content[0].(*mcp.TextContent)
			if !ok {
				t.Fatalf("content type = %T", res.Content[0])
			}
			var got struct {
				Message string `json:"message"`
			}
			if err := json.Unmarshal([]byte(text.Text), &got); err != nil {
				t.Fatalf("unmarshal %q: %v", text.Text, err)
			}
			if got.Message != tt.want {
				t.Errorf("message = %q, want %q", got.Message, tt.want)
			}
		})
	}
}

// TestGetGodotVersionTool drives the real get_godot_version tool against the
// repository-pinned Godot build. The suite is expected to run with Godot 4.7
// (the server's default); pointing GODOT_PATH at another minor makes this test
// fail on purpose.
func TestGetGodotVersionTool(t *testing.T) {
	ctx := context.Background()
	session := connect(t)

	tools, err := session.ListTools(ctx, nil)
	if err != nil {
		t.Fatalf("list tools: %v", err)
	}
	var listed bool
	for _, tool := range tools.Tools {
		if tool.Name == "get_godot_version" {
			listed = true
			break
		}
	}
	if !listed {
		t.Fatalf("get_godot_version tool not listed: %+v", tools.Tools)
	}

	res, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "get_godot_version"})
	if err != nil {
		t.Fatalf("call tool: %v", err)
	}
	if res.IsError {
		t.Skipf("Godot unavailable, skipping: %+v", res.Content)
	}
	if len(res.Content) != 1 {
		t.Fatalf("content = %+v", res.Content)
	}
	text, ok := res.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatalf("content type = %T", res.Content[0])
	}
	if !strings.HasPrefix(text.Text, "4.7.") {
		t.Errorf("version = %q, want a 4.7.x build (GODOT_PATH=%q)", text.Text, os.Getenv("GODOT_PATH"))
	}
}
