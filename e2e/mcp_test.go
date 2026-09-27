// Package e2e is a standalone black-box test module. It knows nothing about
// the server's code: it only executes a pre-built binary and talks to it with
// the official MCP SDK, so the SDK never enters the server's dependency graph.
package e2e

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"testing"
)

// defaultGodotMinor mirrors the server's godot.DefaultMinor: the build used
// when GODOT_PATH is unset. It is duplicated here because this black-box module
// deliberately does not import the server.
const defaultGodotMinor = "4.7"

// TestGetGodotVersionTool drives the real get_godot_version tool against the
// configured Godot build. When GODOT_PATH names the engine the server will use,
// the tool must echo that engine's own `--version`, so the suite passes against
// any installed release rather than only the server's default minor. Otherwise
// the expected minor line comes from GODOT_MINOR (CI sets it per matrix entry)
// and defaults to the server's local build.
func TestGetGodotVersionTool(t *testing.T) {
	session := connect(t)
	findTool(t, session, "get_godot_version")

	res := callTool(t, session, "get_godot_version", nil)
	if res.IsError {
		t.Fatalf("Godot unavailable: %+v", res.Content)
	}
	if len(res.Content) != 1 {
		t.Fatalf("content = %+v", res.Content)
	}
	got := firstText(t, res)
	if path := os.Getenv("GODOT_PATH"); path != "" {
		if want := godotVersion(t, path); got != want {
			t.Errorf("version = %q, want %q (GODOT_PATH=%q)", got, want, path)
		}
		return
	}
	minor := os.Getenv("GODOT_MINOR")
	if minor == "" {
		minor = defaultGodotMinor
	}
	if !strings.HasPrefix(got, minor+".") {
		t.Errorf("version = %q, want a %s.x build (GODOT_PATH=%q)", got, minor, os.Getenv("GODOT_PATH"))
	}
}

// godotVersion returns the trimmed `--version` output of the engine at path.
func godotVersion(t *testing.T, path string) string {
	t.Helper()
	out, err := exec.CommandContext(context.Background(), path, "--version").Output() //nolint:gosec // the path comes from the test environment.
	if err != nil {
		t.Fatalf("run %s --version: %v", path, err)
	}
	return strings.TrimSpace(string(out))
}

// TestProtocolVersionNegotiationEchoesSupported pins a supported protocol
// version and asserts the server echoes it back.
func TestProtocolVersionNegotiationEchoesSupported(t *testing.T) {
	session := connectWithProtocolVersion(t, "2025-06-18")
	if got := session.InitializeResult().ProtocolVersion; got != "2025-06-18" {
		t.Errorf("protocolVersion = %q, want 2025-06-18", got)
	}
}

// TestProtocolVersionNegotiationModern connects as a 2026-07-28 client, which
// probes server/discover first. The server implements it and advertises
// 2026-07-28, so the session negotiates the stateless revision.
func TestProtocolVersionNegotiationModern(t *testing.T) {
	session := connectWithProtocolVersion(t, "2026-07-28")
	if got := session.InitializeResult().ProtocolVersion; got != "2026-07-28" {
		t.Errorf("protocolVersion = %q, want 2026-07-28", got)
	}
}

// TestProtocolVersionNegotiationUnknownModern connects as a client offering a
// future stateless revision. server/discover rejects it with the supported
// list, and the client retries at 2026-07-28.
func TestProtocolVersionNegotiationUnknownModern(t *testing.T) {
	session := connectWithProtocolVersion(t, "2099-01-01")
	if got := session.InitializeResult().ProtocolVersion; got != "2026-07-28" {
		t.Errorf("protocolVersion = %q, want 2026-07-28", got)
	}
}

// TestProtocolVersionNegotiationLegacyFallback connects with an unknown legacy
// version. Discovery is skipped, and the legacy initialize handshake offers the
// newest legacy revision, never a stateless one.
func TestProtocolVersionNegotiationLegacyFallback(t *testing.T) {
	session := connectWithProtocolVersion(t, "1999-01-01")
	if got := session.InitializeResult().ProtocolVersion; got != "2025-11-25" {
		t.Errorf("protocolVersion = %q, want 2025-11-25", got)
	}
}

// TestInitializeInstructions asserts the server advertises usage instructions
// naming the project it is bound to.
func TestInitializeInstructions(t *testing.T) {
	root := t.TempDir()
	session := connectProject(t, root)
	instructions := session.InitializeResult().Instructions
	if instructions == "" {
		t.Error("instructions is empty")
	}
	requireContains(t, instructions, root)
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
	root := t.TempDir()
	seedProject(t, root)
	session := connectProject(t, root)

	res := callTool(t, session, "validate_project", map[string]any{})
	if res.IsError {
		t.Fatalf("validate_project error: %s", firstText(t, res))
	}
	structured, ok := res.StructuredContent.(map[string]any)
	if !ok {
		t.Fatalf("structuredContent type %T", res.StructuredContent)
	}
	if _, ok := structured["valid"].(bool); !ok {
		t.Fatalf("valid type %T", structured["valid"])
	}
}

// TestMutationToolMetadata asserts a mutation tool advertises an output schema
// and non-read-only annotations.
func TestMutationToolMetadata(t *testing.T) {
	tool := findTool(t, connect(t), "create_scene")
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
	root := newProject(t)
	session := connectProject(t, root)

	res := callTool(t, session, "create_scene", map[string]any{
		"scenePath":    "level.tscn",
		"rootNodeType": "Node2D",
	})
	if res.IsError {
		t.Fatalf("create_scene error: %s", firstText(t, res))
	}
	structured, ok := res.StructuredContent.(map[string]any)
	if !ok {
		t.Fatalf("structuredContent type %T", res.StructuredContent)
	}
	if structured["scenePath"] != "level.tscn" {
		t.Errorf("scenePath = %v", structured["scenePath"])
	}
}
