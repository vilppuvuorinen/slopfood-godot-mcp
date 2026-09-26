package tools

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/vilppuvuorinen/go-dot-mcp/internal/godot"
)

func TestRenameSceneNodeSchema(t *testing.T) {
	tool := renameSceneNode()
	if tool.Name != "rename_scene_node" {
		t.Errorf("name = %q", tool.Name)
	}
	if tool.Description != "Rename a node in a .tscn scene" {
		t.Errorf("description = %q", tool.Description)
	}
	required, ok := tool.InputSchema["required"].([]string)
	if !ok || strings.Join(required, ",") != "scenePath,nodePath,newName" {
		t.Errorf("required = %v", tool.InputSchema["required"])
	}
	props, ok := tool.InputSchema["properties"].(map[string]any)
	if !ok {
		t.Fatalf("properties = %T", tool.InputSchema["properties"])
	}
	for _, key := range []string{"scenePath", "nodePath", "newName"} {
		prop, ok := props[key].(map[string]any)
		if !ok || prop["type"] != "string" {
			t.Errorf("property %q = %v, want string", key, props[key])
		}
	}
}

func TestRenameSceneNodeSuccess(t *testing.T) {
	t.Setenv(godot.PathEnv, fakeGodotOps(t, `echo "$@"`))
	root := t.TempDir()
	makeProject(t, root)
	seedFile(t, filepath.Join(root, "scenes", "main.tscn"), "[gd_scene]\n")

	result := callToolArgs(t, renameSceneNode(), root, `{"scenePath":"scenes/main.tscn","nodePath":"root/Player","newName":"Hero"}`)
	if result.IsError {
		t.Fatalf("unexpected error: %+v", result.Content)
	}
	text := result.Content[0].Text
	for _, want := range []string{`"action":"rename"`, `"scenePath":"scenes/main.tscn"`, `"nodePath":"root/Player"`, `"newName":"Hero"`} {
		if !strings.Contains(text, want) {
			t.Errorf("text = %q, want it to contain %q", text, want)
		}
	}
}

func TestRenameSceneNodeValidation(t *testing.T) {
	root := t.TempDir()
	makeProject(t, root)
	notAProject := t.TempDir()

	tests := []struct {
		name    string
		project string
		args    string
		want    string
	}{
		{name: "missing params", args: `{}`, want: "scenePath, nodePath, and newName are required."},
		{name: "missing newName", project: root, args: `{"scenePath":"main.tscn","nodePath":"root"}`, want: "scenePath, nodePath, and newName are required."},
		{name: "invalid project path", project: "../escape", args: `{"scenePath":"main.tscn","nodePath":"root","newName":"Hero"}`, want: "Invalid path."},
		{name: "invalid scene path", project: root, args: `{"scenePath":"../escape.tscn","nodePath":"root","newName":"Hero"}`, want: "Invalid path."},
		{name: "not a project", project: notAProject, args: `{"scenePath":"main.tscn","nodePath":"root","newName":"Hero"}`, want: "Not a valid Godot project"},
		{name: "missing scene file", project: root, args: `{"scenePath":"missing.tscn","nodePath":"root","newName":"Hero"}`, want: "Scene file does not exist: missing.tscn"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := callToolArgs(t, renameSceneNode(), tt.project, tt.args)
			if !result.IsError {
				t.Fatalf("expected an error result: %+v", result.Content)
			}
			if !strings.HasPrefix(result.Content[0].Text, tt.want) {
				t.Errorf("message = %q, want prefix %q", result.Content[0].Text, tt.want)
			}
		})
	}
}

func TestRenameSceneNodeScriptFailure(t *testing.T) {
	t.Setenv(godot.PathEnv, fakeGodotOps(t, `echo 'Failed to load scene: boom' >&2; exit 1`))
	root := t.TempDir()
	makeProject(t, root)
	seedFile(t, filepath.Join(root, "main.tscn"), "[gd_scene]\n")

	result := callToolArgs(t, renameSceneNode(), root, `{"scenePath":"main.tscn","nodePath":"root","newName":"Hero"}`)
	if !result.IsError {
		t.Fatalf("expected an error result: %+v", result.Content)
	}
	if !strings.HasPrefix(result.Content[0].Text, "manage_scene_structure failed: ") {
		t.Errorf("message = %q", result.Content[0].Text)
	}
	if len(result.Content) != 2 || !strings.HasPrefix(result.Content[1].Text, "Possible solutions:") {
		t.Errorf("solutions = %+v", result.Content)
	}
}

func TestRenameSceneNodeSpawnFailure(t *testing.T) {
	t.Setenv(godot.PathEnv, filepath.Join(t.TempDir(), "missing-godot"))
	root := t.TempDir()
	makeProject(t, root)
	seedFile(t, filepath.Join(root, "main.tscn"), "[gd_scene]\n")

	result := callToolArgs(t, renameSceneNode(), root, `{"scenePath":"main.tscn","nodePath":"root","newName":"Hero"}`)
	if !result.IsError {
		t.Fatalf("expected an error result: %+v", result.Content)
	}
	if !strings.HasPrefix(result.Content[0].Text, "manage_scene_structure failed: ") {
		t.Errorf("message = %q", result.Content[0].Text)
	}
}
