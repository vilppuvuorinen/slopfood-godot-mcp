package e2e

import (
	"encoding/json"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestAddSceneSignalTool(t *testing.T) {
	t.Parallel()
	root := newProject(t)
	session := connectProject(t, root)

	t.Run("adds a connection", func(t *testing.T) {
		createScene(t, session, "scenes/signals.tscn", "Button")

		empty := requireToolText(t, session, "read_scene", map[string]any{
			"scenePath": "scenes/signals.tscn",
		})
		requireContains(t, empty, `"connections"`)

		requireToolText(t, session, "add_scene_signal", map[string]any{
			"scenePath":  "scenes/signals.tscn",
			"signalName": "pressed",
			"sourcePath": "root",
			"targetPath": "root",
			"method":     "on_pressed",
		})
		text := requireToolText(t, session, "read_scene", map[string]any{
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

	t.Run("writes the connection to the scene", func(t *testing.T) {
		createScene(t, session, "scenes/connections.tscn", "Button")

		addText := requireToolText(t, session, "add_scene_signal", map[string]any{
			"scenePath":  "scenes/connections.tscn",
			"signalName": "pressed",
			"sourcePath": "root",
			"targetPath": "root",
			"method":     "on_pressed",
		})
		requirePrefix(t, addText, "manage_scene_signals succeeded.")

		connection := `[connection signal="pressed" from="." to="." method="on_pressed"]`
		requireContains(t, readFile(t, filepath.Join(root, "scenes", "connections.tscn")), connection)
	})

	t.Run("duplicate add is idempotent", func(t *testing.T) {
		createScene(t, session, "scenes/duplicate.tscn", "Button")
		args := map[string]any{
			"scenePath":  "scenes/duplicate.tscn",
			"signalName": "pressed",
			"sourcePath": "root",
			"targetPath": "root",
			"method":     "on_pressed",
		}
		requireToolText(t, session, "add_scene_signal", args)
		requireToolText(t, session, "add_scene_signal", args)

		connection := `[connection signal="pressed" from="." to="." method="on_pressed"]`
		scene := readFile(t, filepath.Join(root, "scenes", "duplicate.tscn"))
		if got := strings.Count(scene, connection); got != 1 {
			t.Errorf("connection count = %d, want 1 in:\n%s", got, scene)
		}
	})

	t.Run("rejects missing source node", func(t *testing.T) {
		createScene(t, session, "scenes/dangling.tscn", "Node2D")
		res := callTool(t, session, "add_scene_signal", map[string]any{
			"scenePath":  "scenes/dangling.tscn",
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
		createScene(t, session, "scenes/dangling_target.tscn", "Button")
		res := callTool(t, session, "add_scene_signal", map[string]any{
			"scenePath":  "scenes/dangling_target.tscn",
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

	t.Run("rejects unsafe field", func(t *testing.T) {
		for name, field := range map[string]string{
			"quote":     `pr"essed`,
			"backslash": `on\pressed`,
			"newline":   "on\npressed",
		} {
			t.Run(name, func(t *testing.T) {
				scenePath := "scenes/unsafe_" + name + ".tscn"
				createScene(t, session, scenePath, "Node2D")
				res := callTool(t, session, "add_scene_signal", map[string]any{
					"scenePath":  scenePath,
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

func TestAddSceneSignalErrors(t *testing.T) {
	t.Parallel()
	root := newProject(t)
	session := connectProject(t, root)

	t.Run("missing params", func(t *testing.T) {
		requireToolError(t, session, "add_scene_signal", map[string]any{},
			"scenePath, signalName, sourcePath, targetPath, and method are required.")
	})

	t.Run("missing method", func(t *testing.T) {
		requireToolError(t, session, "add_scene_signal", map[string]any{
			"scenePath":  "main.tscn",
			"signalName": "pressed",
			"sourcePath": "root",
			"targetPath": "root",
		}, "scenePath, signalName, sourcePath, targetPath, and method are required.")
	})

	t.Run("not a project", func(t *testing.T) {
		notProject := t.TempDir()
		notProjectSession := connectProject(t, notProject)
		requireToolError(t, notProjectSession, "add_scene_signal", map[string]any{
			"scenePath":  "main.tscn",
			"signalName": "pressed",
			"sourcePath": "root",
			"targetPath": "root",
			"method":     "on_pressed",
		}, "Not a valid Godot project: "+notProject)
	})

	t.Run("missing scene", func(t *testing.T) {
		requireToolError(t, session, "add_scene_signal", map[string]any{
			"scenePath":  "missing.tscn",
			"signalName": "pressed",
			"sourcePath": "root",
			"targetPath": "root",
			"method":     "on_pressed",
		}, "Scene file does not exist: missing.tscn")
	})
}
