package e2e

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestCreateSceneSchema ports the reference's tool-definition expectations for
// create_scene over the real stdio surface.
func TestCreateSceneSchema(t *testing.T) {
	tool := findTool(t, connect(t), "create_scene")
	if tool.Description != "Create a new Godot scene file" {
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
	for _, key := range []string{"projectPath", "scenePath", "rootNodeType"} {
		if schema.Properties[key]["type"] != "string" {
			t.Errorf("%s type = %v", key, schema.Properties[key]["type"])
		}
	}
}

// TestCreateSceneTool drives the real create_scene tool against Godot,
// exercising the default root node type, custom types, and arg aliasing.
func TestCreateSceneTool(t *testing.T) {
	session := connect(t)
	requireGodot(t, session)

	t.Run("defaults to Node2D", func(t *testing.T) {
		root := t.TempDir()
		seedProject(t, root)

		res := callTool(t, session, "create_scene", map[string]any{
			"projectPath": root,
			"scenePath":   "scenes/main.tscn",
		})
		if res.IsError {
			t.Fatalf("unexpected error: %s", firstText(t, res))
		}
		if got := firstText(t, res); !strings.HasPrefix(got, "Scene created successfully at: scenes/main.tscn") {
			t.Errorf("text = %q", got)
		}
		scene := readFile(t, filepath.Join(root, "scenes", "main.tscn"))
		if !strings.Contains(scene, `type="Node2D"`) {
			t.Errorf("scene = %q, want a Node2D root", scene)
		}
	})

	t.Run("custom root and snake_case aliases", func(t *testing.T) {
		root := t.TempDir()
		seedProject(t, root)

		res := callTool(t, session, "create_scene", map[string]any{
			"project_path":   root,
			"scene_path":     "level.tscn",
			"root_node_type": "Node3D",
		})
		if res.IsError {
			t.Fatalf("unexpected error: %s", firstText(t, res))
		}
		scene := readFile(t, filepath.Join(root, "level.tscn"))
		if !strings.Contains(scene, `type="Node3D"`) {
			t.Errorf("scene = %q, want a Node3D root", scene)
		}
	})
}

// TestCreateSceneErrors covers the reference's validation and failure messages.
func TestCreateSceneErrors(t *testing.T) {
	session := connect(t)

	t.Run("missing paths", func(t *testing.T) {
		res := callTool(t, session, "create_scene", map[string]any{})
		if !res.IsError {
			t.Fatal("expected an error result")
		}
		if got := firstText(t, res); got != "Project path and scene path are required" {
			t.Errorf("message = %q", got)
		}
	})

	t.Run("invalid path", func(t *testing.T) {
		res := callTool(t, session, "create_scene", map[string]any{
			"projectPath": "../escape",
			"scenePath":   "main.tscn",
		})
		if !res.IsError {
			t.Fatal("expected an error result")
		}
		if got := firstText(t, res); got != "Invalid path" {
			t.Errorf("message = %q", got)
		}
	})

	t.Run("invalid rootNodeType", func(t *testing.T) {
		res := callTool(t, session, "create_scene", map[string]any{
			"projectPath":  t.TempDir(),
			"scenePath":    "main.tscn",
			"rootNodeType": "res://evil.gd",
		})
		if !res.IsError {
			t.Fatal("expected an error result")
		}
		if got := firstText(t, res); got != "Invalid rootNodeType" {
			t.Errorf("message = %q", got)
		}
	})

	t.Run("not a project", func(t *testing.T) {
		root := t.TempDir()
		res := callTool(t, session, "create_scene", map[string]any{
			"projectPath": root,
			"scenePath":   "main.tscn",
		})
		if !res.IsError {
			t.Fatal("expected an error result")
		}
		if got := firstText(t, res); got != "Not a valid Godot project: "+root {
			t.Errorf("message = %q", got)
		}
	})
}

// TestCreateSceneFailsWithoutGodot points GODOT_PATH at a missing binary and
// asserts the reference's generic failure message and solutions. It uses its
// own session so the shared environment is untouched.
func TestCreateSceneFailsWithoutGodot(t *testing.T) {
	session := connectWithEnv(t, "GODOT_PATH="+filepath.Join(t.TempDir(), "missing-godot"))
	root := t.TempDir()
	seedProject(t, root)

	res := callTool(t, session, "create_scene", map[string]any{
		"projectPath": root,
		"scenePath":   "main.tscn",
	})
	if !res.IsError {
		t.Fatal("expected an error result")
	}
	if got := firstText(t, res); !strings.HasPrefix(got, "Failed to create scene:") {
		t.Errorf("message = %q", got)
	}
	if len(res.Content) != 2 {
		t.Errorf("content = %+v", res.Content)
	}
}

// TestCreateSceneDebugModeLeavesNoTempFiles asserts the debug-mode write probe
// is cleaned up, matching the reference's GODOT_DEBUG_MODE behavior.
func TestCreateSceneDebugModeLeavesNoTempFiles(t *testing.T) {
	session := connect(t)
	requireGodot(t, session)
	root := t.TempDir()
	seedProject(t, root)

	res := callTool(t, session, "create_scene", map[string]any{
		"projectPath": root,
		"scenePath":   "main.tscn",
	})
	if res.IsError {
		t.Fatalf("unexpected error: %s", firstText(t, res))
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatalf("read project dir: %v", err)
	}
	for _, entry := range entries {
		if strings.HasSuffix(entry.Name(), ".tmp") {
			t.Errorf("leftover temp file: %s", entry.Name())
		}
	}
}
