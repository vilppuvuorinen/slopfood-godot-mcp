package e2e

import (
	"path/filepath"
	"testing"
)

// TestManageThemeResourceSchema ports the reference's tool-definition
// expectations for manage_theme_resource over the real stdio surface.
func TestManageThemeResourceSchema(t *testing.T) {
	schema := requireSchema(t, "manage_theme_resource", "Create/read/modify Theme .tres resources")
	schema.requireRequired(t, "projectPath", "resourcePath", "action")
	schema.requireStringProps(t, "projectPath", "resourcePath", "action")
	schema.requireType(t, "properties", "object")
	schema.requirePropCount(t, 4)
}

// TestManageThemeResourceTool drives the real manage_theme_resource tool
// against Godot and inspects the saved Theme resource.
func TestManageThemeResourceTool(t *testing.T) {
	session := connect(t)
	requireGodot(t, session)

	root := t.TempDir()
	seedProject(t, root)

	createText := requireToolText(t, session, "manage_theme_resource", map[string]any{
		"projectPath":  root,
		"resourcePath": "themes/main.tres",
		"action":       "create",
		"properties":   map[string]any{"default_font_size": 16},
	})
	requirePrefix(t, createText, "manage_theme_resource succeeded.")

	themeFile := filepath.Join(root, "themes", "main.tres")
	requireContains(t, readFile(t, themeFile), `[gd_resource type="Theme"`)

	readText := requireToolText(t, session, "manage_theme_resource", map[string]any{
		"projectPath":  root,
		"resourcePath": "themes/main.tres",
		"action":       "read",
	})
	requireContains(t, readText, `"type": "Theme"`)
	requireContains(t, readText, `"properties"`)
	requireContains(t, readText, `"default_font_size": "16"`)
	requireNotContains(t, readText, "THEME_JSON_START")

	modifyText := requireToolText(t, session, "manage_theme_resource", map[string]any{
		"projectPath":  root,
		"resourcePath": "themes/main.tres",
		"action":       "modify",
		"properties":   map[string]any{"default_font_size": 20},
	})
	requirePrefix(t, modifyText, "manage_theme_resource succeeded.")

	reReadText := requireToolText(t, session, "manage_theme_resource", map[string]any{
		"projectPath":  root,
		"resourcePath": "themes/main.tres",
		"action":       "read",
	})
	requireContains(t, reReadText, `"type": "Theme"`)
	requireContains(t, reReadText, `"default_font_size": "20"`)
	requireNotContains(t, reReadText, "THEME_JSON_START")
}

// TestManageThemeResourceErrors covers the reference's validation and failure
// messages.
func TestManageThemeResourceErrors(t *testing.T) {
	session := connect(t)

	t.Run("missing params", func(t *testing.T) {
		requireToolError(t, session, "manage_theme_resource", map[string]any{}, "projectPath, resourcePath, and action are required.")
	})

	t.Run("unknown action", func(t *testing.T) {
		requireGodot(t, session)
		root := t.TempDir()
		seedProject(t, root)

		res := callTool(t, session, "manage_theme_resource", map[string]any{
			"projectPath":  root,
			"resourcePath": "themes/main.tres",
			"action":       "bogus",
		})
		if !res.IsError {
			t.Fatal("expected an error result")
		}
		requireContains(t, firstText(t, res), "Unknown manage_theme_resource action: bogus")
	})

	t.Run("not a project", func(t *testing.T) {
		root := t.TempDir()
		requireToolError(t, session, "manage_theme_resource", map[string]any{
			"projectPath":  root,
			"resourcePath": "themes/main.tres",
			"action":       "read",
		}, "Not a valid Godot project: "+root)
	})

	t.Run("invalid path", func(t *testing.T) {
		requireToolError(t, session, "manage_theme_resource", map[string]any{
			"projectPath":  "../escape",
			"resourcePath": "themes/main.tres",
			"action":       "read",
		}, "Invalid path.")
	})
}
