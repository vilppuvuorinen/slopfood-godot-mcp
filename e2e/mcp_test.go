// Package e2e is a standalone black-box test module. It knows nothing about
// the server's code: it only executes a pre-built binary and talks to it with
// the official MCP SDK, so the SDK never enters the server's dependency graph.
package e2e

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func TestHelloWorldTool(t *testing.T) {
	session := connect(t)
	findTool(t, session, "hello")

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
			res := callTool(t, session, "hello", tt.args)
			if res.IsError {
				t.Fatalf("tool reported an error: %+v", res.Content)
			}
			if len(res.Content) != 1 {
				t.Fatalf("content = %+v", res.Content)
			}
			var got struct {
				Message string `json:"message"`
			}
			if err := json.Unmarshal([]byte(firstText(t, res)), &got); err != nil {
				t.Fatalf("unmarshal %q: %v", firstText(t, res), err)
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
	session := connect(t)
	findTool(t, session, "get_godot_version")

	res := callTool(t, session, "get_godot_version", nil)
	if res.IsError {
		t.Skipf("Godot unavailable, skipping: %+v", res.Content)
	}
	if len(res.Content) != 1 {
		t.Fatalf("content = %+v", res.Content)
	}
	if got := firstText(t, res); !strings.HasPrefix(got, "4.7.") {
		t.Errorf("version = %q, want a 4.7.x build (GODOT_PATH=%q)", got, os.Getenv("GODOT_PATH"))
	}
}
