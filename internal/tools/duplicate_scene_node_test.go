package tools

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/vilppuvuorinen/go-dot-mcp/internal/godot"
)

func TestDuplicateSceneNodeSchema(t *testing.T) {
	tool := duplicateSceneNode()
	if tool.Name != "duplicate_scene_node" {
		t.Errorf("name = %q", tool.Name)
	}
	if tool.Description != "Duplicate a node within a .tscn scene" {
		t.Errorf("description = %q", tool.Description)
	}
	if !hasRequired(tool.InputSchema, "scenePath", "nodePath") {
		t.Errorf("required = %v", tool.InputSchema["required"])
	}
}

func TestDuplicateSceneNodeSuccess(t *testing.T) {
	t.Setenv(godot.PathEnv, fakeGodotOps(t, `echo "$@"`))
	root := t.TempDir()
	makeProject(t, root)
	seedFile(t, filepath.Join(root, "scenes", "main.tscn"), "[gd_scene]\n")

	result := callToolArgs(t, duplicateSceneNode(), root, `{"scenePath":"scenes/main.tscn","nodePath":"root/Player"}`)
	if result.IsError {
		t.Fatalf("unexpected error: %+v", result.Content)
	}
	text := result.Content[0].Text
	for _, want := range []string{`"action":"duplicate"`, `"scenePath":"scenes/main.tscn"`, `"nodePath":"root/Player"`} {
		if !strings.Contains(text, want) {
			t.Errorf("text = %q, want it to contain %q", text, want)
		}
	}
}

func TestDuplicateSceneNodeValidation(t *testing.T) {
	root := t.TempDir()
	makeProject(t, root)
	notAProject := t.TempDir()

	tests := []struct {
		name    string
		project string
		args    string
		want    string
	}{
		{name: "missing params", args: `{}`, want: "scenePath and nodePath are required."},
		{name: "missing nodePath", project: root, args: `{"scenePath":"main.tscn"}`, want: "scenePath and nodePath are required."},
		{name: "invalid project path", project: "../escape", args: `{"scenePath":"main.tscn","nodePath":"root"}`, want: "Invalid path."},
		{name: "invalid scene path", project: root, args: `{"scenePath":"../escape.tscn","nodePath":"root"}`, want: "Invalid path."},
		{name: "not a project", project: notAProject, args: `{"scenePath":"main.tscn","nodePath":"root"}`, want: "Not a valid Godot project"},
		{name: "missing scene file", project: root, args: `{"scenePath":"missing.tscn","nodePath":"root"}`, want: "Scene file does not exist: missing.tscn"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := callToolArgs(t, duplicateSceneNode(), tt.project, tt.args)
			if !result.IsError {
				t.Fatalf("expected an error result: %+v", result.Content)
			}
			if !strings.HasPrefix(result.Content[0].Text, tt.want) {
				t.Errorf("message = %q, want prefix %q", result.Content[0].Text, tt.want)
			}
		})
	}
}
