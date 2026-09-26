package tools

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/vilppuvuorinen/slopfood-godot-mcp/internal/godot"
)

func TestRemoveSceneSignalSchema(t *testing.T) {
	tool := removeSceneSignal()
	if tool.Name != "remove_scene_signal" {
		t.Errorf("name = %q", tool.Name)
	}
	if tool.Description != "Remove a signal connection from a .tscn scene" {
		t.Errorf("description = %q", tool.Description)
	}
	if !hasRequired(tool.InputSchema, "scenePath", "signalName", "sourcePath", "targetPath") {
		t.Errorf("required = %v", tool.InputSchema["required"])
	}
}

func TestRemoveSceneSignalSuccess(t *testing.T) {
	t.Setenv(godot.PathEnv, fakeGodotOps(t, `echo "$@"`))
	root := t.TempDir()
	makeProject(t, root)
	seedFile(t, filepath.Join(root, "scenes", "main.tscn"), "[gd_scene]\n")

	result := callToolArgs(t, removeSceneSignal(), root, `{"scenePath":"scenes/main.tscn","signalName":"released","sourcePath":"root","targetPath":"root/Player"}`)
	if result.IsError {
		t.Fatalf("unexpected error: %+v", result.Content)
	}
	text := result.Content[0].Text
	for _, want := range []string{`"action":"remove"`, `"signalName":"released"`, `"sourcePath":"root"`, `"targetPath":"root/Player"`} {
		if !strings.Contains(text, want) {
			t.Errorf("text = %q, want it to contain %q", text, want)
		}
	}
	if strings.Contains(text, `"method"`) {
		t.Errorf("text = %q, want it to omit an unset method", text)
	}
}

func TestRemoveSceneSignalForwardsMethodFilter(t *testing.T) {
	t.Setenv(godot.PathEnv, fakeGodotOps(t, `echo "$@"`))
	root := t.TempDir()
	makeProject(t, root)
	seedFile(t, filepath.Join(root, "scenes", "main.tscn"), "[gd_scene]\n")

	result := callToolArgs(t, removeSceneSignal(), root, `{"scenePath":"scenes/main.tscn","signalName":"released","sourcePath":"root","targetPath":"root/Player","method":"on_released"}`)
	if result.IsError {
		t.Fatalf("unexpected error: %+v", result.Content)
	}
	if !strings.Contains(result.Content[0].Text, `"method":"on_released"`) {
		t.Errorf("text = %q, want method forwarded", result.Content[0].Text)
	}
}

func TestRemoveSceneSignalValidation(t *testing.T) {
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
		{name: "missing everything", args: `{}`, want: "scenePath, signalName, sourcePath, and targetPath are required."},
		{name: "missing signal", project: root, args: `{"scenePath":"main.tscn","sourcePath":"root","targetPath":"root"}`, want: "scenePath, signalName, sourcePath, and targetPath are required."},
		{name: "missing target", project: root, args: `{"scenePath":"main.tscn","signalName":"released","sourcePath":"root"}`, want: "scenePath, signalName, sourcePath, and targetPath are required."},
		{name: "invalid project path", project: "../escape", args: `{"scenePath":"main.tscn","signalName":"released","sourcePath":"root","targetPath":"root"}`, want: "Invalid path."},
		{name: "not a project", project: notAProject, args: `{"scenePath":"main.tscn","signalName":"released","sourcePath":"root","targetPath":"root"}`, want: "Not a valid Godot project"},
		{name: "missing scene file", project: root, args: `{"scenePath":"missing.tscn","signalName":"released","sourcePath":"root","targetPath":"root"}`, want: "Scene file does not exist: missing.tscn"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := callToolArgs(t, removeSceneSignal(), tt.project, tt.args)
			if !result.IsError {
				t.Fatalf("expected an error result: %+v", result.Content)
			}
			if !strings.HasPrefix(result.Content[0].Text, tt.want) {
				t.Errorf("message = %q, want prefix %q", result.Content[0].Text, tt.want)
			}
		})
	}
}
