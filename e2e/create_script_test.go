package e2e

import (
	"path/filepath"
	"testing"
)

// TestCreateScriptSchema ports the reference's tool-definition expectations for
// create_script over the real stdio surface.
func TestCreateScriptSchema(t *testing.T) {
	schema := requireSchema(t, "create_script", "Create a GDScript file from a template")
	schema.requireRequired(t, "projectPath", "scriptPath")
	schema.requirePropCount(t, 6)
	schema.requireStringProps(t, "projectPath", "scriptPath", "extends", "className", "source")
	schema.requireType(t, "methods", "array")
}

// TestCreateScriptTool drives the real create_script tool without Godot.
func TestCreateScriptTool(t *testing.T) {
	session := connect(t)

	t.Run("creates with defaults", func(t *testing.T) {
		root := t.TempDir()
		seedProject(t, root)

		text := requireToolText(t, session, "create_script", map[string]any{
			"projectPath": root,
			"scriptPath":  "player.gd",
		})
		if want := "Script created at player.gd"; text != want {
			t.Errorf("message = %q, want %q", text, want)
		}
		content := readFile(t, filepath.Join(root, "player.gd"))
		if want := "extends Node\n\n"; content != want {
			t.Errorf("content = %q, want %q", content, want)
		}
	})

	t.Run("creates with class name and methods", func(t *testing.T) {
		root := t.TempDir()
		seedProject(t, root)

		requireToolText(t, session, "create_script", map[string]any{
			"projectPath": root,
			"scriptPath":  "scripts/player.gd",
			"extends":     "Node2D",
			"className":   "Player",
			"methods":     []string{"_ready", "_process"},
		})
		content := readFile(t, filepath.Join(root, "scripts", "player.gd"))
		want := "extends Node2D\nclass_name Player\n\n\nfunc _ready():\n\tpass\n\nfunc _process():\n\tpass\n"
		if content != want {
			t.Errorf("content = %q, want %q", content, want)
		}
	})

	t.Run("writes a custom source verbatim", func(t *testing.T) {
		root := t.TempDir()
		seedProject(t, root)

		source := "extends Area2D\n\nfunc _ready() -> void:\n\tprint(\"hi\")\n"
		requireToolText(t, session, "create_script", map[string]any{
			"projectPath": root,
			"scriptPath":  "custom.gd",
			"source":      source,
		})
		content := readFile(t, filepath.Join(root, "custom.gd"))
		if content != source {
			t.Errorf("content = %q, want %q", content, source)
		}
	})
}

// TestCreateScriptErrors covers the reference's validation messages.
func TestCreateScriptErrors(t *testing.T) {
	session := connect(t)
	root := t.TempDir()
	seedProject(t, root)

	t.Run("missing params", func(t *testing.T) {
		requireToolError(t, session, "create_script", map[string]any{},
			"projectPath and scriptPath are required.")
	})

	t.Run("invalid path", func(t *testing.T) {
		requireToolError(t, session, "create_script", map[string]any{
			"projectPath": root,
			"scriptPath":  "../escape.gd",
		}, "Invalid path.")
	})

	t.Run("not a project", func(t *testing.T) {
		missing := t.TempDir()
		requireToolError(t, session, "create_script", map[string]any{
			"projectPath": missing,
			"scriptPath":  "player.gd",
		}, "Not a valid Godot project: "+missing)
	})
}
