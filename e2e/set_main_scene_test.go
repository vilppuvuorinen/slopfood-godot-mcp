package e2e

import (
	"path/filepath"
	"testing"
)

// TestSetMainSceneSchema ports the reference's tool-definition expectations for
// set_main_scene over the real stdio surface.
func TestSetMainSceneSchema(t *testing.T) {
	schema := requireSchema(t, "set_main_scene", "Set the main scene in project.godot")
	schema.requireRequired(t, "projectPath", "scenePath")
	schema.requirePropCount(t, 2)
	schema.requireStringProps(t, "projectPath", "scenePath")
}

// TestSetMainSceneTool drives the real set_main_scene tool and reads the
// rewritten project.godot back from disk.
func TestSetMainSceneTool(t *testing.T) {
	session := connect(t)

	t.Run("replaces existing main scene", func(t *testing.T) {
		root := t.TempDir()
		writeE2EFile(t, filepath.Join(root, "project.godot"), `config_version=5

[application]

config/name="Demo"
run/main_scene="res://old.tscn"
`)
		text := requireToolText(t, session, "set_main_scene", map[string]any{
			"projectPath": root,
			"scenePath":   "scenes/main.tscn",
		})
		requirePrefix(t, text, "Main scene set to res://scenes/main.tscn")

		content := readFile(t, filepath.Join(root, "project.godot"))
		requireContains(t, content, `run/main_scene="res://scenes/main.tscn"`, `config/name="Demo"`)
		requireNotContains(t, content, `run/main_scene="res://old.tscn"`)
	})

	t.Run("appends application section", func(t *testing.T) {
		root := t.TempDir()
		writeE2EFile(t, filepath.Join(root, "project.godot"), "config_version=5\n")
		text := requireToolText(t, session, "set_main_scene", map[string]any{
			"projectPath": root,
			"scenePath":   "res://scenes/main.tscn",
		})
		if text != "Main scene set to res://scenes/main.tscn" {
			t.Errorf("text = %q", text)
		}

		content := readFile(t, filepath.Join(root, "project.godot"))
		requireContains(t, content, "[application]", `run/main_scene="res://scenes/main.tscn"`)
	})
}

// TestSetMainSceneErrors covers the reference's validation and failure messages.
func TestSetMainSceneErrors(t *testing.T) {
	session := connect(t)

	t.Run("missing arguments", func(t *testing.T) {
		requireToolError(t, session, "set_main_scene", map[string]any{},
			"projectPath and scenePath are required.")
	})

	t.Run("invalid path", func(t *testing.T) {
		requireToolError(t, session, "set_main_scene", map[string]any{
			"projectPath": "../escape",
			"scenePath":   "main.tscn",
		}, "Invalid path.")
	})

	t.Run("not a project", func(t *testing.T) {
		root := t.TempDir()
		requireToolError(t, session, "set_main_scene", map[string]any{
			"projectPath": root,
			"scenePath":   "main.tscn",
		}, "Not a valid Godot project: "+root)
	})
}
