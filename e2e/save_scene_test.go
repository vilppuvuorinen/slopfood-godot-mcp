package e2e

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestSaveSceneSchema ports the reference's tool-definition expectations for
// save_scene over the real stdio surface.
func TestSaveSceneSchema(t *testing.T) {
	tool := findTool(t, connect(t), "save_scene")
	if tool.Description != "Save changes to a scene file" {
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
	if strings.Join(schema.Required, ",") != "projectPath,scenePath" {
		t.Errorf("required = %v", schema.Required)
	}
	if len(schema.Properties) != 3 {
		t.Errorf("properties = %v", schema.Properties)
	}
	for _, key := range []string{"projectPath", "scenePath", "newPath"} {
		if schema.Properties[key]["type"] != "string" {
			t.Errorf("%s type = %v", key, schema.Properties[key]["type"])
		}
	}
}

// TestSaveSceneTool drives the real save_scene tool against Godot, including
// the optional variant path and the snake_case argument aliases.
func TestSaveSceneTool(t *testing.T) {
	session := connect(t)
	requireGodot(t, session)

	root := t.TempDir()
	seedProject(t, root)

	res := callTool(t, session, "create_scene", map[string]any{
		"projectPath": root,
		"scenePath":   "scenes/main.tscn",
	})
	if res.IsError {
		t.Skipf("create_scene failed, skipping: %s", firstText(t, res))
	}

	t.Run("saves in place", func(t *testing.T) {
		res := callTool(t, session, "save_scene", map[string]any{
			"projectPath": root,
			"scenePath":   "scenes/main.tscn",
		})
		if res.IsError {
			t.Fatalf("unexpected error: %s", firstText(t, res))
		}
		if got := firstText(t, res); !strings.HasPrefix(got, "Scene saved successfully to: scenes/main.tscn") {
			t.Errorf("text = %q", got)
		}
	})

	t.Run("saves a variant", func(t *testing.T) {
		res := callTool(t, session, "save_scene", map[string]any{
			"projectPath": root,
			"scenePath":   "scenes/main.tscn",
			"newPath":     "variants/main_variant.tscn",
		})
		if res.IsError {
			t.Fatalf("unexpected error: %s", firstText(t, res))
		}
		if got := firstText(t, res); !strings.HasPrefix(got, "Scene saved successfully to: variants/main_variant.tscn") {
			t.Errorf("text = %q", got)
		}
		if _, err := os.Stat(filepath.Join(root, "variants", "main_variant.tscn")); err != nil {
			t.Errorf("stat saved variant: %v", err)
		}
	})

	t.Run("snake_case aliases", func(t *testing.T) {
		res := callTool(t, session, "save_scene", map[string]any{
			"project_path": root,
			"scene_path":   "scenes/main.tscn",
			"new_path":     "variants/aliased_variant.tscn",
		})
		if res.IsError {
			t.Fatalf("unexpected error: %s", firstText(t, res))
		}
		if got := firstText(t, res); !strings.HasPrefix(got, "Scene saved successfully to: variants/aliased_variant.tscn") {
			t.Errorf("text = %q", got)
		}
	})
}

// TestSaveSceneErrors covers the reference's validation messages.
func TestSaveSceneErrors(t *testing.T) {
	session := connect(t)

	t.Run("missing params", func(t *testing.T) {
		res := callTool(t, session, "save_scene", map[string]any{})
		if !res.IsError {
			t.Fatal("expected an error result")
		}
		if got := firstText(t, res); got != "Missing required parameters" {
			t.Errorf("message = %q", got)
		}
	})

	t.Run("invalid path", func(t *testing.T) {
		res := callTool(t, session, "save_scene", map[string]any{
			"projectPath": "../escape",
			"scenePath":   "scenes/main.tscn",
		})
		if !res.IsError {
			t.Fatal("expected an error result")
		}
		if got := firstText(t, res); got != "Invalid path" {
			t.Errorf("message = %q", got)
		}
	})

	t.Run("invalid new path", func(t *testing.T) {
		root := t.TempDir()
		seedProject(t, root)
		res := callTool(t, session, "save_scene", map[string]any{
			"projectPath": root,
			"scenePath":   "scenes/main.tscn",
			"newPath":     "../escape",
		})
		if !res.IsError {
			t.Fatal("expected an error result")
		}
		if got := firstText(t, res); got != "Invalid new path" {
			t.Errorf("message = %q", got)
		}
	})

	t.Run("not a project", func(t *testing.T) {
		root := t.TempDir()
		res := callTool(t, session, "save_scene", map[string]any{
			"projectPath": root,
			"scenePath":   "scenes/main.tscn",
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
		res := callTool(t, session, "save_scene", map[string]any{
			"projectPath": root,
			"scenePath":   "missing.tscn",
		})
		if !res.IsError {
			t.Fatal("expected an error result")
		}
		if got := firstText(t, res); got != "Scene file does not exist: missing.tscn" {
			t.Errorf("message = %q", got)
		}
	})
}
