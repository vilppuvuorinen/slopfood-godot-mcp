package e2e

import (
	"encoding/json"
	"path/filepath"
	"testing"
)

// TestReadProjectSettingsSchema ports the reference's tool-definition
// expectations for read_project_settings over the real stdio surface.
func TestReadProjectSettingsSchema(t *testing.T) {
	schema := requireSchema(t, "read_project_settings", "Read project.godot as structured JSON")
	schema.requireRequired(t, "projectPath")
	schema.requirePropCount(t, 1)
	schema.requireStringProps(t, "projectPath")
}

// TestReadProjectSettingsTool drives the real read_project_settings tool and
// parses the resulting JSON document.
func TestReadProjectSettingsTool(t *testing.T) {
	session := connect(t)

	t.Run("parses sections", func(t *testing.T) {
		root := t.TempDir()
		writeE2EFile(t, filepath.Join(root, "project.godot"), `config_version=5

[application]

config/name="Demo"
run/main_scene="res://main.tscn"

[autoload]

Game="*res://game.gd"
`)
		text := requireToolText(t, session, "read_project_settings", map[string]any{
			"projectPath": root,
		})

		var sections map[string]map[string]string
		if err := json.Unmarshal([]byte(text), &sections); err != nil {
			t.Fatalf("unmarshal %q: %v", text, err)
		}
		if got := sections["application"]["config/name"]; got != `"Demo"` {
			t.Errorf("config/name = %q", got)
		}
		if got := sections["autoload"]["Game"]; got != `"*res://game.gd"` {
			t.Errorf("autoload Game = %q", got)
		}
	})

	t.Run("snake_case aliases", func(t *testing.T) {
		root := t.TempDir()
		seedProject(t, root)

		requireToolText(t, session, "read_project_settings", map[string]any{
			"project_path": root,
		})
	})
}

// TestReadProjectSettingsErrors covers the reference's validation and failure
// messages.
func TestReadProjectSettingsErrors(t *testing.T) {
	session := connect(t)

	t.Run("missing projectPath", func(t *testing.T) {
		requireToolError(t, session, "read_project_settings", map[string]any{},
			"projectPath is required.")
	})

	t.Run("invalid path", func(t *testing.T) {
		requireToolError(t, session, "read_project_settings", map[string]any{
			"projectPath": "../escape",
		}, "Invalid path.")
	})

	t.Run("not a project", func(t *testing.T) {
		root := t.TempDir()
		requireToolError(t, session, "read_project_settings", map[string]any{
			"projectPath": root,
		}, "Not a valid Godot project: "+root)
	})
}
