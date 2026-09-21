package e2e

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

// TestLoadSpriteSchema ports the reference's tool-definition expectations for
// load_sprite over the real stdio surface.
func TestLoadSpriteSchema(t *testing.T) {
	tool := findTool(t, connect(t), "load_sprite")
	if tool.Description != "Load a sprite into a Sprite2D node" {
		t.Errorf("description = %q", tool.Description)
	}

	data, err := json.Marshal(tool.InputSchema)
	if err != nil {
		t.Fatalf("marshal schema: %v", err)
	}
	var schema struct {
		Type       string                    `json:"type"`
		Required   []string                  `json:"required"`
		Properties map[string]map[string]any `json:"properties"`
	}
	if err := json.Unmarshal(data, &schema); err != nil {
		t.Fatalf("unmarshal schema %s: %v", data, err)
	}
	if schema.Type != "object" {
		t.Errorf("schema type = %q", schema.Type)
	}
	if strings.Join(schema.Required, ",") != "projectPath,scenePath,nodePath,texturePath" {
		t.Errorf("required = %v", schema.Required)
	}
	if len(schema.Properties) != 4 {
		t.Errorf("properties = %v", schema.Properties)
	}
	for _, key := range []string{"projectPath", "scenePath", "nodePath", "texturePath"} {
		if schema.Properties[key]["type"] != "string" {
			t.Errorf("%s type = %v", key, schema.Properties[key]["type"])
		}
	}
}

// TestLoadSpriteTool drives the real load_sprite tool against Godot, including
// the snake_case argument aliases.
func TestLoadSpriteTool(t *testing.T) {
	session := connect(t)
	requireGodot(t, session)

	t.Run("loads a sprite", func(t *testing.T) {
		root := t.TempDir()
		seedProject(t, root)

		res := callTool(t, session, "create_scene", map[string]any{
			"projectPath": root,
			"scenePath":   "scenes/main.tscn",
		})
		if res.IsError {
			t.Skipf("create_scene failed, skipping: %s", firstText(t, res))
		}

		res = callTool(t, session, "add_node", map[string]any{
			"projectPath": root,
			"scenePath":   "scenes/main.tscn",
			"nodeType":    "Sprite2D",
			"nodeName":    "Sprite",
		})
		if res.IsError {
			t.Skipf("add_node failed, skipping: %s", firstText(t, res))
		}

		writeE2EFile(t, filepath.Join(root, "icon.tres"), "[gd_resource type=\"GradientTexture2D\" format=3]\n\n[resource]\nwidth = 8\nheight = 8\n")

		res = callTool(t, session, "load_sprite", map[string]any{
			"projectPath": root,
			"scenePath":   "scenes/main.tscn",
			"nodePath":    "root/Sprite",
			"texturePath": "icon.tres",
		})
		if res.IsError {
			t.Fatalf("unexpected error: %s", firstText(t, res))
		}
		if got := firstText(t, res); !strings.HasPrefix(got, "Sprite loaded successfully with texture: icon.tres") {
			t.Errorf("text = %q", got)
		}
		scene := readFile(t, filepath.Join(root, "scenes", "main.tscn"))
		if !strings.Contains(scene, `texture = ExtResource(`) {
			t.Errorf("scene = %q, want a texture reference", scene)
		}
	})

	t.Run("snake_case aliases", func(t *testing.T) {
		root := t.TempDir()
		seedProject(t, root)

		res := callTool(t, session, "create_scene", map[string]any{
			"projectPath": root,
			"scenePath":   "scenes/aliased.tscn",
		})
		if res.IsError {
			t.Skipf("create_scene failed, skipping: %s", firstText(t, res))
		}

		res = callTool(t, session, "add_node", map[string]any{
			"projectPath": root,
			"scenePath":   "scenes/aliased.tscn",
			"nodeType":    "Sprite2D",
			"nodeName":    "Sprite",
		})
		if res.IsError {
			t.Skipf("add_node failed, skipping: %s", firstText(t, res))
		}

		writeE2EFile(t, filepath.Join(root, "icon.tres"), "[gd_resource type=\"GradientTexture2D\" format=3]\n\n[resource]\nwidth = 8\nheight = 8\n")

		res = callTool(t, session, "load_sprite", map[string]any{
			"project_path": root,
			"scene_path":   "scenes/aliased.tscn",
			"node_path":    "root/Sprite",
			"texture_path": "icon.tres",
		})
		if res.IsError {
			t.Fatalf("unexpected error: %s", firstText(t, res))
		}
		if got := firstText(t, res); !strings.HasPrefix(got, "Sprite loaded successfully with texture: icon.tres") {
			t.Errorf("text = %q", got)
		}
	})
}

// TestLoadSpriteErrors covers the reference's validation and failure messages.
func TestLoadSpriteErrors(t *testing.T) {
	session := connect(t)

	t.Run("missing params", func(t *testing.T) {
		res := callTool(t, session, "load_sprite", map[string]any{})
		if !res.IsError {
			t.Fatal("expected an error result")
		}
		if got := firstText(t, res); got != "Missing required parameters" {
			t.Errorf("message = %q", got)
		}
	})

	t.Run("invalid path", func(t *testing.T) {
		res := callTool(t, session, "load_sprite", map[string]any{
			"projectPath": "../escape",
			"scenePath":   "scenes/main.tscn",
			"nodePath":    "root/Sprite",
			"texturePath": "icon.tres",
		})
		if !res.IsError {
			t.Fatal("expected an error result")
		}
		if got := firstText(t, res); got != "Invalid path" {
			t.Errorf("message = %q", got)
		}
	})

	t.Run("not a project", func(t *testing.T) {
		root := t.TempDir()
		res := callTool(t, session, "load_sprite", map[string]any{
			"projectPath": root,
			"scenePath":   "scenes/main.tscn",
			"nodePath":    "root/Sprite",
			"texturePath": "icon.tres",
		})
		if !res.IsError {
			t.Fatal("expected an error result")
		}
		if got := firstText(t, res); got != "Not a valid Godot project: "+root {
			t.Errorf("message = %q", got)
		}
	})

	t.Run("missing scene", func(t *testing.T) {
		root := t.TempDir()
		seedProject(t, root)
		res := callTool(t, session, "load_sprite", map[string]any{
			"projectPath": root,
			"scenePath":   "missing.tscn",
			"nodePath":    "root/Sprite",
			"texturePath": "icon.tres",
		})
		if !res.IsError {
			t.Fatal("expected an error result")
		}
		if got := firstText(t, res); got != "Scene file does not exist: missing.tscn" {
			t.Errorf("message = %q", got)
		}
	})

	t.Run("missing texture", func(t *testing.T) {
		root := t.TempDir()
		seedProject(t, root)
		writeE2EFile(t, filepath.Join(root, "scenes", "main.tscn"), "scene")
		res := callTool(t, session, "load_sprite", map[string]any{
			"projectPath": root,
			"scenePath":   "scenes/main.tscn",
			"nodePath":    "root/Sprite",
			"texturePath": "missing.tres",
		})
		if !res.IsError {
			t.Fatal("expected an error result")
		}
		if got := firstText(t, res); got != "Texture file does not exist: missing.tres" {
			t.Errorf("message = %q", got)
		}
	})
}
