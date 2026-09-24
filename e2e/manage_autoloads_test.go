package e2e

import (
	"encoding/json"
	"path/filepath"
	"testing"
)

// TestManageAutoloadsSchema ports the reference's tool-definition
// expectations for manage_autoloads over the real stdio surface.
func TestManageAutoloadsSchema(t *testing.T) {
	schema := requireSchema(t, "manage_autoloads", "Add, remove, or list autoloads in a Godot project")
	schema.requireRequired(t, "projectPath", "action")
	schema.requirePropCount(t, 4)
	schema.requireStringProps(t, "projectPath", "action", "name", "path")
}

// TestManageAutoloadsTool drives the real manage_autoloads tool through a
// list/add/remove round-trip, including the snake_case alias. Writes are read
// back through Godot (via read_project_settings) to prove the engine accepts
// the rewritten project.godot.
func TestManageAutoloadsTool(t *testing.T) {
	session := connect(t)

	t.Run("lists autoloads", func(t *testing.T) {
		root := t.TempDir()
		writeE2EFile(t, filepath.Join(root, "project.godot"), `config_version=5

[autoload]

Game="*res://game.gd"
Music="*res://music.gd"
`)
		text := requireToolText(t, session, "manage_autoloads", map[string]any{
			"projectPath": root,
			"action":      "list",
		})

		var autoloads map[string]string
		if err := json.Unmarshal([]byte(text), &autoloads); err != nil {
			t.Fatalf("unmarshal %q: %v", text, err)
		}
		if got := autoloads["Game"]; got != `"*res://game.gd"` {
			t.Errorf("Game = %q", got)
		}
		if got := autoloads["Music"]; got != `"*res://music.gd"` {
			t.Errorf("Music = %q", got)
		}
	})

	t.Run("add and remove round trip", func(t *testing.T) {
		requireGodot(t, session)

		root := t.TempDir()
		seedProject(t, root)

		added := requireToolText(t, session, "manage_autoloads", map[string]any{
			"projectPath": root,
			"action":      "add",
			"name":        "Game",
			"path":        "res://game.gd",
		})
		if added != `Autoload "Game" added: res://game.gd` {
			t.Errorf("add text = %q", added)
		}
		// Seed the autoload script so the read-back probe has clean stderr.
		writeE2EFile(t, filepath.Join(root, "game.gd"), "extends Node\n")
		sections := requireProjectSettingsRoundTrip(t, session, root)
		if got := sections["autoload"]["Game"]; got != `"*res://game.gd"` {
			t.Errorf("autoload Game = %q, want %q", got, `"*res://game.gd"`)
		}

		removed := requireToolText(t, session, "manage_autoloads", map[string]any{
			"projectPath": root,
			"action":      "remove",
			"name":        "Game",
		})
		if removed != `Autoload "Game" removed.` {
			t.Errorf("remove text = %q", removed)
		}
		sections = requireProjectSettingsRoundTrip(t, session, root)
		if _, ok := sections["autoload"]["Game"]; ok {
			t.Errorf("autoload Game still present: %v", sections["autoload"])
		}
	})

	t.Run("add preserves existing section", func(t *testing.T) {
		requireGodot(t, session)

		root := t.TempDir()
		writeE2EFile(t, filepath.Join(root, "project.godot"), `config_version=5

[autoload]

Game="*res://game.gd"
`)
		requireToolText(t, session, "manage_autoloads", map[string]any{
			"project_path": root,
			"action":       "add",
			"name":         "Music",
			"path":         "res://music.gd",
		})
		// Seed the autoload scripts so the read-back probe has clean stderr.
		writeE2EFile(t, filepath.Join(root, "game.gd"), "extends Node\n")
		writeE2EFile(t, filepath.Join(root, "music.gd"), "extends Node\n")
		sections := requireProjectSettingsRoundTrip(t, session, root)
		if got := sections["autoload"]["Game"]; got != `"*res://game.gd"` {
			t.Errorf("autoload Game = %q, want it preserved", got)
		}
		if got := sections["autoload"]["Music"]; got != `"*res://music.gd"` {
			t.Errorf("autoload Music = %q", got)
		}
	})

	t.Run("snake_case alias", func(t *testing.T) {
		root := t.TempDir()
		seedProject(t, root)

		requireToolText(t, session, "manage_autoloads", map[string]any{
			"project_path": root,
			"action":       "list",
		})
	})
}

// TestManageAutoloadsErrors covers the reference's validation and failure
// messages.
func TestManageAutoloadsErrors(t *testing.T) {
	session := connect(t)

	t.Run("missing args", func(t *testing.T) {
		requireToolError(t, session, "manage_autoloads", map[string]any{},
			"projectPath and action are required.")
	})

	t.Run("missing action", func(t *testing.T) {
		root := t.TempDir()
		seedProject(t, root)
		requireToolError(t, session, "manage_autoloads", map[string]any{
			"projectPath": root,
		}, "projectPath and action are required.")
	})

	t.Run("invalid path", func(t *testing.T) {
		requireToolError(t, session, "manage_autoloads", map[string]any{
			"projectPath": "../escape",
			"action":      "list",
		}, "Invalid path.")
	})

	t.Run("not a project", func(t *testing.T) {
		root := t.TempDir()
		requireToolError(t, session, "manage_autoloads", map[string]any{
			"projectPath": root,
			"action":      "list",
		}, "Not a valid Godot project: "+root)
	})

	t.Run("invalid action", func(t *testing.T) {
		root := t.TempDir()
		seedProject(t, root)
		requireToolError(t, session, "manage_autoloads", map[string]any{
			"projectPath": root,
			"action":      "bogus",
		}, `Invalid action. Use "list", "add", or "remove".`)
	})

	t.Run("add missing name and path", func(t *testing.T) {
		root := t.TempDir()
		seedProject(t, root)
		requireToolError(t, session, "manage_autoloads", map[string]any{
			"projectPath": root,
			"action":      "add",
		}, "name and path are required for add action.")
	})

	t.Run("remove missing name", func(t *testing.T) {
		root := t.TempDir()
		seedProject(t, root)
		requireToolError(t, session, "manage_autoloads", map[string]any{
			"projectPath": root,
			"action":      "remove",
		}, "name is required for remove action.")
	})
}
