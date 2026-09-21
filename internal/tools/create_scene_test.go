package tools

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/vilppuvuorinen/go-dot-mcp/internal/godot"
)

func TestCreateSceneSchema(t *testing.T) {
	tool := createScene()
	if tool.Name != "create_scene" {
		t.Errorf("name = %q", tool.Name)
	}
	if tool.Description != "Create a new Godot scene file" {
		t.Errorf("description = %q", tool.Description)
	}
	required, ok := tool.InputSchema["required"].([]string)
	if !ok || strings.Join(required, ",") != "projectPath,scenePath" {
		t.Errorf("required = %v", tool.InputSchema["required"])
	}
	props, ok := tool.InputSchema["properties"].(map[string]any)
	if !ok {
		t.Fatalf("properties = %T", tool.InputSchema["properties"])
	}
	for _, key := range []string{"projectPath", "scenePath", "rootNodeType"} {
		if _, ok := props[key]; !ok {
			t.Errorf("missing property %q", key)
		}
	}
}

func TestCreateSceneSuccess(t *testing.T) {
	t.Setenv(godot.PathEnv, fakeGodotOps(t, `echo "$@"`))
	root := t.TempDir()
	makeProject(t, root)

	result := callToolArgs(t, createScene(), `{"projectPath":`+jsonString(root)+`,"scenePath":"scenes/main.tscn"}`)
	if result.IsError {
		t.Fatalf("unexpected error: %+v", result.Content)
	}
	if len(result.Content) != 1 {
		t.Fatalf("content = %+v", result.Content)
	}
	text := result.Content[0].Text
	if !strings.HasPrefix(text, "Scene created successfully at: scenes/main.tscn") {
		t.Errorf("text = %q", text)
	}
	for _, want := range []string{`"scene_path":"scenes/main.tscn"`, `"root_node_type":"Node2D"`} {
		if !strings.Contains(text, want) {
			t.Errorf("text = %q, want it to contain %q", text, want)
		}
	}
}

func TestCreateSceneCustomRootNodeType(t *testing.T) {
	t.Setenv(godot.PathEnv, fakeGodotOps(t, `echo "$@"`))
	root := t.TempDir()
	makeProject(t, root)

	result := callToolArgs(t, createScene(), `{"projectPath":`+jsonString(root)+`,"scenePath":"main.tscn","rootNodeType":"Node3D"}`)
	if result.IsError {
		t.Fatalf("unexpected error: %+v", result.Content)
	}
	if !strings.Contains(result.Content[0].Text, `"root_node_type":"Node3D"`) {
		t.Errorf("text = %q", result.Content[0].Text)
	}
}

func TestCreateSceneSnakeCaseAliases(t *testing.T) {
	t.Setenv(godot.PathEnv, fakeGodotOps(t, `echo "$@"`))
	root := t.TempDir()
	makeProject(t, root)

	result := callToolArgs(t, createScene(), `{"project_path":`+jsonString(root)+`,"scene_path":"main.tscn","root_node_type":"Node3D"}`)
	if result.IsError {
		t.Fatalf("unexpected error: %+v", result.Content)
	}
	if !strings.Contains(result.Content[0].Text, `"root_node_type":"Node3D"`) {
		t.Errorf("text = %q", result.Content[0].Text)
	}
}

func TestCreateSceneValidation(t *testing.T) {
	root := t.TempDir()
	makeProject(t, root)

	tests := []struct {
		name string
		args string
		want string
	}{
		{name: "missing paths", args: `{}`, want: "Project path and scene path are required"},
		{name: "missing scene", args: `{"projectPath":` + jsonString(root) + `}`, want: "Project path and scene path are required"},
		{name: "invalid project path", args: `{"projectPath":"../escape","scenePath":"main.tscn"}`, want: "Invalid path"},
		{name: "invalid scene path", args: `{"projectPath":` + jsonString(root) + `,"scenePath":"../escape.tscn"}`, want: "Invalid path"},
		{name: "invalid root node type", args: `{"projectPath":` + jsonString(root) + `,"scenePath":"main.tscn","rootNodeType":"res://evil.gd"}`, want: "Invalid rootNodeType"},
		{name: "not a project", args: `{"projectPath":` + jsonString(t.TempDir()) + `,"scenePath":"main.tscn"}`, want: "Not a valid Godot project"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := callToolArgs(t, createScene(), tt.args)
			if !result.IsError {
				t.Fatalf("expected an error result: %+v", result.Content)
			}
			if !strings.HasPrefix(result.Content[0].Text, tt.want) {
				t.Errorf("message = %q, want prefix %q", result.Content[0].Text, tt.want)
			}
		})
	}
}

func TestCreateSceneScriptFailure(t *testing.T) {
	t.Setenv(godot.PathEnv, fakeGodotOps(t, `echo 'Failed to create scene: boom' >&2; exit 1`))
	root := t.TempDir()
	makeProject(t, root)

	result := callToolArgs(t, createScene(), `{"projectPath":`+jsonString(root)+`,"scenePath":"main.tscn"}`)
	if !result.IsError {
		t.Fatalf("expected an error result: %+v", result.Content)
	}
	if len(result.Content) != 2 {
		t.Fatalf("content = %+v", result.Content)
	}
	if !strings.HasPrefix(result.Content[0].Text, "Failed to create scene: ") {
		t.Errorf("message = %q", result.Content[0].Text)
	}
	if !strings.HasPrefix(result.Content[1].Text, "Possible solutions:") {
		t.Errorf("solutions = %q", result.Content[1].Text)
	}
}

func TestCreateSceneSpawnFailure(t *testing.T) {
	t.Setenv(godot.PathEnv, filepath.Join(t.TempDir(), "missing-godot"))
	root := t.TempDir()
	makeProject(t, root)

	result := callToolArgs(t, createScene(), `{"projectPath":`+jsonString(root)+`,"scenePath":"main.tscn"}`)
	if !result.IsError {
		t.Fatalf("expected an error result: %+v", result.Content)
	}
	if !strings.HasPrefix(result.Content[0].Text, "Failed to create scene: ") {
		t.Errorf("message = %q", result.Content[0].Text)
	}
}
