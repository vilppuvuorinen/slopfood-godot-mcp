package e2e

import (
	"path/filepath"
	"testing"
)

// TestRemoveSceneNodeTool drives the real remove_scene_node tool against Godot
// and checks the saved scene no longer references the removed node.
func TestRemoveSceneNodeTool(t *testing.T) {
	t.Parallel()
	root, session := seedScene(t, "scenes/main.tscn", "Node2D")

	requireToolText(t, session, "add_node", map[string]any{
		"scenePath": "scenes/main.tscn",
		"nodeType":  "Sprite2D",
		"nodeName":  "Player",
	})

	requireToolText(t, session, "remove_scene_node", map[string]any{
		"scenePath": "scenes/main.tscn",
		"nodePath":  "root/Player",
	})

	requireNotContains(t, readFile(t, filepath.Join(root, "scenes", "main.tscn")), "Player")
}

// TestRemoveSceneNodeRoot verifies the operations script refuses to remove the
// scene root and that the refusal surfaces as a failed tool result.
func TestRemoveSceneNodeRoot(t *testing.T) {
	t.Parallel()
	_, session := seedScene(t, "scenes/main.tscn", "Node2D")

	res := callTool(t, session, "remove_scene_node", map[string]any{
		"scenePath": "scenes/main.tscn",
		"nodePath":  "root",
	})
	if !res.IsError {
		t.Fatal("remove_scene_node: expected an error result")
	}
	requireContains(t, firstText(t, res), "remove_node failed", "Failed to remove the root node")
}

// TestRemoveSceneNodeErrors covers the reference's validation and failure messages.
func TestRemoveSceneNodeErrors(t *testing.T) {
	t.Parallel()
	// Every project-backed case shares one warm project and session; only "not a
	// project" needs its own non-project directory.
	root := newProject(t)
	session := connectProject(t, root)

	t.Run("missing paths", func(t *testing.T) {
		requireToolError(t, session, "remove_scene_node", map[string]any{}, "scenePath and nodePath are required.")
	})

	t.Run("not a project", func(t *testing.T) {
		notProject := t.TempDir()
		notProjectSession := connectProject(t, notProject)
		requireToolError(t, notProjectSession, "remove_scene_node", map[string]any{
			"scenePath": "main.tscn",
			"nodePath":  "root/Player",
		}, "Not a valid Godot project: "+notProject)
	})

	t.Run("missing scene", func(t *testing.T) {
		requireToolError(t, session, "remove_scene_node", map[string]any{
			"scenePath": "missing.tscn",
			"nodePath":  "root/Player",
		}, "Scene file does not exist: missing.tscn")
	})

	t.Run("missing node", func(t *testing.T) {
		createScene(t, session, "scenes/errors_missing_node.tscn", "Node2D")

		res := callTool(t, session, "remove_scene_node", map[string]any{
			"scenePath": "scenes/errors_missing_node.tscn",
			"nodePath":  "root/Missing",
		})
		if !res.IsError {
			t.Fatal("remove_scene_node: expected an error result")
		}
		requireContains(t, firstText(t, res), "remove_node failed", "Node not found: root/Missing")
	})
}
