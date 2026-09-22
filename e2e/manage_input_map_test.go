package e2e

import (
	"encoding/json"
	"path/filepath"
	"testing"
)

// TestManageInputMapSchema ports the reference's tool-definition expectations
// for manage_input_map over the real stdio surface.
func TestManageInputMapSchema(t *testing.T) {
	schema := requireSchema(t, "manage_input_map", "Add, remove, or list input actions and bindings")
	schema.requireRequired(t, "projectPath", "action")
	schema.requirePropCount(t, 5)
	schema.requireStringProps(t, "projectPath", "action", "actionName", "key")
	schema.requireType(t, "deadzone", "number")
}

// TestManageInputMapTool drives the real manage_input_map tool against seeded
// project.godot files.
func TestManageInputMapTool(t *testing.T) {
	session := connect(t)

	t.Run("list", func(t *testing.T) {
		root := t.TempDir()
		writeE2EFile(t, filepath.Join(root, "project.godot"), `config_version=5

[input]

move_left={"deadzone": 0.5}
jump={"deadzone": 0.5}
`)
		text := requireToolText(t, session, "manage_input_map", map[string]any{
			"projectPath": root,
			"action":      "list",
		})
		var actions map[string]string
		if err := json.Unmarshal([]byte(text), &actions); err != nil {
			t.Fatalf("unmarshal %q: %v", text, err)
		}
		if got := actions["move_left"]; got != `{"deadzone": 0.5}` {
			t.Errorf("move_left = %q", got)
		}
		if got := actions["jump"]; got != `{"deadzone": 0.5}` {
			t.Errorf("jump = %q", got)
		}
	})

	t.Run("add with key", func(t *testing.T) {
		root := t.TempDir()
		seedProject(t, root)
		text := requireToolText(t, session, "manage_input_map", map[string]any{
			"projectPath": root,
			"action":      "add",
			"actionName":  "move_left",
			"key":         "W",
		})
		if want := `Input action "move_left" added.`; text != want {
			t.Errorf("text = %q, want %q", text, want)
		}
		content := readFile(t, filepath.Join(root, "project.godot"))
		requireContains(t, content, `move_left={"deadzone": 0.5, "events": [Object(InputEventKey,`, `"physical_keycode":87`)
	})

	t.Run("add without key", func(t *testing.T) {
		root := t.TempDir()
		seedProject(t, root)
		text := requireToolText(t, session, "manage_input_map", map[string]any{
			"projectPath": root,
			"action":      "add",
			"actionName":  "jump",
			"deadzone":    1,
		})
		if want := `Input action "jump" added.`; text != want {
			t.Errorf("text = %q, want %q", text, want)
		}
		content := readFile(t, filepath.Join(root, "project.godot"))
		requireContains(t, content, `jump={"deadzone": 1}`)
	})

	t.Run("merge existing action", func(t *testing.T) {
		root := t.TempDir()
		existingEvent := `Object(InputEventKey,"resource_local_to_scene":false,"resource_name":"","device":-1,"window_id":0,"alt_pressed":false,"shift_pressed":false,"ctrl_pressed":false,"meta_pressed":false,"pressed":false,"keycode":0,"physical_keycode":87,"key_label":0,"unicode":0,"location":0,"echo":false,"script":null)`
		writeE2EFile(t, filepath.Join(root, "project.godot"), "config_version=5\n\n[input]\n\nmove_left={\"deadzone\": 0.5, \"events\": ["+existingEvent+"]}\n")
		text := requireToolText(t, session, "manage_input_map", map[string]any{
			"projectPath": root,
			"action":      "add",
			"actionName":  "move_left",
			"key":         "D",
		})
		if want := `Input action "move_left" updated (event merged into existing action).`; text != want {
			t.Errorf("text = %q, want %q", text, want)
		}
		content := readFile(t, filepath.Join(root, "project.godot"))
		requireContains(t, content, `"physical_keycode":87`, `"physical_keycode":68`)
	})

	t.Run("remove", func(t *testing.T) {
		root := t.TempDir()
		writeE2EFile(t, filepath.Join(root, "project.godot"), `config_version=5

[input]

move_left={"deadzone": 0.5}
jump={"deadzone": 0.5}
`)
		text := requireToolText(t, session, "manage_input_map", map[string]any{
			"projectPath": root,
			"action":      "remove",
			"actionName":  "move_left",
		})
		if want := `Input action "move_left" removed.`; text != want {
			t.Errorf("text = %q, want %q", text, want)
		}
		content := readFile(t, filepath.Join(root, "project.godot"))
		requireNotContains(t, content, "move_left")
		requireContains(t, content, "jump=")
	})
}

// TestManageInputMapErrors covers the reference's validation and failure
// messages.
func TestManageInputMapErrors(t *testing.T) {
	session := connect(t)
	root := t.TempDir()
	seedProject(t, root)

	t.Run("missing params", func(t *testing.T) {
		requireToolError(t, session, "manage_input_map", map[string]any{},
			"projectPath and action are required.")
	})

	t.Run("invalid path", func(t *testing.T) {
		requireToolError(t, session, "manage_input_map", map[string]any{
			"projectPath": "../escape",
			"action":      "list",
		}, "Invalid path.")
	})

	t.Run("not a project", func(t *testing.T) {
		empty := t.TempDir()
		requireToolError(t, session, "manage_input_map", map[string]any{
			"projectPath": empty,
			"action":      "list",
		}, "Not a valid Godot project: "+empty)
	})

	t.Run("add missing actionName", func(t *testing.T) {
		requireToolError(t, session, "manage_input_map", map[string]any{
			"projectPath": root,
			"action":      "add",
		}, "actionName is required for add action.")
	})

	t.Run("remove missing actionName", func(t *testing.T) {
		requireToolError(t, session, "manage_input_map", map[string]any{
			"projectPath": root,
			"action":      "remove",
		}, "actionName is required for remove action.")
	})

	t.Run("invalid action", func(t *testing.T) {
		requireToolError(t, session, "manage_input_map", map[string]any{
			"projectPath": root,
			"action":      "bogus",
		}, `Invalid action. Use "list", "add", or "remove".`)
	})
}
