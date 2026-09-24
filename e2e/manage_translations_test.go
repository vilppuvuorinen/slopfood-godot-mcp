package e2e

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

// TestManageTranslationsSchema ports the reference's tool-definition
// expectations for manage_translations over the real stdio surface.
func TestManageTranslationsSchema(t *testing.T) {
	schema := requireSchema(t, "manage_translations", "List/add/remove translation files in project")
	schema.requireRequired(t, "projectPath", "action")
	schema.requirePropCount(t, 3)
	schema.requireStringProps(t, "projectPath", "action", "translationPath")
}

// TestManageTranslationsTool drives the real manage_translations tool against
// seeded project.godot files.
func TestManageTranslationsTool(t *testing.T) {
	session := connect(t)

	t.Run("list", func(t *testing.T) {
		root := t.TempDir()
		writeE2EFile(t, filepath.Join(root, "project.godot"), `config_version=5

[internationalization]

translations=PackedStringArray("res://en.translation", "res://fi.translation")
`)
		text := requireToolText(t, session, "manage_translations", map[string]any{
			"projectPath": root,
			"action":      "list",
		})
		var payload struct {
			Translations []string `json:"translations"`
		}
		if err := json.Unmarshal([]byte(text), &payload); err != nil {
			t.Fatalf("unmarshal %q: %v", text, err)
		}
		if got, want := strings.Join(payload.Translations, ","), "res://en.translation,res://fi.translation"; got != want {
			t.Errorf("translations = %q, want %q", got, want)
		}
	})

	t.Run("add to existing list", func(t *testing.T) {
		requireGodot(t, session)
		root := t.TempDir()
		writeE2EFile(t, filepath.Join(root, "project.godot"), `config_version=5

[internationalization]

translations=PackedStringArray("res://en.translation")
`)
		text := requireToolText(t, session, "manage_translations", map[string]any{
			"projectPath":     root,
			"action":          "add",
			"translationPath": "fi.translation",
		})
		if want := "Translation added: res://fi.translation"; text != want {
			t.Errorf("text = %q, want %q", text, want)
		}
		sections := requireProjectSettingsRoundTrip(t, session, root)
		requireContains(t, sections["internationalization"]["translations"], `"res://en.translation"`, `"res://fi.translation"`)
	})

	t.Run("add creates section", func(t *testing.T) {
		requireGodot(t, session)
		root := t.TempDir()
		seedProject(t, root)
		text := requireToolText(t, session, "manage_translations", map[string]any{
			"projectPath":     root,
			"action":          "add",
			"translationPath": "res://en.translation",
		})
		if want := "Translation added: res://en.translation"; text != want {
			t.Errorf("text = %q, want %q", text, want)
		}
		sections := requireProjectSettingsRoundTrip(t, session, root)
		requireContains(t, sections["internationalization"]["translations"], `"res://en.translation"`)
	})

	t.Run("remove", func(t *testing.T) {
		requireGodot(t, session)
		root := t.TempDir()
		writeE2EFile(t, filepath.Join(root, "project.godot"), `config_version=5

[internationalization]

translations=PackedStringArray("res://en.translation", "res://fi.translation")
`)
		text := requireToolText(t, session, "manage_translations", map[string]any{
			"projectPath":     root,
			"action":          "remove",
			"translationPath": "en.translation",
		})
		if want := "Translation removed: res://en.translation"; text != want {
			t.Errorf("text = %q, want %q", text, want)
		}
		sections := requireProjectSettingsRoundTrip(t, session, root)
		requireNotContains(t, sections["internationalization"]["translations"], "res://en.translation")
		requireContains(t, sections["internationalization"]["translations"], `"res://fi.translation"`)
	})
}

// TestManageTranslationsErrors covers the reference's validation and failure
// messages.
func TestManageTranslationsErrors(t *testing.T) {
	session := connect(t)

	t.Run("missing args", func(t *testing.T) {
		requireToolError(t, session, "manage_translations", map[string]any{},
			"projectPath and action are required.")
	})

	t.Run("missing translationPath", func(t *testing.T) {
		root := t.TempDir()
		seedProject(t, root)
		requireToolError(t, session, "manage_translations", map[string]any{
			"projectPath": root,
			"action":      "add",
		}, "translationPath is required.")
	})

	t.Run("unknown action", func(t *testing.T) {
		root := t.TempDir()
		seedProject(t, root)
		requireToolError(t, session, "manage_translations", map[string]any{
			"projectPath": root,
			"action":      "bogus",
		}, "Unknown action: bogus")
	})

	t.Run("invalid path", func(t *testing.T) {
		requireToolError(t, session, "manage_translations", map[string]any{
			"projectPath": "../escape",
			"action":      "list",
		}, "Invalid path.")
	})

	t.Run("not a project", func(t *testing.T) {
		root := t.TempDir()
		requireToolError(t, session, "manage_translations", map[string]any{
			"projectPath": root,
			"action":      "list",
		}, "Not a valid Godot project: "+root)
	})
}
