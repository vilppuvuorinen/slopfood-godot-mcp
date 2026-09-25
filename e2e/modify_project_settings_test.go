package e2e

import (
	"path/filepath"
	"testing"
)

// TestModifyProjectSettingsSchema ports the reference's tool-definition
// expectations for modify_project_settings over the real stdio surface.
func TestModifyProjectSettingsSchema(t *testing.T) {
	schema := requireSchema(t, "modify_project_settings", "Modify a project.godot setting")
	schema.requireRequired(t, "section", "key", "value")
	schema.requirePropCount(t, 3)
	schema.requireStringProps(t, "section", "key")
	schema.requireProps(t, "value")
}

// TestModifyProjectSettingsTool drives the real modify_project_settings tool
// against a seeded project.godot. After each write it reads the project back
// through Godot (via read_project_settings), proving the rewritten file is one
// the engine accepts. Each subtest binds a fresh session to its own project.
func TestModifyProjectSettingsTool(t *testing.T) {
	t.Run("update existing key", func(t *testing.T) {
		root := t.TempDir()
		writeE2EFile(t, filepath.Join(root, "project.godot"), `config_version=5

[application]

config/name="Demo"
run/main_scene="res://main.tscn"
`)
		session := connectProject(t, root)
		requireGodot(t, session)
		text := requireToolText(t, session, "modify_project_settings", map[string]any{
			"section": "application",
			"key":     "config/name",
			"value":   "New",
		})
		if want := `Setting updated: [application] config/name="New"`; text != want {
			t.Errorf("text = %q, want %q", text, want)
		}

		sections := requireProjectSettingsRoundTrip(t, session)
		if got := sections["application"]["config/name"]; got != "New" {
			t.Errorf("config/name = %v, want New", got)
		}
	})

	t.Run("add new key", func(t *testing.T) {
		root := t.TempDir()
		writeE2EFile(t, filepath.Join(root, "project.godot"), `config_version=5

[application]

config/name="Demo"
`)
		session := connectProject(t, root)
		requireGodot(t, session)
		requireToolText(t, session, "modify_project_settings", map[string]any{
			"section": "application",
			"key":     "config/description",
			"value":   "Hi",
		})

		sections := requireProjectSettingsRoundTrip(t, session)
		if got := sections["application"]["config/description"]; got != "Hi" {
			t.Errorf("config/description = %v, want Hi", got)
		}
	})
}

// TestModifyProjectSettingsErrors covers the reference's validation and failure
// messages.
func TestModifyProjectSettingsErrors(t *testing.T) {
	t.Run("missing args", func(t *testing.T) {
		session := connectProject(t, newProject(t))
		requireToolError(t, session, "modify_project_settings", map[string]any{},
			"section, key, and value are required.")
	})

	t.Run("not a project", func(t *testing.T) {
		root := t.TempDir()
		session := connectProject(t, root)
		requireToolError(t, session, "modify_project_settings", map[string]any{
			"section": "application",
			"key":     "config/name",
			"value":   "x",
		}, "Not a valid Godot project: "+root)
	})
}
