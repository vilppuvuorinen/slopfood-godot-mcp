package tools

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/vilppuvuorinen/go-dot-mcp/internal/godot"
)

func TestModifySceneNodeSchema(t *testing.T) {
	tool := modifySceneNode()
	if tool.Name != "modify_scene_node" {
		t.Errorf("name = %q", tool.Name)
	}
	if tool.Description != "Modify node properties in a scene file (headless)" {
		t.Errorf("description = %q", tool.Description)
	}
	required, ok := tool.InputSchema["required"].([]string)
	if !ok || strings.Join(required, ",") != "projectPath,scenePath,nodePath,properties" {
		t.Errorf("required = %v", tool.InputSchema["required"])
	}
	props, ok := tool.InputSchema["properties"].(map[string]any)
	if !ok {
		t.Fatalf("properties = %T", tool.InputSchema["properties"])
	}
	for _, key := range []string{"projectPath", "scenePath", "nodePath"} {
		prop, propOK := props[key].(map[string]any)
		if !propOK || prop["type"] != "string" {
			t.Errorf("property %q = %v, want string", key, props[key])
		}
	}
	properties, ok := props["properties"].(map[string]any)
	if !ok || properties["type"] != "object" {
		t.Errorf("properties property = %v", props["properties"])
	}
}

func TestModifySceneNodeSuccess(t *testing.T) {
	t.Setenv(godot.PathEnv, fakeGodotOps(t, `echo "$@"`))
	root := t.TempDir()
	makeProject(t, root)
	seedFile(t, filepath.Join(root, "scenes", "main.tscn"), "scene")

	result := callToolArgs(t, modifySceneNode(), `{"projectPath":`+jsonString(root)+`,"scenePath":"scenes/main.tscn","nodePath":"root/Player","properties":{"visible":false}}`)
	if result.IsError {
		t.Fatalf("unexpected error: %+v", result.Content)
	}
	if len(result.Content) != 1 {
		t.Fatalf("content = %+v", result.Content)
	}
	text := result.Content[0].Text
	if !strings.HasPrefix(text, "modify_node succeeded.") {
		t.Errorf("text = %q", text)
	}
	for _, want := range []string{`"scenePath":"scenes/main.tscn"`, `"nodePath":"root/Player"`, `"properties":{"visible":false}`} {
		if !strings.Contains(text, want) {
			t.Errorf("text = %q, want it to contain %q", text, want)
		}
	}
}

func TestModifySceneNodeVector2Properties(t *testing.T) {
	t.Setenv(godot.PathEnv, fakeGodotOps(t, `echo "$@"`))
	root := t.TempDir()
	makeProject(t, root)
	seedFile(t, filepath.Join(root, "scenes", "main.tscn"), "scene")

	result := callToolArgs(t, modifySceneNode(), `{"projectPath":`+jsonString(root)+`,"scenePath":"scenes/main.tscn","nodePath":"root/Player","properties":{"position":{"$type":"Vector2","x":10,"y":20}}}`)
	if result.IsError {
		t.Fatalf("unexpected error: %+v", result.Content)
	}
	if !strings.Contains(result.Content[0].Text, `"properties":{"position":{"$type":"Vector2","x":10,"y":20}}`) {
		t.Errorf("text = %q, want the tagged Vector2 to round-trip", result.Content[0].Text)
	}
}

func TestModifySceneNodeLeavesDictionaryValuedPropertiesUntouched(t *testing.T) {
	t.Setenv(godot.PathEnv, fakeGodotOps(t, `echo "$@"`))
	root := t.TempDir()
	makeProject(t, root)
	seedFile(t, filepath.Join(root, "scenes", "main.tscn"), "scene")

	result := callToolArgs(t, modifySceneNode(), `{"projectPath":`+jsonString(root)+`,"scenePath":"scenes/main.tscn","nodePath":"root/Player","properties":{"config":{"myKey":1}}}`)
	if result.IsError {
		t.Fatalf("unexpected error: %+v", result.Content)
	}
	text := result.Content[0].Text
	if !strings.Contains(text, `"myKey":1`) {
		t.Errorf("text = %q, want dictionary key myKey preserved", text)
	}
	if strings.Contains(text, `"my_key"`) {
		t.Errorf("text = %q, dictionary key was rewritten", text)
	}
}

func TestModifySceneNodeValidation(t *testing.T) {
	root := t.TempDir()
	makeProject(t, root)

	tests := []struct {
		name string
		args string
		want string
	}{
		{name: "missing params", args: `{}`, want: "projectPath, scenePath, nodePath, and properties are required."},
		{name: "missing properties", args: `{"projectPath":` + jsonString(root) + `,"scenePath":"main.tscn","nodePath":"root"}`, want: "projectPath, scenePath, nodePath, and properties are required."},
		{name: "invalid project path", args: `{"projectPath":"../escape","scenePath":"main.tscn","nodePath":"root","properties":{"visible":true}}`, want: "Invalid path."},
		{name: "invalid scene path", args: `{"projectPath":` + jsonString(root) + `,"scenePath":"../escape.tscn","nodePath":"root","properties":{"visible":true}}`, want: "Invalid path."},
		{name: "not a project", args: `{"projectPath":` + jsonString(t.TempDir()) + `,"scenePath":"main.tscn","nodePath":"root","properties":{"visible":true}}`, want: "Not a valid Godot project"},
		{name: "missing scene file", args: `{"projectPath":` + jsonString(root) + `,"scenePath":"missing.tscn","nodePath":"root","properties":{"visible":true}}`, want: "Scene file does not exist: missing.tscn"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := callToolArgs(t, modifySceneNode(), tt.args)
			if !result.IsError {
				t.Fatalf("expected an error result: %+v", result.Content)
			}
			if !strings.HasPrefix(result.Content[0].Text, tt.want) {
				t.Errorf("message = %q, want prefix %q", result.Content[0].Text, tt.want)
			}
		})
	}
}

func TestModifySceneNodeScriptFailure(t *testing.T) {
	t.Setenv(godot.PathEnv, fakeGodotOps(t, `echo 'Failed to modify node: boom' >&2; exit 1`))
	root := t.TempDir()
	makeProject(t, root)
	seedFile(t, filepath.Join(root, "main.tscn"), "[gd_scene]\n")

	result := callToolArgs(t, modifySceneNode(), `{"projectPath":`+jsonString(root)+`,"scenePath":"main.tscn","nodePath":"root","properties":{"visible":true}}`)
	if !result.IsError {
		t.Fatalf("expected an error result: %+v", result.Content)
	}
	if !strings.HasPrefix(result.Content[0].Text, "modify_node failed: ") {
		t.Errorf("message = %q", result.Content[0].Text)
	}
	if len(result.Content) != 2 || !strings.HasPrefix(result.Content[1].Text, "Possible solutions:") {
		t.Errorf("solutions = %+v", result.Content)
	}
}

func TestModifySceneNodeSpawnFailure(t *testing.T) {
	t.Setenv(godot.PathEnv, filepath.Join(t.TempDir(), "missing-godot"))
	root := t.TempDir()
	makeProject(t, root)
	seedFile(t, filepath.Join(root, "main.tscn"), "[gd_scene]\n")

	result := callToolArgs(t, modifySceneNode(), `{"projectPath":`+jsonString(root)+`,"scenePath":"main.tscn","nodePath":"root","properties":{"visible":true}}`)
	if !result.IsError {
		t.Fatalf("expected an error result: %+v", result.Content)
	}
	if !strings.HasPrefix(result.Content[0].Text, "modify_node failed: ") {
		t.Errorf("message = %q", result.Content[0].Text)
	}
}
