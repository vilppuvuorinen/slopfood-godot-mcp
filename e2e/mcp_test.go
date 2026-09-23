// Package e2e is a standalone black-box test module. It knows nothing about
// the server's code: it only executes a pre-built binary and talks to it with
// the official MCP SDK, so the SDK never enters the server's dependency graph.
package e2e

import (
	"os"
	"strings"
	"testing"
)

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
