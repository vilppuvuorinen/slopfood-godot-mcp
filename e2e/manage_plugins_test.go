package e2e

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// TestManagePluginsSchema ports the reference's tool-definition expectations
// for manage_plugins over the real stdio surface.
func TestManagePluginsSchema(t *testing.T) {
	schema := requireSchema(t, "manage_plugins", "List/enable/disable editor plugins")
	schema.requireRequired(t, "projectPath", "action")
	schema.requirePropCount(t, 3)
	schema.requireStringProps(t, "projectPath", "action", "pluginName")
}

// TestManagePluginsTool drives the real manage_plugins tool for list, enable,
// and disable.
func TestManagePluginsTool(t *testing.T) {
	session := connect(t)

	t.Run("list", func(t *testing.T) {
		root := t.TempDir()
		writeE2EFile(t, filepath.Join(root, "project.godot"), `config_version=5

[editor_plugins]

MyPlugin/enabled=true
`)
		if err := os.MkdirAll(filepath.Join(root, "addons", "Demo"), 0o755); err != nil {
			t.Fatalf("mkdir addons: %v", err)
		}

		text := requireToolText(t, session, "manage_plugins", map[string]any{
			"projectPath": root,
			"action":      "list",
		})
		var list struct {
			Enabled   []string `json:"enabled"`
			Available []string `json:"available"`
		}
		if err := json.Unmarshal([]byte(text), &list); err != nil {
			t.Fatalf("unmarshal %q: %v", text, err)
		}
		requireContains(t, text, "MyPlugin", "Demo")
		if len(list.Enabled) != 1 || list.Enabled[0] != "MyPlugin" {
			t.Errorf("enabled = %v", list.Enabled)
		}
		if len(list.Available) != 1 || list.Available[0] != "Demo" {
			t.Errorf("available = %v", list.Available)
		}
	})

	t.Run("enable", func(t *testing.T) {
		requireGodot(t, session)
		root := t.TempDir()
		seedProject(t, root)

		text := requireToolText(t, session, "manage_plugins", map[string]any{
			"projectPath": root,
			"action":      "enable",
			"pluginName":  "Foo",
		})
		if text != "Plugin Foo enabled." {
			t.Errorf("text = %q", text)
		}
		sections := requireProjectSettingsRoundTrip(t, session, root)
		if got := sections["editor_plugins"]["Foo/enabled"]; got != "true" {
			t.Errorf("Foo/enabled = %q, want true", got)
		}
	})

	t.Run("disable", func(t *testing.T) {
		requireGodot(t, session)
		root := t.TempDir()
		writeE2EFile(t, filepath.Join(root, "project.godot"), `config_version=5

[editor_plugins]

Foo/enabled=true
`)

		text := requireToolText(t, session, "manage_plugins", map[string]any{
			"projectPath": root,
			"action":      "disable",
			"pluginName":  "Foo",
		})
		if text != "Plugin Foo disabled." {
			t.Errorf("text = %q", text)
		}
		sections := requireProjectSettingsRoundTrip(t, session, root)
		if got := sections["editor_plugins"]["Foo/enabled"]; got != "false" {
			t.Errorf("Foo/enabled = %q, want false", got)
		}
	})
}

// TestManagePluginsErrors covers the reference's validation and failure
// messages.
func TestManagePluginsErrors(t *testing.T) {
	session := connect(t)

	t.Run("missing params", func(t *testing.T) {
		requireToolError(t, session, "manage_plugins", map[string]any{},
			"projectPath and action are required.")
	})

	t.Run("invalid path", func(t *testing.T) {
		requireToolError(t, session, "manage_plugins", map[string]any{
			"projectPath": "../escape",
			"action":      "list",
		}, "Invalid path.")
	})

	t.Run("not a project", func(t *testing.T) {
		root := t.TempDir()
		requireToolError(t, session, "manage_plugins", map[string]any{
			"projectPath": root,
			"action":      "list",
		}, "Not a valid Godot project: "+root)
	})

	t.Run("pluginName required", func(t *testing.T) {
		root := t.TempDir()
		seedProject(t, root)
		requireToolError(t, session, "manage_plugins", map[string]any{
			"projectPath": root,
			"action":      "enable",
		}, "pluginName is required.")
	})

	t.Run("unknown action", func(t *testing.T) {
		root := t.TempDir()
		seedProject(t, root)
		requireToolError(t, session, "manage_plugins", map[string]any{
			"projectPath": root,
			"action":      "bogus",
		}, "Unknown action: bogus")
	})
}
