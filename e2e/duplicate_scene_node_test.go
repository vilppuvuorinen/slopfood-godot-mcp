package e2e

import (
	"path/filepath"
	"testing"
)

// TestDuplicateSceneNodeSchema pins the tool's input surface.
func TestDuplicateSceneNodeSchema(t *testing.T) {
	t.Parallel()
	schema := requireSchema(t, "duplicate_scene_node", "Duplicate a node within a .tscn scene")
	schema.requireRequired(t, "scenePath", "nodePath")
	schema.requireStringProps(t, "scenePath", "nodePath")
	schema.requirePropCount(t, 2)
}

// TestDuplicateSceneNodeTool drives the real duplicate_scene_node tool against
// Godot.
func TestDuplicateSceneNodeTool(t *testing.T) {
	t.Parallel()
	t.Run("duplicates a node", func(t *testing.T) {
		root, session := seedScene(t, "scenes/duplicate.tscn", "Node2D")
		requireToolText(t, session, "add_node", map[string]any{
			"scenePath": "scenes/duplicate.tscn",
			"nodeType":  "Sprite2D",
			"nodeName":  "Player",
		})

		requireToolText(t, session, "duplicate_scene_node", map[string]any{
			"scenePath": "scenes/duplicate.tscn",
			"nodePath":  "root/Player",
		})

		scene := readFile(t, filepath.Join(root, "scenes", "duplicate.tscn"))
		requireContains(t, scene, `[node name="Player"`, `[node name="Player2"`)
	})

	t.Run("duplicates a node with children", func(t *testing.T) {
		root, session := seedScene(t, "scenes/duplicate_tree.tscn", "Node2D")
		requireToolText(t, session, "add_node", map[string]any{
			"scenePath": "scenes/duplicate_tree.tscn",
			"nodeType":  "Node2D",
			"nodeName":  "Parent",
		})
		requireToolText(t, session, "add_node", map[string]any{
			"scenePath":      "scenes/duplicate_tree.tscn",
			"nodeType":       "Sprite2D",
			"nodeName":       "Child",
			"parentNodePath": "root/Parent",
		})

		requireToolText(t, session, "duplicate_scene_node", map[string]any{
			"scenePath": "scenes/duplicate_tree.tscn",
			"nodePath":  "root/Parent",
		})

		// The duplicate's child must be owned back to the scene root, or the
		// pack step drops it; its parent attribute proves it survived.
		scene := readFile(t, filepath.Join(root, "scenes", "duplicate_tree.tscn"))
		requireContains(t, scene, `[node name="Parent2"`, `parent="Parent2"`)
	})
}

// TestDuplicateSceneNodeErrors covers validation and engine failure messages.
func TestDuplicateSceneNodeErrors(t *testing.T) {
	t.Parallel()
	t.Run("missing params", func(t *testing.T) {
		session := connectProject(t, newProject(t))
		requireToolError(t, session, "duplicate_scene_node", map[string]any{},
			"scenePath and nodePath are required.")
	})

	t.Run("not a project", func(t *testing.T) {
		root := t.TempDir()
		session := connectProject(t, root)
		requireToolError(t, session, "duplicate_scene_node", map[string]any{
			"scenePath": "main.tscn",
			"nodePath":  "root",
		}, "Not a valid Godot project: "+root)
	})

	t.Run("missing scene", func(t *testing.T) {
		session := connectProject(t, newProject(t))
		requireToolError(t, session, "duplicate_scene_node", map[string]any{
			"scenePath": "missing.tscn",
			"nodePath":  "root",
		}, "Scene file does not exist: missing.tscn")
	})

	t.Run("duplicate root", func(t *testing.T) {
		_, session := seedScene(t, "scenes/structure.tscn", "Node2D")
		res := callTool(t, session, "duplicate_scene_node", map[string]any{
			"scenePath": "scenes/structure.tscn",
			"nodePath":  "root",
		})
		if !res.IsError {
			t.Fatal("duplicate_scene_node: expected an error result")
		}
		requireContains(t, firstText(t, res), "manage_scene_structure failed", "Cannot duplicate the root node")
	})
}
