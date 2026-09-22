package e2e

import (
	"path/filepath"
	"testing"
)

// TestManageSceneSignalsSchema ports the reference's tool-definition
// expectations for manage_scene_signals over the real stdio surface.
func TestManageSceneSignalsSchema(t *testing.T) {
	schema := requireSchema(t, "manage_scene_signals", "List/add/remove signal connections in .tscn files")
	schema.requireRequired(t, "projectPath", "scenePath", "action")
	schema.requireStringProps(t, "projectPath", "scenePath", "action", "signalName", "sourcePath", "targetPath", "method")
	schema.requirePropCount(t, 7)
}

// TestManageSceneSignalsTool drives the real manage_scene_signals tool against
// Godot.
func TestManageSceneSignalsTool(t *testing.T) {
	session := connect(t)
	requireGodot(t, session)

	t.Run("lists signals", func(t *testing.T) {
		root := seedScene(t, session, "scenes/signals.tscn", "Node2D")

		text := requireToolText(t, session, "manage_scene_signals", map[string]any{
			"projectPath": root,
			"scenePath":   "scenes/signals.tscn",
			"action":      "list",
		})
		requirePrefix(t, text, "manage_scene_signals succeeded.")
	})

	t.Run("adds then removes a connection", func(t *testing.T) {
		root := seedScene(t, session, "scenes/connections.tscn", "Node2D")

		addText := requireToolText(t, session, "manage_scene_signals", map[string]any{
			"projectPath": root,
			"scenePath":   "scenes/connections.tscn",
			"action":      "add",
			"signalName":  "pressed",
			"sourcePath":  "root",
			"targetPath":  "root",
			"method":      "on_pressed",
		})
		requirePrefix(t, addText, "manage_scene_signals succeeded.")

		connection := `[connection signal="pressed" from="root" to="root" method="on_pressed"]`
		sceneFile := filepath.Join(root, "scenes", "connections.tscn")
		requireContains(t, readFile(t, sceneFile), connection)

		removeText := requireToolText(t, session, "manage_scene_signals", map[string]any{
			"projectPath": root,
			"scenePath":   "scenes/connections.tscn",
			"action":      "remove",
			"signalName":  "pressed",
		})
		requirePrefix(t, removeText, "manage_scene_signals succeeded.")
		requireNotContains(t, readFile(t, sceneFile), connection)
	})
}

// TestManageSceneSignalsErrors covers the reference's validation and failure
// messages.
func TestManageSceneSignalsErrors(t *testing.T) {
	session := connect(t)

	t.Run("missing params", func(t *testing.T) {
		requireToolError(t, session, "manage_scene_signals", map[string]any{}, "projectPath, scenePath, and action are required.")
	})

	t.Run("invalid path", func(t *testing.T) {
		requireToolError(t, session, "manage_scene_signals", map[string]any{
			"projectPath": "../escape",
			"scenePath":   "main.tscn",
			"action":      "list",
		}, "Invalid path.")
	})

	t.Run("not a project", func(t *testing.T) {
		root := t.TempDir()
		requireToolError(t, session, "manage_scene_signals", map[string]any{
			"projectPath": root,
			"scenePath":   "main.tscn",
			"action":      "list",
		}, "Not a valid Godot project: "+root)
	})

	t.Run("missing scene", func(t *testing.T) {
		root := t.TempDir()
		seedProject(t, root)
		requireToolError(t, session, "manage_scene_signals", map[string]any{
			"projectPath": root,
			"scenePath":   "missing.tscn",
			"action":      "list",
		}, "Scene file does not exist: missing.tscn")
	})
}
