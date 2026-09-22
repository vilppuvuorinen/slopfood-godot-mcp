package e2e

import (
	"path/filepath"
	"strings"
	"testing"
)

// TestModifyProjectSettingsSchema ports the reference's tool-definition
// expectations for modify_project_settings over the real stdio surface.
func TestModifyProjectSettingsSchema(t *testing.T) {
	schema := requireSchema(t, "modify_project_settings", "Modify a project.godot setting")
	schema.requireRequired(t, "projectPath", "section", "key", "value")
	schema.requirePropCount(t, 4)
	schema.requireStringProps(t, "projectPath", "section", "key", "value")
}

// TestModifyProjectSettingsTool drives the real modify_project_settings tool
// against a seeded project.godot.
func TestModifyProjectSettingsTool(t *testing.T) {
	session := connect(t)

	t.Run("update existing key", func(t *testing.T) {
		root := t.TempDir()
		writeE2EFile(t, filepath.Join(root, "project.godot"), `config_version=5

[application]

config/name="Demo"
run/main_scene="res://main.tscn"
`)
		text := requireToolText(t, session, "modify_project_settings", map[string]any{
			"projectPath": root,
			"section":     "application",
			"key":         "config/name",
			"value":       `"New"`,
		})
		if want := `Setting updated: [application] config/name="New"`; text != want {
			t.Errorf("text = %q, want %q", text, want)
		}
		content := readFile(t, filepath.Join(root, "project.godot"))
		if !strings.Contains(content, `config/name="New"`) {
			t.Errorf("config/name not updated:\n%s", content)
		}
	})

	t.Run("add new key", func(t *testing.T) {
		root := t.TempDir()
		writeE2EFile(t, filepath.Join(root, "project.godot"), `config_version=5

[application]

config/name="Demo"
`)
		requireToolText(t, session, "modify_project_settings", map[string]any{
			"projectPath": root,
			"section":     "application",
			"key":         "config/description",
			"value":       `"Hi"`,
		})
		content := readFile(t, filepath.Join(root, "project.godot"))
		if !strings.Contains(content, "\nconfig/description=\"Hi\"\n") {
			t.Errorf("new key not inserted:\n%s", content)
		}
	})
}

// TestModifyProjectSettingsErrors covers the reference's validation and failure
// messages.
func TestModifyProjectSettingsErrors(t *testing.T) {
	session := connect(t)

	t.Run("missing args", func(t *testing.T) {
		requireToolError(t, session, "modify_project_settings", map[string]any{},
			"projectPath, section, key, and value are required.")
	})

	t.Run("invalid path", func(t *testing.T) {
		requireToolError(t, session, "modify_project_settings", map[string]any{
			"projectPath": "../escape",
			"section":     "application",
			"key":         "config/name",
			"value":       "x",
		}, "Invalid path.")
	})

	t.Run("not a project", func(t *testing.T) {
		root := t.TempDir()
		requireToolError(t, session, "modify_project_settings", map[string]any{
			"projectPath": root,
			"section":     "application",
			"key":         "config/name",
			"value":       "x",
		}, "Not a valid Godot project: "+root)
	})
}
