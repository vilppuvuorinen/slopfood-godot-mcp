// Package e2e is a standalone black-box test module: it only executes a pre-built binary via the official MCP SDK.
package e2e

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"testing"
)

// Mirrors the server's godot.DefaultMinor: the build used when GODOT_PATH is unset.
const defaultGodotMinor = "4.7"

// With GODOT_PATH set the tool must echo that engine's own --version; otherwise expect GODOT_MINOR or the server's default minor.
func TestGetGodotVersionTool(t *testing.T) {
	t.Parallel()
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

func godotVersion(t *testing.T, path string) string {
	t.Helper()
	out, err := exec.CommandContext(context.Background(), path, "--version").Output() //nolint:gosec // the path comes from the test environment.
	if err != nil {
		t.Fatalf("run %s --version: %v", path, err)
	}
	return strings.TrimSpace(string(out))
}

func TestProtocolVersionNegotiationEchoesSupported(t *testing.T) {
	t.Parallel()
	session := connectWithProtocolVersion(t, "2025-06-18")
	if got := session.InitializeResult().ProtocolVersion; got != "2025-06-18" {
		t.Errorf("protocolVersion = %q, want 2025-06-18", got)
	}
}

// A 2026-07-28 client probes server/discover first and negotiates the stateless revision.
func TestProtocolVersionNegotiationModern(t *testing.T) {
	t.Parallel()
	session := connectWithProtocolVersion(t, "2026-07-28")
	if got := session.InitializeResult().ProtocolVersion; got != "2026-07-28" {
		t.Errorf("protocolVersion = %q, want 2026-07-28", got)
	}
}

// An unknown future revision is rejected with the supported list and the client retries at 2026-07-28.
func TestProtocolVersionNegotiationUnknownModern(t *testing.T) {
	t.Parallel()
	session := connectWithProtocolVersion(t, "2099-01-01")
	if got := session.InitializeResult().ProtocolVersion; got != "2026-07-28" {
		t.Errorf("protocolVersion = %q, want 2026-07-28", got)
	}
}

// Unknown legacy versions skip discovery and negotiate the newest legacy revision, never a stateless one.
func TestProtocolVersionNegotiationLegacyFallback(t *testing.T) {
	t.Parallel()
	session := connectWithProtocolVersion(t, "1999-01-01")
	if got := session.InitializeResult().ProtocolVersion; got != "2025-11-25" {
		t.Errorf("protocolVersion = %q, want 2025-11-25", got)
	}
}

func TestInitializeInstructions(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	session := connectProject(t, root)
	instructions := session.InitializeResult().Instructions
	if instructions == "" {
		t.Error("instructions is empty")
	}
	requireContains(t, instructions, root)
}

func TestToolMetadata(t *testing.T) {
	t.Parallel()
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

func TestStructuredContent(t *testing.T) {
	t.Parallel()
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

func TestMutationToolMetadata(t *testing.T) {
	t.Parallel()
	tool := findTool(t, connect(t), "create_scene")
	if tool.OutputSchema == nil {
		t.Error("outputSchema missing")
	}
	if tool.Annotations == nil || tool.Annotations.ReadOnlyHint {
		t.Errorf("annotations = %+v", tool.Annotations)
	}
}

func TestMutationStructuredContent(t *testing.T) {
	t.Parallel()
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
