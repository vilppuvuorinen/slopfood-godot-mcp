// Package e2e is a standalone black-box test module. It knows nothing about
// the server's code: it only executes a pre-built binary and talks to it with
// the official MCP SDK, so the SDK never enters the server's dependency graph.
package e2e

import (
	"os"
	"strings"
	"testing"
)

// defaultGodotMinor mirrors the server's godot.DefaultMinor: the build used
// when GODOT_PATH is unset. It is duplicated here because this black-box module
// deliberately does not import the server.
const defaultGodotMinor = "4.7"

// TestGetGodotVersionTool drives the real get_godot_version tool against the
// configured Godot build. The expected minor line comes from GODOT_MINOR (CI
// sets it per matrix entry) and defaults to the server's local build, so the
// suite passes against any supported release.
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
	minor := os.Getenv("GODOT_MINOR")
	if minor == "" {
		minor = defaultGodotMinor
	}
	if got := firstText(t, res); !strings.HasPrefix(got, minor+".") {
		t.Errorf("version = %q, want a %s.x build (GODOT_PATH=%q)", got, minor, os.Getenv("GODOT_PATH"))
	}
}
