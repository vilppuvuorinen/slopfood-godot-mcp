package e2e

import (
	"path/filepath"
	"testing"
)

// TestManageSceneStructureSchema ports the reference's tool-definition
// expectations for manage_scene_structure over the real stdio surface.
func TestManageSceneStructureSchema(t *testing.T) {
	schema := requireSchema(t, "manage_scene_structure", "Rename/duplicate/move nodes within .tscn scenes")
	schema.requireRequired(t, "projectPath", "scenePath", "action", "nodePath")
	schema.requireStringProps(t, "projectPath", "scenePath", "action", "nodePath", "newName", "newParentPath")
	schema.requirePropCount(t, 6)
}

// TestManageSceneStructureTool drives the real manage_scene_structure tool
// against Godot for each action.
func TestManageSceneStructureTool(t *testing.T) {
	session := connect(t)
	requireGodot(t, session)

	t.Run("renames a node", func(t *testing.T) {
		root := seedScene(t, session, "scenes/rename.tscn", "Node2D")
		requireToolText(t, session, "add_node", map[string]any{
			"projectPath": root,
			"scenePath":   "scenes/rename.tscn",
			"nodeType":    "Sprite2D",
			"nodeName":    "Player",
		})

		requireToolText(t, session, "manage_scene_structure", map[string]any{
			"projectPath": root,
			"scenePath":   "scenes/rename.tscn",
			"action":      "rename",
			"nodePath":    "root/Player",
			"newName":     "Hero",
		})

		scene := readFile(t, filepath.Join(root, "scenes", "rename.tscn"))
		requireContains(t, scene, `[node name="Hero"`)
	})

	t.Run("duplicates a node", func(t *testing.T) {
		root := seedScene(t, session, "scenes/duplicate.tscn", "Node2D")
		requireToolText(t, session, "add_node", map[string]any{
			"projectPath": root,
			"scenePath":   "scenes/duplicate.tscn",
			"nodeType":    "Sprite2D",
			"nodeName":    "Player",
		})

		requireToolText(t, session, "manage_scene_structure", map[string]any{
			"projectPath": root,
			"scenePath":   "scenes/duplicate.tscn",
			"action":      "duplicate",
			"nodePath":    "root/Player",
		})

		scene := readFile(t, filepath.Join(root, "scenes", "duplicate.tscn"))
		requireContains(t, scene, `[node name="Player"`, `[node name="Player2"`)
	})

	t.Run("moves a node", func(t *testing.T) {
		root := seedScene(t, session, "scenes/move.tscn", "Node2D")
		for _, name := range []string{"Player", "World"} {
			requireToolText(t, session, "add_node", map[string]any{
				"projectPath": root,
				"scenePath":   "scenes/move.tscn",
				"nodeType":    "Node2D",
				"nodeName":    name,
			})
		}

		requireToolText(t, session, "manage_scene_structure", map[string]any{
			"projectPath":   root,
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
	session := connect(t)

	t.Run("missing params", func(t *testing.T) {
		requireToolError(t, session, "manage_scene_structure", map[string]any{},
			"projectPath, scenePath, action, and nodePath are required.")
	})

	t.Run("invalid path", func(t *testing.T) {
		requireToolError(t, session, "manage_scene_structure", map[string]any{
			"projectPath": "../escape",
			"scenePath":   "main.tscn",
			"action":      "rename",
			"nodePath":    "root",
		}, "Invalid path.")
	})

	t.Run("not a project", func(t *testing.T) {
		root := t.TempDir()
		requireToolError(t, session, "manage_scene_structure", map[string]any{
			"projectPath": root,
			"scenePath":   "main.tscn",
			"action":      "rename",
			"nodePath":    "root",
		}, "Not a valid Godot project: "+root)
	})

	t.Run("missing scene", func(t *testing.T) {
		root := t.TempDir()
		seedProject(t, root)
		requireToolError(t, session, "manage_scene_structure", map[string]any{
			"projectPath": root,
			"scenePath":   "missing.tscn",
			"action":      "rename",
			"nodePath":    "root",
		}, "Scene file does not exist: missing.tscn")
	})
}
