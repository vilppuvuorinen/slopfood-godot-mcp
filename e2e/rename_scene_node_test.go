package e2e

import (
	"path/filepath"
	"testing"
)

// TestRenameSceneNodeTool drives the real rename_scene_node tool against Godot.
func TestRenameSceneNodeTool(t *testing.T) {
	t.Parallel()
	root, session := seedScene(t, "scenes/rename.tscn", "Node2D")
	requireToolText(t, session, "add_node", map[string]any{
		"scenePath": "scenes/rename.tscn",
		"nodeType":  "Sprite2D",
		"nodeName":  "Player",
	})

	requireToolText(t, session, "rename_scene_node", map[string]any{
		"scenePath": "scenes/rename.tscn",
		"nodePath":  "root/Player",
		"newName":   "Hero",
	})

	scene := readFile(t, filepath.Join(root, "scenes", "rename.tscn"))
	requireContains(t, scene, `[node name="Hero"`)
}

// TestRenameSceneNodeErrors covers validation and engine failure messages.
func TestRenameSceneNodeErrors(t *testing.T) {
	t.Parallel()
	// Every project-backed case shares one warm project and session; only "not a
	// project" needs its own non-project directory.
	root := newProject(t)
	session := connectProject(t, root)

	t.Run("missing params", func(t *testing.T) {
		requireToolError(t, session, "rename_scene_node", map[string]any{},
			"scenePath, nodePath, and newName are required.")
	})

	t.Run("not a project", func(t *testing.T) {
		notProject := t.TempDir()
		notProjectSession := connectProject(t, notProject)
		requireToolError(t, notProjectSession, "rename_scene_node", map[string]any{
			"scenePath": "main.tscn",
			"nodePath":  "root",
			"newName":   "Hero",
		}, "Not a valid Godot project: "+notProject)
	})

	t.Run("missing scene", func(t *testing.T) {
		requireToolError(t, session, "rename_scene_node", map[string]any{
			"scenePath": "missing.tscn",
			"nodePath":  "root",
			"newName":   "Hero",
		}, "Scene file does not exist: missing.tscn")
	})

	t.Run("missing node", func(t *testing.T) {
		createScene(t, session, "scenes/rename_missing_node.tscn", "Node2D")
		res := callTool(t, session, "rename_scene_node", map[string]any{
			"scenePath": "scenes/rename_missing_node.tscn",
			"nodePath":  "root/Missing",
			"newName":   "Hero",
		})
		if !res.IsError {
			t.Fatal("rename_scene_node: expected an error result")
		}
		requireContains(t, firstText(t, res), "manage_scene_structure failed", "Node not found: root/Missing")
	})
}
