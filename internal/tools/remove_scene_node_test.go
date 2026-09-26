package tools

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/vilppuvuorinen/slopfood-godot-mcp/internal/godot"
)

func TestRemoveSceneNodeSchema(t *testing.T) {
	tool := removeSceneNode()
	if tool.Name != "remove_scene_node" {
		t.Errorf("name = %q", tool.Name)
	}
	if tool.Description != "Remove a node from a scene file (headless)" {
		t.Errorf("description = %q", tool.Description)
	}
	required, ok := tool.InputSchema["required"].([]string)
	if !ok || strings.Join(required, ",") != "scenePath,nodePath" {
		t.Errorf("required = %v", tool.InputSchema["required"])
	}
	props, ok := tool.InputSchema["properties"].(map[string]any)
	if !ok {
		t.Fatalf("properties = %T", tool.InputSchema["properties"])
	}
	for _, key := range []string{"scenePath", "nodePath"} {
		prop, exists := props[key]
		if !exists {
			t.Fatalf("missing property %q", key)
		}
		if prop.(map[string]any)["type"] != "string" {
			t.Errorf("property %q = %v", key, prop)
		}
	}
}

func TestRemoveSceneNodeSuccess(t *testing.T) {
	t.Setenv(godot.PathEnv, fakeGodotOps(t, `echo "$@"`))
	root := t.TempDir()
	makeProject(t, root)
	seedFile(t, filepath.Join(root, "scenes", "main.tscn"), "scene")

	result := callToolArgs(t, removeSceneNode(), root, `{"scenePath":"scenes/main.tscn","nodePath":"root/Player"}`)
	if result.IsError {
		t.Fatalf("unexpected error: %+v", result.Content)
	}
	if len(result.Content) != 1 {
		t.Fatalf("content = %+v", result.Content)
	}
	text := result.Content[0].Text
	if !strings.HasPrefix(text, "remove_node succeeded.") {
		t.Errorf("text = %q", text)
	}
	for _, want := range []string{`"scenePath":"scenes/main.tscn"`, `"nodePath":"root/Player"`} {
		if !strings.Contains(text, want) {
			t.Errorf("text = %q, want it to contain %q", text, want)
		}
	}
}

func TestRemoveSceneNodeValidation(t *testing.T) {
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
		{name: "invalid path", project: "../escape", args: `{"scenePath":"main.tscn","nodePath":"root/Player"}`, want: "Invalid path."},
		{name: "not a project", project: notAProject, args: `{"scenePath":"main.tscn","nodePath":"root/Player"}`, want: "Not a valid Godot project: "},
		{name: "missing scene", project: root, args: `{"scenePath":"missing.tscn","nodePath":"root/Player"}`, want: "Scene file does not exist: "},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := callToolArgs(t, removeSceneNode(), tt.project, tt.args)
			if !result.IsError {
				t.Fatalf("expected an error result: %+v", result.Content)
			}
			if !strings.HasPrefix(result.Content[0].Text, tt.want) {
				t.Errorf("message = %q, want prefix %q", result.Content[0].Text, tt.want)
			}
		})
	}
}

func TestRemoveSceneNodeScriptFailure(t *testing.T) {
	t.Setenv(godot.PathEnv, fakeGodotOps(t, `echo 'Failed to remove the root node of a scene' >&2; exit 1`))
	root := t.TempDir()
	makeProject(t, root)
	seedFile(t, filepath.Join(root, "scenes", "main.tscn"), "scene")

	result := callToolArgs(t, removeSceneNode(), root, `{"scenePath":"scenes/main.tscn","nodePath":"root"}`)
	if !result.IsError {
		t.Fatalf("expected an error result: %+v", result.Content)
	}
	if len(result.Content) != 2 {
		t.Fatalf("content = %+v", result.Content)
	}
	if !strings.HasPrefix(result.Content[0].Text, "remove_node failed: ") {
		t.Errorf("message = %q", result.Content[0].Text)
	}
	if !strings.HasPrefix(result.Content[1].Text, "Possible solutions:") {
		t.Errorf("solutions = %q", result.Content[1].Text)
	}
}

func TestRemoveSceneNodeSpawnFailure(t *testing.T) {
	t.Setenv(godot.PathEnv, filepath.Join(t.TempDir(), "missing-godot"))
	root := t.TempDir()
	makeProject(t, root)
	seedFile(t, filepath.Join(root, "scenes", "main.tscn"), "scene")

	result := callToolArgs(t, removeSceneNode(), root, `{"scenePath":"scenes/main.tscn","nodePath":"root/Player"}`)
	if !result.IsError {
		t.Fatalf("expected an error result: %+v", result.Content)
	}
	if !strings.HasPrefix(result.Content[0].Text, "remove_node failed: ") {
		t.Errorf("message = %q", result.Content[0].Text)
	}
}
