package e2e

import (
	"path/filepath"
	"testing"
)

// TestMoveSceneNodeSchema pins the tool's input surface.
func TestMoveSceneNodeSchema(t *testing.T) {
	schema := requireSchema(t, "move_scene_node", "Move a node to a new parent in a .tscn scene")
	schema.requireRequired(t, "scenePath", "nodePath", "newParentPath")
	schema.requireStringProps(t, "scenePath", "nodePath", "newParentPath")
	schema.requirePropCount(t, 3)
}

// TestMoveSceneNodeTool drives the real move_scene_node tool against Godot.
func TestMoveSceneNodeTool(t *testing.T) {
	root, session := seedScene(t, "scenes/move.tscn", "Node2D")
	for _, name := range []string{"Player", "World"} {
		requireToolText(t, session, "add_node", map[string]any{
			"scenePath": "scenes/move.tscn",
			"nodeType":  "Node2D",
			"nodeName":  name,
		})
	}

	requireToolText(t, session, "move_scene_node", map[string]any{
		"scenePath":     "scenes/move.tscn",
		"nodePath":      "root/Player",
		"newParentPath": "root/World",
	})

	scene := readFile(t, filepath.Join(root, "scenes", "move.tscn"))
	requireContains(t, scene, `[node name="Player"`, `parent="World"`)
}

// TestMoveSceneNodeErrors covers validation and engine failure messages.
func TestMoveSceneNodeErrors(t *testing.T) {
	t.Run("missing params", func(t *testing.T) {
		session := connectProject(t, newProject(t))
		requireToolError(t, session, "move_scene_node", map[string]any{},
			"scenePath, nodePath, and newParentPath are required.")
	})

	t.Run("not a project", func(t *testing.T) {
		root := t.TempDir()
		session := connectProject(t, root)
		requireToolError(t, session, "move_scene_node", map[string]any{
			"scenePath":     "main.tscn",
			"nodePath":      "root",
			"newParentPath": "root/World",
		}, "Not a valid Godot project: "+root)
	})

	t.Run("missing scene", func(t *testing.T) {
		session := connectProject(t, newProject(t))
		requireToolError(t, session, "move_scene_node", map[string]any{
			"scenePath":     "missing.tscn",
			"nodePath":      "root",
			"newParentPath": "root/World",
		}, "Scene file does not exist: missing.tscn")
	})

	t.Run("move root", func(t *testing.T) {
		_, session := seedScene(t, "scenes/structure.tscn", "Node2D")
		requireToolText(t, session, "add_node", map[string]any{
			"scenePath": "scenes/structure.tscn",
			"nodeType":  "Node2D",
			"nodeName":  "World",
		})
		res := callTool(t, session, "move_scene_node", map[string]any{
			"scenePath":     "scenes/structure.tscn",
			"nodePath":      "root",
			"newParentPath": "root/World",
		})
		if !res.IsError {
			t.Fatal("move_scene_node: expected an error result")
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
		res := callTool(t, session, "move_scene_node", map[string]any{
			"scenePath":     "scenes/structure.tscn",
			"nodePath":      "root/Parent",
			"newParentPath": "root/Parent/Child",
		})
		if !res.IsError {
			t.Fatal("move_scene_node: expected an error result")
		}
		requireContains(t, firstText(t, res), "manage_scene_structure failed", "Cannot move a node into itself or one of its descendants")
	})

	t.Run("move to a missing parent", func(t *testing.T) {
		_, session := seedScene(t, "scenes/structure.tscn", "Node2D")
		requireToolText(t, session, "add_node", map[string]any{
			"scenePath": "scenes/structure.tscn",
			"nodeType":  "Node2D",
			"nodeName":  "Player",
		})
		res := callTool(t, session, "move_scene_node", map[string]any{
			"scenePath":     "scenes/structure.tscn",
			"nodePath":      "root/Player",
			"newParentPath": "root/Missing",
		})
		if !res.IsError {
			t.Fatal("move_scene_node: expected an error result")
		}
		requireContains(t, firstText(t, res), "manage_scene_structure failed", "New parent not found: root/Missing")
	})
}
