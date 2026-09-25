package tools

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/vilppuvuorinen/go-dot-mcp/internal/godot"
)

func TestLoadSpriteSchema(t *testing.T) {
	tool := loadSprite()
	if tool.Name != "load_sprite" {
		t.Errorf("name = %q", tool.Name)
	}
	if tool.Description != "Load a sprite into a Sprite2D node" {
		t.Errorf("description = %q", tool.Description)
	}
	required, ok := tool.InputSchema["required"].([]string)
	if !ok || strings.Join(required, ",") != "projectPath,scenePath,nodePath,texturePath" {
		t.Errorf("required = %v", tool.InputSchema["required"])
	}
	props, ok := tool.InputSchema["properties"].(map[string]any)
	if !ok {
		t.Fatalf("properties = %T", tool.InputSchema["properties"])
	}
	if len(props) != 4 {
		t.Errorf("properties = %v", props)
	}
	for _, key := range []string{"projectPath", "scenePath", "nodePath", "texturePath"} {
		property, ok := props[key].(map[string]any)
		if !ok || property["type"] != "string" {
			t.Errorf("%s property = %v", key, props[key])
		}
	}
}

func TestLoadSpriteSuccess(t *testing.T) {
	t.Setenv(godot.PathEnv, fakeGodotOps(t, `echo "$@"`))
	root := t.TempDir()
	makeProject(t, root)
	seedFile(t, filepath.Join(root, "scenes", "main.tscn"), "scene")
	seedFile(t, filepath.Join(root, "icon.tres"), "texture")

	result := callToolArgs(t, loadSprite(), `{"projectPath":`+jsonString(root)+`,"scenePath":"scenes/main.tscn","nodePath":"root/Sprite","texturePath":"icon.tres"}`)
	if result.IsError {
		t.Fatalf("unexpected error: %+v", result.Content)
	}
	if len(result.Content) != 1 {
		t.Fatalf("content = %+v", result.Content)
	}
	text := result.Content[0].Text
	if !strings.HasPrefix(text, "Sprite loaded successfully with texture: icon.tres") {
		t.Errorf("text = %q", text)
	}
	for _, want := range []string{`"nodePath":"root/Sprite"`, `"texturePath":"icon.tres"`} {
		if !strings.Contains(text, want) {
			t.Errorf("text = %q, want it to contain %q", text, want)
		}
	}
}

func TestLoadSpriteValidation(t *testing.T) {
	root := t.TempDir()
	makeProject(t, root)
	seedFile(t, filepath.Join(root, "scenes", "main.tscn"), "scene")

	tests := []struct {
		name string
		args string
		want string
	}{
		{name: "missing params", args: `{}`, want: "Missing required parameters"},
		{name: "invalid path", args: `{"projectPath":"../escape","scenePath":"scenes/main.tscn","nodePath":"root/Sprite","texturePath":"icon.tres"}`, want: "Invalid path"},
		{name: "not a project", args: `{"projectPath":` + jsonString(t.TempDir()) + `,"scenePath":"scenes/main.tscn","nodePath":"root/Sprite","texturePath":"icon.tres"}`, want: "Not a valid Godot project"},
		{name: "missing scene", args: `{"projectPath":` + jsonString(root) + `,"scenePath":"missing.tscn","nodePath":"root/Sprite","texturePath":"icon.tres"}`, want: "Scene file does not exist: "},
		{name: "missing texture", args: `{"projectPath":` + jsonString(root) + `,"scenePath":"scenes/main.tscn","nodePath":"root/Sprite","texturePath":"missing.tres"}`, want: "Texture file does not exist: "},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := callToolArgs(t, loadSprite(), tt.args)
			if !result.IsError {
				t.Fatalf("expected an error result: %+v", result.Content)
			}
			if !strings.HasPrefix(result.Content[0].Text, tt.want) {
				t.Errorf("message = %q, want prefix %q", result.Content[0].Text, tt.want)
			}
		})
	}
}

func TestLoadSpriteScriptFailure(t *testing.T) {
	t.Setenv(godot.PathEnv, fakeGodotOps(t, `echo 'Failed to load sprite: boom' >&2; exit 1`))
	root := t.TempDir()
	makeProject(t, root)
	seedFile(t, filepath.Join(root, "scenes", "main.tscn"), "scene")
	seedFile(t, filepath.Join(root, "icon.tres"), "texture")

	result := callToolArgs(t, loadSprite(), `{"projectPath":`+jsonString(root)+`,"scenePath":"scenes/main.tscn","nodePath":"root/Sprite","texturePath":"icon.tres"}`)
	if !result.IsError {
		t.Fatalf("expected an error result: %+v", result.Content)
	}
	if len(result.Content) != 2 {
		t.Fatalf("content = %+v", result.Content)
	}
	if !strings.HasPrefix(result.Content[0].Text, "Failed to load sprite: ") {
		t.Errorf("message = %q", result.Content[0].Text)
	}
	if !strings.HasPrefix(result.Content[1].Text, "Possible solutions:") {
		t.Errorf("solutions = %q", result.Content[1].Text)
	}
}

func TestLoadSpriteSpawnFailure(t *testing.T) {
	t.Setenv(godot.PathEnv, filepath.Join(t.TempDir(), "missing-godot"))
	root := t.TempDir()
	makeProject(t, root)
	seedFile(t, filepath.Join(root, "scenes", "main.tscn"), "scene")
	seedFile(t, filepath.Join(root, "icon.tres"), "texture")

	result := callToolArgs(t, loadSprite(), `{"projectPath":`+jsonString(root)+`,"scenePath":"scenes/main.tscn","nodePath":"root/Sprite","texturePath":"icon.tres"}`)
	if !result.IsError {
		t.Fatalf("expected an error result: %+v", result.Content)
	}
	if !strings.HasPrefix(result.Content[0].Text, "Failed to load sprite: ") {
		t.Errorf("message = %q", result.Content[0].Text)
	}
}
