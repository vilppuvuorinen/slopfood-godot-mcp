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
}
