package tools

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/vilppuvuorinen/go-dot-mcp/internal/godot"
)

func TestAddNodeSchema(t *testing.T) {
	tool := addNode()
	if tool.Name != "add_node" {
		t.Errorf("name = %q", tool.Name)
	}
	if tool.Description != "Add a node to an existing scene" {
		t.Errorf("description = %q", tool.Description)
	}
	required, ok := tool.InputSchema["required"].([]string)
	if !ok || strings.Join(required, ",") != "scenePath,nodeType,nodeName" {
		t.Errorf("required = %v", tool.InputSchema["required"])
	}
	props, ok := tool.InputSchema["properties"].(map[string]any)
	if !ok {
		t.Fatalf("properties = %T", tool.InputSchema["properties"])
	}
	for _, key := range []string{"scenePath", "parentNodePath", "nodeType", "nodeName", "properties"} {
		if _, exists := props[key]; !exists {
			t.Errorf("missing property %q", key)
		}
	}
	properties, ok := props["properties"].(map[string]any)
	if !ok || properties["type"] != "object" {
		t.Errorf("properties property = %v", props["properties"])
	}
}

func TestAddNodeSuccess(t *testing.T) {
	t.Setenv(godot.PathEnv, fakeGodotOps(t, `echo "$@"`))
	root := t.TempDir()
	makeProject(t, root)
	seedFile(t, filepath.Join(root, "scenes", "main.tscn"), "scene")

	result := callToolArgs(t, addNode(), `{"projectPath":`+jsonString(root)+`,"scenePath":"scenes/main.tscn","nodeType":"Sprite2D","nodeName":"Player","parentNodePath":"root","properties":{"position":1}}`)
	if result.IsError {
		t.Fatalf("unexpected error: %+v", result.Content)
	}
	if len(result.Content) != 1 {
		t.Fatalf("content = %+v", result.Content)
	}
	text := result.Content[0].Text
	if !strings.HasPrefix(text, "Node 'Player' of type 'Sprite2D' added successfully to 'scenes/main.tscn'.") {
		t.Errorf("text = %q", text)
	}
	for _, want := range []string{`"nodeType":"Sprite2D"`, `"nodeName":"Player"`, `"parentNodePath":"root"`, `"properties":{"position":1}`} {
		if !strings.Contains(text, want) {
			t.Errorf("text = %q, want it to contain %q", text, want)
		}
	}
}

func TestAddNodeOmitsOptionalParams(t *testing.T) {
	t.Setenv(godot.PathEnv, fakeGodotOps(t, `echo "$@"`))
	root := t.TempDir()
	makeProject(t, root)
	seedFile(t, filepath.Join(root, "scenes", "main.tscn"), "scene")

	result := callToolArgs(t, addNode(), `{"projectPath":`+jsonString(root)+`,"scenePath":"scenes/main.tscn","nodeType":"Sprite2D","nodeName":"Player"}`)
	if result.IsError {
		t.Fatalf("unexpected error: %+v", result.Content)
	}
	text := result.Content[0].Text
	for _, unwanted := range []string{"parentNodePath", "properties"} {
		if strings.Contains(text, unwanted) {
			t.Errorf("text = %q, want it to omit %q", text, unwanted)
		}
	}
}

func TestAddNodeValidation(t *testing.T) {
	root := t.TempDir()
	makeProject(t, root)

	tests := []struct {
		name string
		args string
		want string
	}{
		{name: "missing params", args: `{}`, want: "Missing required parameters"},
		{name: "invalid path", args: `{"projectPath":"../escape","scenePath":"main.tscn","nodeType":"Sprite2D","nodeName":"Player"}`, want: "Invalid path"},
		{name: "invalid nodeType", args: `{"projectPath":` + jsonString(root) + `,"scenePath":"main.tscn","nodeType":"res://evil.gd","nodeName":"Player"}`, want: "Invalid nodeType"},
		{name: "not a project", args: `{"projectPath":` + jsonString(t.TempDir()) + `,"scenePath":"main.tscn","nodeType":"Sprite2D","nodeName":"Player"}`, want: "Not a valid Godot project"},
		{name: "missing scene", args: `{"projectPath":` + jsonString(root) + `,"scenePath":"missing.tscn","nodeType":"Sprite2D","nodeName":"Player"}`, want: "Scene file does not exist: "},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := callToolArgs(t, addNode(), tt.args)
			if !result.IsError {
				t.Fatalf("expected an error result: %+v", result.Content)
			}
			if !strings.HasPrefix(result.Content[0].Text, tt.want) {
				t.Errorf("message = %q, want prefix %q", result.Content[0].Text, tt.want)
			}
		})
	}
}

func TestAddNodeScriptFailure(t *testing.T) {
	t.Setenv(godot.PathEnv, fakeGodotOps(t, `echo 'Failed to add node: boom' >&2; exit 1`))
	root := t.TempDir()
	makeProject(t, root)
	seedFile(t, filepath.Join(root, "scenes", "main.tscn"), "scene")

	result := callToolArgs(t, addNode(), `{"projectPath":`+jsonString(root)+`,"scenePath":"scenes/main.tscn","nodeType":"Sprite2D","nodeName":"Player"}`)
	if !result.IsError {
		t.Fatalf("expected an error result: %+v", result.Content)
	}
	if len(result.Content) != 2 {
		t.Fatalf("content = %+v", result.Content)
	}
	if !strings.HasPrefix(result.Content[0].Text, "Failed to add node: ") {
		t.Errorf("message = %q", result.Content[0].Text)
	}
	if !strings.HasPrefix(result.Content[1].Text, "Possible solutions:") {
		t.Errorf("solutions = %q", result.Content[1].Text)
	}
}

func TestAddNodeSpawnFailure(t *testing.T) {
	t.Setenv(godot.PathEnv, filepath.Join(t.TempDir(), "missing-godot"))
	root := t.TempDir()
	makeProject(t, root)
	seedFile(t, filepath.Join(root, "scenes", "main.tscn"), "scene")

	result := callToolArgs(t, addNode(), `{"projectPath":`+jsonString(root)+`,"scenePath":"scenes/main.tscn","nodeType":"Sprite2D","nodeName":"Player"}`)
	if !result.IsError {
		t.Fatalf("expected an error result: %+v", result.Content)
	}
	if !strings.HasPrefix(result.Content[0].Text, "Failed to add node: ") {
		t.Errorf("message = %q", result.Content[0].Text)
	}
}
