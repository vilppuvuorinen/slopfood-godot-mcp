package tools

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/vilppuvuorinen/slopfood-godot-mcp/internal/godot"
)

func TestExportProjectSchema(t *testing.T) {
	tool := newToolset("", nil).exportProject()
	if tool.Name != "export_project" {
		t.Errorf("name = %q", tool.Name)
	}
	if tool.Description != "Export a Godot project using a preset from export_presets.cfg, overwriting the output file if it exists" {
		t.Errorf("description = %q", tool.Description)
	}
	required, ok := tool.InputSchema["required"].([]string)
	if !ok || strings.Join(required, ",") != "presetName,outputPath" {
		t.Errorf("required = %v", tool.InputSchema["required"])
	}
	props, ok := tool.InputSchema["properties"].(map[string]any)
	if !ok {
		t.Fatalf("properties = %T", tool.InputSchema["properties"])
	}
	for _, key := range []string{"presetName", "outputPath"} {
		prop, isString := props[key].(map[string]any)
		if !isString || prop["type"] != "string" {
			t.Errorf("%s property = %v", key, props[key])
		}
	}
	debug, ok := props["debug"].(map[string]any)
	if !ok || debug["type"] != "boolean" {
		t.Errorf("debug property = %v", props["debug"])
	}
}

func TestExportProjectSuccess(t *testing.T) {
	t.Setenv(godot.PathEnv, fakeGodotOps(t, `echo "$@"`))
	root := t.TempDir()
	makeProject(t, root)

	result := callToolArgs(t, (*toolset).exportProject, root, `{"presetName":"Linux","outputPath":"build/game.x86_64"}`)
	if result.IsError {
		t.Fatalf("unexpected error: %+v", result.Content)
	}
	text := result.Content[0].Text
	if !strings.HasPrefix(text, "Export succeeded.\n\nOutput: ") {
		t.Errorf("text = %q", text)
	}
	for _, want := range []string{"--headless", "--export-release", "Linux", filepath.Join(root, "build/game.x86_64")} {
		if !strings.Contains(text, want) {
			t.Errorf("text = %q, want it to contain %q", text, want)
		}
	}
	structured, ok := result.StructuredContent.(map[string]any)
	if !ok {
		t.Fatalf("structuredContent = %T, want object", result.StructuredContent)
	}
	if got := structured["outputPath"]; got != "build/game.x86_64" {
		t.Errorf("outputPath = %v, want the caller's relative path", got)
	}
	if got, want := structured["resolvedOutputPath"], filepath.Join(root, "build/game.x86_64"); got != want {
		t.Errorf("resolvedOutputPath = %v, want %q", got, want)
	}
}

func TestExportProjectAbsoluteOutputPath(t *testing.T) {
	t.Setenv(godot.PathEnv, fakeGodotOps(t, `echo "$@"`))
	root := t.TempDir()
	makeProject(t, root)
	out := filepath.Join(t.TempDir(), "game.x86_64")

	result := callToolArgs(t, (*toolset).exportProject, root, `{"presetName":"Linux","outputPath":`+jsonString(out)+`}`)
	if result.IsError {
		t.Fatalf("unexpected error: %+v", result.Content)
	}
	if text := result.Content[0].Text; !strings.Contains(text, out) {
		t.Errorf("text = %q, want it to contain %q", text, out)
	}
}

func TestExportProjectDebug(t *testing.T) {
	t.Setenv(godot.PathEnv, fakeGodotOps(t, `echo "$@"`))
	root := t.TempDir()
	makeProject(t, root)

	result := callToolArgs(t, (*toolset).exportProject, root, `{"presetName":"Linux","outputPath":"build/game.x86_64","debug":true}`)
	if result.IsError {
		t.Fatalf("unexpected error: %+v", result.Content)
	}
	if text := result.Content[0].Text; !strings.Contains(text, "--export-debug") || strings.Contains(text, "--export-release") {
		t.Errorf("text = %q, want --export-debug only", text)
	}
}

func TestExportProjectUsesOutputPathWhenStdoutEmpty(t *testing.T) {
	t.Setenv(godot.PathEnv, fakeGodotOps(t, `exit 0`))
	root := t.TempDir()
	makeProject(t, root)

	result := callToolArgs(t, (*toolset).exportProject, root, `{"presetName":"Linux","outputPath":"build/game.x86_64"}`)
	if result.IsError {
		t.Fatalf("unexpected error: %+v", result.Content)
	}
	if got, want := result.Content[0].Text, "Export succeeded.\n\nOutput: "+filepath.Join(root, "build/game.x86_64"); got != want {
		t.Errorf("text = %q, want %q", got, want)
	}
}

func TestExportProjectNonZeroExit(t *testing.T) {
	t.Setenv(godot.PathEnv, fakeGodotOps(t, `echo 'could not pack project' >&2; exit 1`))
	root := t.TempDir()
	makeProject(t, root)

	result := callToolArgs(t, (*toolset).exportProject, root, `{"presetName":"Linux","outputPath":"build/game.x86_64"}`)
	if !result.IsError {
		t.Fatalf("expected an error result: %+v", result.Content)
	}
	if got := result.Content[0].Text; !strings.HasPrefix(got, "Export failed: ") || !strings.Contains(got, "could not pack project") {
		t.Errorf("message = %q", got)
	}
}

func TestExportProjectUnexpectedStderrFails(t *testing.T) {
	t.Setenv(godot.PathEnv, fakeGodotOps(t, `echo 'ERROR: harmless diagnostic' >&2; exit 0`))
	root := t.TempDir()
	makeProject(t, root)

	result := callToolArgs(t, (*toolset).exportProject, root, `{"presetName":"Linux","outputPath":"build/game.x86_64"}`)
	if !result.IsError {
		t.Fatalf("unexpected stderr must fail even on exit 0: %+v", result.Content)
	}
	if got := result.Content[0].Text; !strings.HasPrefix(got, "Export failed: ") || !strings.Contains(got, "harmless diagnostic") {
		t.Errorf("message = %q", got)
	}
}

func TestExportProjectSpawnFailure(t *testing.T) {
	t.Setenv(godot.PathEnv, filepath.Join(t.TempDir(), "missing-godot"))
	root := t.TempDir()
	makeProject(t, root)

	result := callToolArgs(t, (*toolset).exportProject, root, `{"presetName":"Linux","outputPath":"build/game.x86_64"}`)
	if !result.IsError {
		t.Fatalf("expected an error result: %+v", result.Content)
	}
	if got := result.Content[0].Text; !strings.HasPrefix(got, "Export failed: ") {
		t.Errorf("message = %q", got)
	}
}

func TestExportProjectValidation(t *testing.T) {
	root := t.TempDir()
	makeProject(t, root)
	notAProject := t.TempDir()

	tests := []struct {
		name    string
		project string
		args    string
		want    string
	}{
		{name: "missing params", args: `{}`, want: "presetName and outputPath are required."},
		{name: "missing outputPath", project: root, args: `{"presetName":"Linux"}`, want: "presetName and outputPath are required."},
		{name: "invalid path", project: "../escape", args: `{"presetName":"Linux","outputPath":"out"}`, want: "Invalid path."},
		{name: "invalid output path", project: root, args: `{"presetName":"Linux","outputPath":"../escape"}`, want: "Path escapes the project root."},
		{name: "not a project", project: notAProject, args: `{"presetName":"Linux","outputPath":"out"}`, want: "Not a valid Godot project: "},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := callToolArgs(t, (*toolset).exportProject, tt.project, tt.args)
			if !result.IsError {
				t.Fatalf("expected an error result: %+v", result.Content)
			}
			if got := result.Content[0].Text; !strings.HasPrefix(got, tt.want) {
				t.Errorf("message = %q, want prefix %q", got, tt.want)
			}
		})
	}
}
