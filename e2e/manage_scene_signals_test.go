package e2e

import (
	"encoding/json"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// TestManageSceneSignalsSchema ports the reference's tool-definition
// expectations for manage_scene_signals over the real stdio surface.
func TestManageSceneSignalsSchema(t *testing.T) {
	schema := requireSchema(t, "manage_scene_signals", "Add or remove signal connections in .tscn files")
	schema.requireRequired(t, "scenePath", "action")
	schema.requireStringProps(t, "scenePath", "action", "signalName", "sourcePath", "targetPath", "method")
	schema.requirePropCount(t, 6)
}

// TestManageSceneSignalsTool drives the real manage_scene_signals tool against
// Godot.
func TestManageSceneSignalsTool(t *testing.T) {
	t.Run("lists signals", func(t *testing.T) {
		_, session := seedScene(t, "scenes/signals.tscn", "Button")

		empty := requireToolText(t, session, "read_scene_signals", map[string]any{
			"scenePath": "scenes/signals.tscn",
		})
		requireContains(t, empty, `"connections"`)

		requireToolText(t, session, "manage_scene_signals", map[string]any{
			"scenePath":  "scenes/signals.tscn",
			"action":     "add",
			"signalName": "pressed",
			"sourcePath": "root",
			"targetPath": "root",
			"method":     "on_pressed",
		})
		text := requireToolText(t, session, "read_scene_signals", map[string]any{
			"scenePath": "scenes/signals.tscn",
		})
		var decoded struct {
			Connections []struct {
				Signal string `json:"signal"`
				From   string `json:"from"`
				To     string `json:"to"`
				Method string `json:"method"`
			} `json:"connections"`
		}
		if err := json.Unmarshal([]byte(text), &decoded); err != nil {
			t.Fatalf("decode %q: %v", text, err)
		}
		want := []struct {
			Signal string `json:"signal"`
			From   string `json:"from"`
			To     string `json:"to"`
			Method string `json:"method"`
		}{{"pressed", ".", ".", "on_pressed"}}
		if !reflect.DeepEqual(decoded.Connections, want) {
			t.Errorf("connections = %+v, want %+v", decoded.Connections, want)
		}
	})

	t.Run("adds then removes a connection", func(t *testing.T) {
		// Button carries the "pressed" signal; connecting it must not make the
		// engine log an error when the saved scene is re-instantiated.
		root, session := seedScene(t, "scenes/connections.tscn", "Button")

		addText := requireToolText(t, session, "manage_scene_signals", map[string]any{
			"scenePath":  "scenes/connections.tscn",
			"action":     "add",
			"signalName": "pressed",
			"sourcePath": "root",
			"targetPath": "root",
			"method":     "on_pressed",
		})
		requirePrefix(t, addText, "manage_scene_signals succeeded.")

		connection := `[connection signal="pressed" from="." to="." method="on_pressed"]`
		sceneFile := filepath.Join(root, "scenes", "connections.tscn")
		requireContains(t, readFile(t, sceneFile), connection)

		removeText := requireToolText(t, session, "manage_scene_signals", map[string]any{
			"scenePath":  "scenes/connections.tscn",
			"action":     "remove",
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
		root, session := seedScene(t, "scenes/targeted.tscn", "Button")
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
			requireToolText(t, session, "manage_scene_signals", map[string]any{
				"scenePath":  "scenes/targeted.tscn",
				"action":     "add",
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

		requireToolText(t, session, "manage_scene_signals", map[string]any{
			"scenePath":  "scenes/targeted.tscn",
			"action":     "remove",
			"signalName": "pressed",
			"sourcePath": "root/Second",
			"targetPath": "root",
			"method":     "on_second",
		})
		scene := readFile(t, sceneFile)
		requireContains(t, scene, keep)
		requireNotContains(t, scene, drop)
	})

	t.Run("duplicate add is idempotent", func(t *testing.T) {
		root, session := seedScene(t, "scenes/duplicate.tscn", "Button")
		args := map[string]any{
			"scenePath":  "scenes/duplicate.tscn",
			"action":     "add",
			"signalName": "pressed",
			"sourcePath": "root",
			"targetPath": "root",
			"method":     "on_pressed",
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
		root, session := seedScene(t, "scenes/keep.tscn", "Button")
		connection := `[connection signal="pressed" from="." to="." method="on_pressed"]`
		requireToolText(t, session, "manage_scene_signals", map[string]any{
			"scenePath":  "scenes/keep.tscn",
			"action":     "add",
			"signalName": "pressed",
			"sourcePath": "root",
			"targetPath": "root",
			"method":     "on_pressed",
		})

		requireToolError(t, session, "manage_scene_signals", map[string]any{
			"scenePath": "scenes/keep.tscn",
			"action":    "remove",
		}, "signalName, sourcePath, and targetPath are required for remove.")
		requireContains(t, readFile(t, filepath.Join(root, "scenes", "keep.tscn")), connection)
	})

	t.Run("rejects missing source node", func(t *testing.T) {
		_, session := seedScene(t, "scenes/dangling.tscn", "Node2D")
		res := callTool(t, session, "manage_scene_signals", map[string]any{
			"scenePath":  "scenes/dangling.tscn",
			"action":     "add",
			"signalName": "pressed",
			"sourcePath": "root/Missing",
			"targetPath": "root",
			"method":     "on_pressed",
		})
		if !res.IsError {
			t.Fatalf("expected an error result: %s", firstText(t, res))
		}
		requireContains(t, firstText(t, res), "Source node not found: root/Missing")
	})

	t.Run("rejects missing target node", func(t *testing.T) {
		_, session := seedScene(t, "scenes/dangling_target.tscn", "Button")
		res := callTool(t, session, "manage_scene_signals", map[string]any{
			"scenePath":  "scenes/dangling_target.tscn",
			"action":     "add",
			"signalName": "pressed",
			"sourcePath": "root",
			"targetPath": "root/Missing",
			"method":     "on_pressed",
		})
		if !res.IsError {
			t.Fatalf("expected an error result: %s", firstText(t, res))
		}
		requireContains(t, firstText(t, res), "Target node not found: root/Missing")
	})

	t.Run("remove with no match leaves the connection", func(t *testing.T) {
		root, session := seedScene(t, "scenes/nomatch.tscn", "Button")
		requireToolText(t, session, "manage_scene_signals", map[string]any{
			"scenePath":  "scenes/nomatch.tscn",
			"action":     "add",
			"signalName": "pressed",
			"sourcePath": "root",
			"targetPath": "root",
			"method":     "on_pressed",
		})
		connection := `[connection signal="pressed" from="." to="." method="on_pressed"]`

		// The method does not match the stored connection, so removal is a
		// successful no-op rather than a silent deletion of a sibling.
		text := requireToolText(t, session, "manage_scene_signals", map[string]any{
			"scenePath":  "scenes/nomatch.tscn",
			"action":     "remove",
			"signalName": "pressed",
			"sourcePath": "root",
			"targetPath": "root",
			"method":     "on_other",
		})
		requireContains(t, text, "No matching signal connection removed")
		requireContains(t, readFile(t, filepath.Join(root, "scenes", "nomatch.tscn")), connection)
	})

	t.Run("rejects unsafe field", func(t *testing.T) {
		for name, field := range map[string]string{
			"quote":     `pr"essed`,
			"backslash": `on\pressed`,
			"newline":   "on\npressed",
		} {
			t.Run(name, func(t *testing.T) {
				_, session := seedScene(t, "scenes/unsafe.tscn", "Node2D")
				res := callTool(t, session, "manage_scene_signals", map[string]any{
					"scenePath":  "scenes/unsafe.tscn",
					"action":     "add",
					"signalName": "pressed",
					"sourcePath": "root",
					"targetPath": "root",
					"method":     field,
				})
				if !res.IsError {
					t.Fatalf("expected an error result: %s", firstText(t, res))
				}
				requireContains(t, firstText(t, res), "cannot contain quotes, backslashes, or newlines")
			})
		}
	})
}

// TestManageSceneSignalsErrors covers the reference's validation and failure
// messages.
func TestManageSceneSignalsErrors(t *testing.T) {
	t.Run("missing params", func(t *testing.T) {
		session := connectProject(t, newProject(t))
		requireToolError(t, session, "manage_scene_signals", map[string]any{}, "scenePath and action are required.")
	})

	t.Run("not a project", func(t *testing.T) {
		root := t.TempDir()
		session := connectProject(t, root)
		requireToolError(t, session, "read_scene_signals", map[string]any{
			"scenePath": "main.tscn",
		}, "Not a valid Godot project: "+root)
	})

	t.Run("missing scene", func(t *testing.T) {
		session := connectProject(t, newProject(t))
		requireToolError(t, session, "read_scene_signals", map[string]any{
			"scenePath": "missing.tscn",
		}, "Scene file does not exist: missing.tscn")
	})

	t.Run("add missing fields", func(t *testing.T) {
		session := connectProject(t, newProject(t))
		requireToolError(t, session, "manage_scene_signals", map[string]any{
			"scenePath":  "main.tscn",
			"action":     "add",
			"signalName": "pressed",
		}, "signalName, sourcePath, targetPath, and method are required for add.")
	})

	t.Run("remove missing signal", func(t *testing.T) {
		session := connectProject(t, newProject(t))
		requireToolError(t, session, "manage_scene_signals", map[string]any{
			"scenePath": "main.tscn",
			"action":    "remove",
		}, "signalName, sourcePath, and targetPath are required for remove.")
	})

	t.Run("unknown action", func(t *testing.T) {
		session := connectProject(t, newProject(t))
		requireToolError(t, session, "manage_scene_signals", map[string]any{
			"scenePath": "main.tscn",
			"action":    "frobnicate",
		}, "Unknown action: frobnicate. Use add or remove.")
	})
}
