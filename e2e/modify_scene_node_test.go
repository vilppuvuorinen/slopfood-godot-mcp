package e2e

import (
	"path/filepath"
	"testing"
)

// TestModifySceneNodeSchema ports the reference's tool-definition expectations
// for modify_scene_node over the real stdio surface.
func TestModifySceneNodeSchema(t *testing.T) {
	schema := requireSchema(t, "modify_scene_node", "Modify node properties in a scene file (headless)")
	schema.requireRequired(t, "projectPath", "scenePath", "nodePath", "properties")
	schema.requireStringProps(t, "projectPath", "scenePath", "nodePath")
	schema.requireType(t, "properties", "object")
	schema.requirePropCount(t, 4)
}

// TestModifySceneNodeTool drives the real modify_scene_node tool against Godot
// and reads the saved scene back from disk.
func TestModifySceneNodeTool(t *testing.T) {
	session := connect(t)
	requireGodot(t, session)

	root := seedScene(t, session, "scenes/main.tscn", "Node2D")

	requireToolText(t, session, "add_node", map[string]any{
		"projectPath": root,
		"scenePath":   "scenes/main.tscn",
		"nodeType":    "Sprite2D",
		"nodeName":    "Player",
	})

	requireToolText(t, session, "modify_scene_node", map[string]any{
		"projectPath": root,
		"scenePath":   "scenes/main.tscn",
		"nodePath":    "root/Player",
		"properties": map[string]any{
			"position": map[string]any{"x": 10, "y": 20},
			"visible":  false,
		},
	})

	scene := readFile(t, filepath.Join(root, "scenes", "main.tscn"))
	requireContains(t, scene, "position = Vector2(10, 20)", "visible = false")
}

// TestModifySceneNodeErrors covers the reference's validation and failure
// messages.
func TestModifySceneNodeErrors(t *testing.T) {
	session := connect(t)

	t.Run("missing scene", func(t *testing.T) {
		root := t.TempDir()
		seedProject(t, root)
		requireToolError(t, session, "modify_scene_node", map[string]any{
			"projectPath": root,
			"scenePath":   "missing.tscn",
			"nodePath":    "root",
			"properties":  map[string]any{"visible": true},
		}, "Scene file does not exist: missing.tscn")
	})

	t.Run("not a project", func(t *testing.T) {
		root := t.TempDir()
		requireToolError(t, session, "modify_scene_node", map[string]any{
			"projectPath": root,
			"scenePath":   "main.tscn",
			"nodePath":    "root",
			"properties":  map[string]any{"visible": true},
		}, "Not a valid Godot project: "+root)
	})

	t.Run("invalid path", func(t *testing.T) {
		requireToolError(t, session, "modify_scene_node", map[string]any{
			"projectPath": "../escape",
			"scenePath":   "main.tscn",
			"nodePath":    "root",
			"properties":  map[string]any{"visible": true},
		}, "Invalid path.")
	})
}
