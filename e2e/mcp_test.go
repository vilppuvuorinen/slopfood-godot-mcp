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

// TestProtocolVersionNegotiationEchoesSupported pins a supported protocol
// version and asserts the server echoes it back.
func TestProtocolVersionNegotiationEchoesSupported(t *testing.T) {
	session := connectWithProtocolVersion(t, "2025-06-18")
	if got := session.InitializeResult().ProtocolVersion; got != "2025-06-18" {
		t.Errorf("protocolVersion = %q, want 2025-06-18", got)
	}
}

// TestProtocolVersionNegotiationFallsBack connects as a modern client, which
// probes server/discover first. The server does not implement it, so the client
// falls back to the legacy handshake and the server offers its newest revision.
func TestProtocolVersionNegotiationFallsBack(t *testing.T) {
	session := connectWithProtocolVersion(t, "2026-07-28")
	if got := session.InitializeResult().ProtocolVersion; got != "2025-11-25" {
		t.Errorf("protocolVersion = %q, want 2025-11-25", got)
	}
}

// TestInitializeInstructions asserts the server advertises usage instructions.
func TestInitializeInstructions(t *testing.T) {
	session := connect(t)
	if session.InitializeResult().Instructions == "" {
		t.Error("instructions is empty")
	}
}

// TestToolMetadata asserts a listed tool carries a title, annotations, and an
// output schema.
func TestToolMetadata(t *testing.T) {
	tool := findTool(t, connect(t), "get_godot_version")
	if tool.Title != "Get Godot Version" {
		t.Errorf("title = %q", tool.Title)
	}
	if tool.Annotations == nil || !tool.Annotations.ReadOnlyHint {
		t.Errorf("annotations = %+v", tool.Annotations)
	}
	if tool.OutputSchema == nil {
		t.Error("outputSchema missing")
	}
}

// TestStructuredContent asserts a read-only tool returns structuredContent
// alongside its text block.
func TestStructuredContent(t *testing.T) {
	session := connect(t)
	root := t.TempDir()
	seedProject(t, root)

	res := callTool(t, session, "list_projects", map[string]any{"directory": root})
	if res.IsError {
		t.Fatalf("list_projects error: %s", firstText(t, res))
	}
	projects, ok := res.StructuredContent.([]any)
	if !ok {
		t.Fatalf("structuredContent type %T", res.StructuredContent)
	}
	if len(projects) != 1 {
		t.Fatalf("projects = %+v", projects)
	}
}

// TestMutationToolMetadata asserts a mutation tool advertises an output schema
// and non-read-only annotations.
func TestMutationToolMetadata(t *testing.T) {
	tool := findTool(t, connect(t), "set_main_scene")
	if tool.OutputSchema == nil {
		t.Error("outputSchema missing")
	}
	if tool.Annotations == nil || tool.Annotations.ReadOnlyHint {
		t.Errorf("annotations = %+v", tool.Annotations)
	}
}

// TestMutationStructuredContent asserts a mutation tool returns
// structuredContent alongside its text block.
func TestMutationStructuredContent(t *testing.T) {
	session := connect(t)
	root := t.TempDir()
	seedProject(t, root)

	res := callTool(t, session, "set_main_scene", map[string]any{
		"projectPath": root,
		"scenePath":   "scenes/main.tscn",
	})
	if res.IsError {
		t.Fatalf("set_main_scene error: %s", firstText(t, res))
	}
	structured, ok := res.StructuredContent.(map[string]any)
	if !ok {
		t.Fatalf("structuredContent type %T", res.StructuredContent)
	}
	if structured["scenePath"] != "res://scenes/main.tscn" {
		t.Errorf("scenePath = %v", structured["scenePath"])
	}
}
