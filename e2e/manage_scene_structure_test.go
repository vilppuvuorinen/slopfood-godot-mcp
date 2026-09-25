package e2e

import (
	"path/filepath"
	"testing"
)

// TestManageSceneStructureSchema ports the reference's tool-definition
// expectations for manage_scene_structure over the real stdio surface.
func TestManageSceneStructureSchema(t *testing.T) {
	schema := requireSchema(t, "manage_scene_structure", "Rename/duplicate/move nodes within .tscn scenes")
	schema.requireRequired(t, "scenePath", "action", "nodePath")
	schema.requireStringProps(t, "scenePath", "action", "nodePath", "newName", "newParentPath")
	schema.requirePropCount(t, 5)
}

// TestManageSceneStructureTool drives the real manage_scene_structure tool
// against Godot for each action.
func TestManageSceneStructureTool(t *testing.T) {
	t.Run("renames a node", func(t *testing.T) {
		root, session := seedScene(t, "scenes/rename.tscn", "Node2D")
		requireToolText(t, session, "add_node", map[string]any{
			"scenePath": "scenes/rename.tscn",
			"nodeType":  "Sprite2D",
			"nodeName":  "Player",
		})

		requireToolText(t, session, "manage_scene_structure", map[string]any{
			"scenePath": "scenes/rename.tscn",
			"action":    "rename",
			"nodePath":  "root/Player",
			"newName":   "Hero",
		})

		scene := readFile(t, filepath.Join(root, "scenes", "rename.tscn"))
		requireContains(t, scene, `[node name="Hero"`)
	})

	t.Run("duplicates a node", func(t *testing.T) {
		root, session := seedScene(t, "scenes/duplicate.tscn", "Node2D")
		requireToolText(t, session, "add_node", map[string]any{
			"scenePath": "scenes/duplicate.tscn",
			"nodeType":  "Sprite2D",
			"nodeName":  "Player",
		})

		requireToolText(t, session, "manage_scene_structure", map[string]any{
			"scenePath": "scenes/duplicate.tscn",
			"action":    "duplicate",
			"nodePath":  "root/Player",
		})

		scene := readFile(t, filepath.Join(root, "scenes", "duplicate.tscn"))
		requireContains(t, scene, `[node name="Player"`, `[node name="Player2"`)
	})

	t.Run("moves a node", func(t *testing.T) {
		root, session := seedScene(t, "scenes/move.tscn", "Node2D")
		for _, name := range []string{"Player", "World"} {
			requireToolText(t, session, "add_node", map[string]any{
				"scenePath": "scenes/move.tscn",
				"nodeType":  "Node2D",
				"nodeName":  name,
			})
		}

		requireToolText(t, session, "manage_scene_structure", map[string]any{
			"scenePath":     "scenes/move.tscn",
			"action":        "move",
			"nodePath":      "root/Player",
			"newParentPath": "root/World",
		})

		scene := readFile(t, filepath.Join(root, "scenes", "move.tscn"))
		requireContains(t, scene, `[node name="Player"`, `parent="World"`)
	})
}

// TestManageSceneStructureErrors covers the reference's validation and failure
// messages.
func TestManageSceneStructureErrors(t *testing.T) {
	t.Run("missing params", func(t *testing.T) {
		session := connectProject(t, newProject(t))
		requireToolError(t, session, "manage_scene_structure", map[string]any{},
			"scenePath, action, and nodePath are required.")
	})

	t.Run("not a project", func(t *testing.T) {
		root := t.TempDir()
		session := connectProject(t, root)
		requireToolError(t, session, "manage_scene_structure", map[string]any{
			"scenePath": "main.tscn",
			"action":    "rename",
			"nodePath":  "root",
		}, "Not a valid Godot project: "+root)
	})

	t.Run("missing scene", func(t *testing.T) {
		session := connectProject(t, newProject(t))
		requireToolError(t, session, "manage_scene_structure", map[string]any{
			"scenePath": "missing.tscn",
			"action":    "rename",
			"nodePath":  "root",
		}, "Scene file does not exist: missing.tscn")
	})

	t.Run("duplicate root", func(t *testing.T) {
		_, session := seedScene(t, "scenes/structure.tscn", "Node2D")
		res := callTool(t, session, "manage_scene_structure", map[string]any{
			"scenePath": "scenes/structure.tscn",
			"action":    "duplicate",
			"nodePath":  "root",
		})
		if !res.IsError {
			t.Fatal("manage_scene_structure: expected an error result")
		}
		requireContains(t, firstText(t, res), "manage_scene_structure failed", "Cannot duplicate the root node")
	})

	t.Run("move root", func(t *testing.T) {
		_, session := seedScene(t, "scenes/structure.tscn", "Node2D")
		requireToolText(t, session, "add_node", map[string]any{
			"scenePath": "scenes/structure.tscn",
			"nodeType":  "Node2D",
			"nodeName":  "World",
		})
		res := callTool(t, session, "manage_scene_structure", map[string]any{
			"scenePath":     "scenes/structure.tscn",
			"action":        "move",
			"nodePath":      "root",
			"newParentPath": "root/World",
		})
		if !res.IsError {
			t.Fatal("manage_scene_structure: expected an error result")
		}
		requireContains(t, firstText(t, res), "manage_scene_structure failed", "Cannot move the root node")
	})

	t.Run("move into a descendant", func(t *testing.T) {
		_, session := seedScene(t, "scenes/structure.tscn", "Node2D")
		requireToolText(t, session, "add_node", map[string]any{
			"scenePath": "scenes/structure.tscn",
			"nodeType":  "Node2D",
			"nodeName":  "Parent",
		})
		requireToolText(t, session, "add_node", map[string]any{
			"scenePath":      "scenes/structure.tscn",
			"nodeType":       "Node2D",
			"nodeName":       "Child",
			"parentNodePath": "root/Parent",
		})
		res := callTool(t, session, "manage_scene_structure", map[string]any{
			"scenePath":     "scenes/structure.tscn",
			"action":        "move",
			"nodePath":      "root/Parent",
			"newParentPath": "root/Parent/Child",
		})
		if !res.IsError {
			t.Fatal("manage_scene_structure: expected an error result")
		}
		requireContains(t, firstText(t, res), "manage_scene_structure failed", "Cannot move a node into itself or one of its descendants")
	})

	t.Run("rename without newName", func(t *testing.T) {
		_, session := seedScene(t, "scenes/structure.tscn", "Node2D")
		res := callTool(t, session, "manage_scene_structure", map[string]any{
			"scenePath": "scenes/structure.tscn",
			"action":    "rename",
			"nodePath":  "root",
		})
		if !res.IsError {
			t.Fatal("manage_scene_structure: expected an error result")
		}
		requireContains(t, firstText(t, res), "manage_scene_structure failed", "newName is required for rename")
	})

	t.Run("move without newParentPath", func(t *testing.T) {
		_, session := seedScene(t, "scenes/structure.tscn", "Node2D")
		res := callTool(t, session, "manage_scene_structure", map[string]any{
			"scenePath": "scenes/structure.tscn",
			"action":    "move",
			"nodePath":  "root",
		})
		if !res.IsError {
			t.Fatal("manage_scene_structure: expected an error result")
		}
		requireContains(t, firstText(t, res), "manage_scene_structure failed", "newParentPath is required for move")
	})

	t.Run("unknown action", func(t *testing.T) {
		_, session := seedScene(t, "scenes/structure.tscn", "Node2D")
		res := callTool(t, session, "manage_scene_structure", map[string]any{
			"scenePath": "scenes/structure.tscn",
			"action":    "bogus",
			"nodePath":  "root",
		})
		if !res.IsError {
			t.Fatal("manage_scene_structure: expected an error result")
		}
		requireContains(t, firstText(t, res), "manage_scene_structure failed", "Unknown manage_scene_structure action: bogus")
	})
}
