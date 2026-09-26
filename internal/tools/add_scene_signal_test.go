package tools

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/vilppuvuorinen/slopfood-godot-mcp/internal/godot"
)

func TestAddSceneSignalSchema(t *testing.T) {
	tool := addSceneSignal()
	if tool.Name != "add_scene_signal" {
		t.Errorf("name = %q", tool.Name)
	}
	if tool.Description != "Add a signal connection in a .tscn scene" {
		t.Errorf("description = %q", tool.Description)
	}
	if !hasRequired(tool.InputSchema, "scenePath", "signalName", "sourcePath", "targetPath", "method") {
		t.Errorf("required = %v", tool.InputSchema["required"])
	}
}

func TestAddSceneSignalSuccess(t *testing.T) {
	t.Setenv(godot.PathEnv, fakeGodotOps(t, `echo "$@"`))
	root := t.TempDir()
	makeProject(t, root)
	seedFile(t, filepath.Join(root, "scenes", "main.tscn"), "[gd_scene]\n")

	result := callToolArgs(t, addSceneSignal(), root, `{"scenePath":"scenes/main.tscn","signalName":"pressed","sourcePath":"root","targetPath":"root","method":"on_pressed"}`)
	if result.IsError {
		t.Fatalf("unexpected error: %+v", result.Content)
	}
	text := result.Content[0].Text
	if !strings.HasPrefix(text, "manage_scene_signals succeeded.") {
		t.Errorf("text = %q", text)
	}
	for _, want := range []string{`"action":"add"`, `"scenePath":"scenes/main.tscn"`, `"signalName":"pressed"`, `"sourcePath":"root"`, `"targetPath":"root"`, `"method":"on_pressed"`} {
		if !strings.Contains(text, want) {
			t.Errorf("text = %q, want it to contain %q", text, want)
		}
	}
}

func TestAddSceneSignalValidation(t *testing.T) {
	root := t.TempDir()
	makeProject(t, root)
	seedFile(t, filepath.Join(root, "scenes", "main.tscn"), "[gd_scene]\n")
	notAProject := t.TempDir()

	tests := []struct {
		name    string
		project string
		args    string
		want    string
	}{
		{name: "missing everything", args: `{}`, want: "scenePath, signalName, sourcePath, targetPath, and method are required."},
		{name: "missing method", project: root, args: `{"scenePath":"main.tscn","signalName":"pressed","sourcePath":"root","targetPath":"root"}`, want: "scenePath, signalName, sourcePath, targetPath, and method are required."},
		{name: "invalid project path", project: "../escape", args: `{"scenePath":"main.tscn","signalName":"pressed","sourcePath":"root","targetPath":"root","method":"on_pressed"}`, want: "Invalid path."},
		{name: "not a project", project: notAProject, args: `{"scenePath":"main.tscn","signalName":"pressed","sourcePath":"root","targetPath":"root","method":"on_pressed"}`, want: "Not a valid Godot project"},
		{name: "missing scene file", project: root, args: `{"scenePath":"missing.tscn","signalName":"pressed","sourcePath":"root","targetPath":"root","method":"on_pressed"}`, want: "Scene file does not exist: missing.tscn"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := callToolArgs(t, addSceneSignal(), tt.project, tt.args)
			if !result.IsError {
				t.Fatalf("expected an error result: %+v", result.Content)
			}
			if !strings.HasPrefix(result.Content[0].Text, tt.want) {
				t.Errorf("message = %q, want prefix %q", result.Content[0].Text, tt.want)
			}
		})
	}
}

func TestAddSceneSignalScriptFailure(t *testing.T) {
	t.Setenv(godot.PathEnv, fakeGodotOps(t, `echo 'Failed to connect signal' >&2; exit 1`))
	root := t.TempDir()
	makeProject(t, root)
	seedFile(t, filepath.Join(root, "scenes", "main.tscn"), "[gd_scene]\n")

	result := callToolArgs(t, addSceneSignal(), root, `{"scenePath":"scenes/main.tscn","signalName":"pressed","sourcePath":"root","targetPath":"root","method":"on_pressed"}`)
	if !result.IsError {
		t.Fatalf("expected an error result: %+v", result.Content)
	}
	if !strings.HasPrefix(result.Content[0].Text, "manage_scene_signals failed: ") {
		t.Errorf("message = %q", result.Content[0].Text)
	}
}
