package tools

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/vilppuvuorinen/slopfood-godot-mcp/internal/godot"
)

func TestMoveSceneNodeSchema(t *testing.T) {
	tool := moveSceneNode()
	if tool.Name != "move_scene_node" {
		t.Errorf("name = %q", tool.Name)
	}
	if tool.Description != "Move a node to a new parent in a .tscn scene" {
		t.Errorf("description = %q", tool.Description)
	}
	if !hasRequired(tool.InputSchema, "scenePath", "nodePath", "newParentPath") {
		t.Errorf("required = %v", tool.InputSchema["required"])
	}
}

func TestMoveSceneNodeSuccess(t *testing.T) {
	t.Setenv(godot.PathEnv, fakeGodotOps(t, `echo "$@"`))
	root := t.TempDir()
	makeProject(t, root)
	seedFile(t, filepath.Join(root, "scenes", "main.tscn"), "[gd_scene]\n")

	result := callToolArgs(t, moveSceneNode(), root, `{"scenePath":"scenes/main.tscn","nodePath":"root/Player","newParentPath":"root/World"}`)
	if result.IsError {
		t.Fatalf("unexpected error: %+v", result.Content)
	}
	text := result.Content[0].Text
	for _, want := range []string{`"action":"move"`, `"nodePath":"root/Player"`, `"newParentPath":"root/World"`} {
		if !strings.Contains(text, want) {
			t.Errorf("text = %q, want it to contain %q", text, want)
		}
	}
}

func TestMoveSceneNodeValidation(t *testing.T) {
	root := t.TempDir()
	makeProject(t, root)
	notAProject := t.TempDir()

	tests := []struct {
		name    string
		project string
		args    string
		want    string
	}{
		{name: "missing params", args: `{}`, want: "scenePath, nodePath, and newParentPath are required."},
		{name: "missing newParentPath", project: root, args: `{"scenePath":"main.tscn","nodePath":"root"}`, want: "scenePath, nodePath, and newParentPath are required."},
		{name: "invalid project path", project: "../escape", args: `{"scenePath":"main.tscn","nodePath":"root","newParentPath":"root/World"}`, want: "Invalid path."},
		{name: "not a project", project: notAProject, args: `{"scenePath":"main.tscn","nodePath":"root","newParentPath":"root/World"}`, want: "Not a valid Godot project"},
		{name: "missing scene file", project: root, args: `{"scenePath":"missing.tscn","nodePath":"root","newParentPath":"root/World"}`, want: "Scene file does not exist: missing.tscn"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := callToolArgs(t, moveSceneNode(), tt.project, tt.args)
			if !result.IsError {
				t.Fatalf("expected an error result: %+v", result.Content)
			}
			if !strings.HasPrefix(result.Content[0].Text, tt.want) {
				t.Errorf("message = %q, want prefix %q", result.Content[0].Text, tt.want)
			}
		})
	}
}
