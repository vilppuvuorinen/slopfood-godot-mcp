package e2e

import (
	"path/filepath"
	"testing"
)

// TestRemoveSceneSignalTool drives the real remove_scene_signal tool against
// Godot.
func TestRemoveSceneSignalTool(t *testing.T) {
	t.Parallel()
	// The three subtests share one warm project and session; each owns its scene.
	root := newProject(t)
	session := connectProject(t, root)

	t.Run("adds then removes a connection", func(t *testing.T) {
		createScene(t, session, "scenes/connections.tscn", "Button")
		requireToolText(t, session, "add_scene_signal", map[string]any{
			"scenePath":  "scenes/connections.tscn",
			"signalName": "pressed",
			"sourcePath": "root",
			"targetPath": "root",
			"method":     "on_pressed",
		})

		connection := `[connection signal="pressed" from="." to="." method="on_pressed"]`
		sceneFile := filepath.Join(root, "scenes", "connections.tscn")
		requireContains(t, readFile(t, sceneFile), connection)

		removeText := requireToolText(t, session, "remove_scene_signal", map[string]any{
			"scenePath":  "scenes/connections.tscn",
			"signalName": "pressed",
			"sourcePath": "root",
			"targetPath": "root",
		})
		requirePrefix(t, removeText, "manage_scene_signals succeeded.")
		requireNotContains(t, readFile(t, sceneFile), connection)
	})

	t.Run("removes only the targeted connection", func(t *testing.T) {
		// Two connections share the "pressed" signal but differ in source and
		// method; removing one must leave the other untouched.
		createScene(t, session, "scenes/targeted.tscn", "Button")
		requireToolText(t, session, "add_node", map[string]any{
			"scenePath": "scenes/targeted.tscn",
			"nodeType":  "Button",
			"nodeName":  "Second",
		})

		for _, c := range []struct {
			source string
			method string
		}{
			{"root", "on_root"},
			{"root/Second", "on_second"},
		} {
			requireToolText(t, session, "add_scene_signal", map[string]any{
				"scenePath":  "scenes/targeted.tscn",
				"signalName": "pressed",
				"sourcePath": c.source,
				"targetPath": "root",
				"method":     c.method,
			})
		}

		keep := `[connection signal="pressed" from="." to="." method="on_root"]`
		drop := `[connection signal="pressed" from="Second" to="." method="on_second"]`
		sceneFile := filepath.Join(root, "scenes", "targeted.tscn")
		requireContains(t, readFile(t, sceneFile), keep, drop)

		requireToolText(t, session, "remove_scene_signal", map[string]any{
			"scenePath":  "scenes/targeted.tscn",
			"signalName": "pressed",
			"sourcePath": "root/Second",
			"targetPath": "root",
			"method":     "on_second",
		})
		scene := readFile(t, sceneFile)
		requireContains(t, scene, keep)
		requireNotContains(t, scene, drop)
	})

	t.Run("remove with no match leaves the connection", func(t *testing.T) {
		createScene(t, session, "scenes/nomatch.tscn", "Button")
		requireToolText(t, session, "add_scene_signal", map[string]any{
			"scenePath":  "scenes/nomatch.tscn",
			"signalName": "pressed",
			"sourcePath": "root",
			"targetPath": "root",
			"method":     "on_pressed",
		})
		connection := `[connection signal="pressed" from="." to="." method="on_pressed"]`

		// The method does not match the stored connection, so removal is a
		// successful no-op rather than a silent deletion of a sibling.
		text := requireToolText(t, session, "remove_scene_signal", map[string]any{
			"scenePath":  "scenes/nomatch.tscn",
			"signalName": "pressed",
			"sourcePath": "root",
			"targetPath": "root",
			"method":     "on_other",
		})
		requireContains(t, text, "No matching signal connection removed")
		requireContains(t, readFile(t, filepath.Join(root, "scenes", "nomatch.tscn")), connection)
	})
}

// TestRemoveSceneSignalErrors covers the tool's validation messages.
func TestRemoveSceneSignalErrors(t *testing.T) {
	t.Parallel()
	// Every project-backed case shares one warm project and session; only "not a
	// project" needs its own non-project directory.
	root := newProject(t)
	session := connectProject(t, root)

	t.Run("missing params", func(t *testing.T) {
		requireToolError(t, session, "remove_scene_signal", map[string]any{},
			"scenePath, signalName, sourcePath, and targetPath are required.")
	})

	t.Run("missing signal", func(t *testing.T) {
		requireToolError(t, session, "remove_scene_signal", map[string]any{
			"scenePath":  "main.tscn",
			"sourcePath": "root",
			"targetPath": "root",
		}, "scenePath, signalName, sourcePath, and targetPath are required.")
	})

	t.Run("not a project", func(t *testing.T) {
		notProject := t.TempDir()
		notProjectSession := connectProject(t, notProject)
		requireToolError(t, notProjectSession, "remove_scene_signal", map[string]any{
			"scenePath":  "main.tscn",
			"signalName": "released",
			"sourcePath": "root",
			"targetPath": "root",
		}, "Not a valid Godot project: "+notProject)
	})

	t.Run("missing scene", func(t *testing.T) {
		requireToolError(t, session, "remove_scene_signal", map[string]any{
			"scenePath":  "missing.tscn",
			"signalName": "released",
			"sourcePath": "root",
			"targetPath": "root",
		}, "Scene file does not exist: missing.tscn")
	})
}
