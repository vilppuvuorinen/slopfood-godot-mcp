package e2e

import (
	"path/filepath"
	"strings"
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
		requireContains(t, text, `"connections"`)
	})

	t.Run("adds then removes a connection", func(t *testing.T) {
		// Button carries the "pressed" signal; connecting it must not make the
		// engine log an error when the saved scene is re-instantiated.
		root := seedScene(t, session, "scenes/connections.tscn", "Button")

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

		connection := `[connection signal="pressed" from="." to="." method="on_pressed"]`
		sceneFile := filepath.Join(root, "scenes", "connections.tscn")
		requireContains(t, readFile(t, sceneFile), connection)

		removeText := requireToolText(t, session, "manage_scene_signals", map[string]any{
			"projectPath": root,
			"scenePath":   "scenes/connections.tscn",
			"action":      "remove",
			"signalName":  "pressed",
			"sourcePath":  "root",
			"targetPath":  "root",
		})
		requirePrefix(t, removeText, "manage_scene_signals succeeded.")
		requireNotContains(t, readFile(t, sceneFile), connection)
	})

	t.Run("removes only the targeted connection", func(t *testing.T) {
		// Two connections share the "pressed" signal but differ in source and
		// method; removing one must leave the other untouched.
		root := seedScene(t, session, "scenes/targeted.tscn", "Button")
		requireToolText(t, session, "add_node", map[string]any{
			"projectPath": root,
			"scenePath":   "scenes/targeted.tscn",
			"nodeType":    "Button",
			"nodeName":    "Second",
		})

		for _, c := range []struct {
			source string
			method string
		}{
			{"root", "on_root"},
			{"root/Second", "on_second"},
		} {
			requireToolText(t, session, "manage_scene_signals", map[string]any{
				"projectPath": root,
				"scenePath":   "scenes/targeted.tscn",
				"action":      "add",
				"signalName":  "pressed",
				"sourcePath":  c.source,
				"targetPath":  "root",
				"method":      c.method,
			})
		}

		keep := `[connection signal="pressed" from="." to="." method="on_root"]`
		drop := `[connection signal="pressed" from="Second" to="." method="on_second"]`
		sceneFile := filepath.Join(root, "scenes", "targeted.tscn")
		requireContains(t, readFile(t, sceneFile), keep, drop)

		requireToolText(t, session, "manage_scene_signals", map[string]any{
			"projectPath": root,
			"scenePath":   "scenes/targeted.tscn",
			"action":      "remove",
			"signalName":  "pressed",
			"sourcePath":  "root/Second",
			"targetPath":  "root",
			"method":      "on_second",
		})
		scene := readFile(t, sceneFile)
		requireContains(t, scene, keep)
		requireNotContains(t, scene, drop)
	})

	t.Run("duplicate add is idempotent", func(t *testing.T) {
		root := seedScene(t, session, "scenes/duplicate.tscn", "Button")
		args := map[string]any{
			"projectPath": root,
			"scenePath":   "scenes/duplicate.tscn",
			"action":      "add",
			"signalName":  "pressed",
			"sourcePath":  "root",
			"targetPath":  "root",
			"method":      "on_pressed",
		}
		requireToolText(t, session, "manage_scene_signals", args)
		requireToolText(t, session, "manage_scene_signals", args)

		connection := `[connection signal="pressed" from="." to="." method="on_pressed"]`
		scene := readFile(t, filepath.Join(root, "scenes", "duplicate.tscn"))
		if got := strings.Count(scene, connection); got != 1 {
			t.Errorf("connection count = %d, want 1 in:\n%s", got, scene)
		}
	})

	t.Run("remove without signal keeps connections", func(t *testing.T) {
		root := seedScene(t, session, "scenes/keep.tscn", "Button")
		connection := `[connection signal="pressed" from="." to="." method="on_pressed"]`
		requireToolText(t, session, "manage_scene_signals", map[string]any{
			"projectPath": root,
			"scenePath":   "scenes/keep.tscn",
			"action":      "add",
			"signalName":  "pressed",
			"sourcePath":  "root",
			"targetPath":  "root",
			"method":      "on_pressed",
		})

		requireToolError(t, session, "manage_scene_signals", map[string]any{
			"projectPath": root,
			"scenePath":   "scenes/keep.tscn",
			"action":      "remove",
		}, "signalName, sourcePath, and targetPath are required for remove.")
		requireContains(t, readFile(t, filepath.Join(root, "scenes", "keep.tscn")), connection)
	})

	t.Run("rejects missing source node", func(t *testing.T) {
		root := seedScene(t, session, "scenes/dangling.tscn", "Node2D")
		res := callTool(t, session, "manage_scene_signals", map[string]any{
			"projectPath": root,
			"scenePath":   "scenes/dangling.tscn",
			"action":      "add",
			"signalName":  "pressed",
			"sourcePath":  "root/Missing",
			"targetPath":  "root",
			"method":      "on_pressed",
		})
		if !res.IsError {
			t.Fatalf("expected an error result: %s", firstText(t, res))
		}
		requireContains(t, firstText(t, res), "Source node not found: root/Missing")
	})

	t.Run("rejects unsafe field", func(t *testing.T) {
		root := seedScene(t, session, "scenes/unsafe.tscn", "Node2D")
		res := callTool(t, session, "manage_scene_signals", map[string]any{
			"projectPath": root,
			"scenePath":   "scenes/unsafe.tscn",
			"action":      "add",
			"signalName":  `pr"essed`,
			"sourcePath":  "root",
			"targetPath":  "root",
			"method":      "on_pressed",
		})
		if !res.IsError {
			t.Fatalf("expected an error result: %s", firstText(t, res))
		}
		requireContains(t, firstText(t, res), "cannot contain quotes")
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

	t.Run("add missing fields", func(t *testing.T) {
		root := t.TempDir()
		seedProject(t, root)
		requireToolError(t, session, "manage_scene_signals", map[string]any{
			"projectPath": root,
			"scenePath":   "main.tscn",
			"action":      "add",
			"signalName":  "pressed",
		}, "signalName, sourcePath, targetPath, and method are required for add.")
	})

	t.Run("remove missing signal", func(t *testing.T) {
		root := t.TempDir()
		seedProject(t, root)
		requireToolError(t, session, "manage_scene_signals", map[string]any{
			"projectPath": root,
			"scenePath":   "main.tscn",
			"action":      "remove",
		}, "signalName, sourcePath, and targetPath are required for remove.")
	})

	t.Run("unknown action", func(t *testing.T) {
		root := t.TempDir()
		seedProject(t, root)
		requireToolError(t, session, "manage_scene_signals", map[string]any{
			"projectPath": root,
			"scenePath":   "main.tscn",
			"action":      "frobnicate",
		}, "Unknown action: frobnicate. Use list, add, or remove.")
	})
}
