package e2e

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestCreateSceneSchema ports the reference's tool-definition expectations for
// create_scene over the real stdio surface.
func TestCreateSceneSchema(t *testing.T) {
	schema := requireSchema(t, "create_scene", "Create a new Godot scene file")
	schema.requireRequired(t, "projectPath", "scenePath")
	schema.requireStringProps(t, "projectPath", "scenePath", "rootNodeType")
}

// TestCreateSceneTool drives the real create_scene tool against Godot,
// exercising the default root node type, custom types, and arg aliasing.
func TestCreateSceneTool(t *testing.T) {
	session := connect(t)
	requireGodot(t, session)

	t.Run("defaults to Node2D", func(t *testing.T) {
		root := seedScene(t, session, "scenes/main.tscn")
		scene := readFile(t, filepath.Join(root, "scenes", "main.tscn"))
		requireContains(t, scene, `type="Node2D"`)
	})

	t.Run("custom root and snake_case aliases", func(t *testing.T) {
		root := t.TempDir()
		seedProject(t, root)

		requireToolText(t, session, "create_scene", map[string]any{
			"project_path":   root,
			"scene_path":     "level.tscn",
			"root_node_type": "Node3D",
		})

		scene := readFile(t, filepath.Join(root, "level.tscn"))
		requireContains(t, scene, `type="Node3D"`)
	})
}

// TestCreateSceneErrors covers the reference's validation and failure messages.
func TestCreateSceneErrors(t *testing.T) {
	session := connect(t)

	t.Run("missing paths", func(t *testing.T) {
		requireToolError(t, session, "create_scene", map[string]any{}, "Project path and scene path are required")
	})

	t.Run("invalid path", func(t *testing.T) {
		requireToolError(t, session, "create_scene", map[string]any{
			"projectPath": "../escape",
			"scenePath":   "main.tscn",
		}, "Invalid path")
	})

	t.Run("invalid rootNodeType", func(t *testing.T) {
		requireToolError(t, session, "create_scene", map[string]any{
			"projectPath":  t.TempDir(),
			"scenePath":    "main.tscn",
			"rootNodeType": "res://evil.gd",
		}, "Invalid rootNodeType")
	})

	t.Run("not a project", func(t *testing.T) {
		root := t.TempDir()
		requireToolError(t, session, "create_scene", map[string]any{
			"projectPath": root,
			"scenePath":   "main.tscn",
		}, "Not a valid Godot project: "+root)
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
	requirePrefix(t, firstText(t, res), "Failed to create scene:")
	if len(res.Content) != 2 {
		t.Errorf("content = %+v", res.Content)
	}
}

// TestCreateSceneDebugModeLeavesNoTempFiles asserts the debug-mode write probe
// is cleaned up, matching the reference's GODOT_DEBUG_MODE behavior.
func TestCreateSceneDebugModeLeavesNoTempFiles(t *testing.T) {
	session := connect(t)
	requireGodot(t, session)
	root := seedScene(t, session, "main.tscn")

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
