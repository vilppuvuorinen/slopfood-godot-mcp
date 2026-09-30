package e2e

import (
	"path/filepath"
	"testing"
)

func TestDuplicateSceneNodeTool(t *testing.T) {
	t.Parallel()
	root := newProject(t)
	session := connectProject(t, root)

	t.Run("duplicates a node", func(t *testing.T) {
		createScene(t, session, "scenes/duplicate.tscn", "Node2D")
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
		createScene(t, session, "scenes/duplicate_tree.tscn", "Node2D")
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

		// The duplicate's child must be owned back to the scene root, or the pack step drops it.
		scene := readFile(t, filepath.Join(root, "scenes", "duplicate_tree.tscn"))
		requireContains(t, scene, `[node name="Parent2"`, `parent="Parent2"`)
	})
}

func TestDuplicateSceneNodeErrors(t *testing.T) {
	t.Parallel()
	root := newProject(t)
	session := connectProject(t, root)

	t.Run("missing params", func(t *testing.T) {
		requireToolError(t, session, "duplicate_scene_node", map[string]any{},
			"scenePath and nodePath are required.")
	})

	t.Run("not a project", func(t *testing.T) {
		notProject := t.TempDir()
		notProjectSession := connectProject(t, notProject)
		requireToolError(t, notProjectSession, "duplicate_scene_node", map[string]any{
			"scenePath": "main.tscn",
			"nodePath":  "root",
		}, "Not a valid Godot project: "+notProject)
	})

	t.Run("missing scene", func(t *testing.T) {
		requireToolError(t, session, "duplicate_scene_node", map[string]any{
			"scenePath": "missing.tscn",
			"nodePath":  "root",
		}, "Scene file does not exist: missing.tscn")
	})

	t.Run("duplicate root", func(t *testing.T) {
		createScene(t, session, "scenes/structure.tscn", "Node2D")
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
