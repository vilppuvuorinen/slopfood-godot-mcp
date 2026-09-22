package e2e

import (
	"os"
	"path/filepath"
	"testing"
)

// TestSaveSceneSchema ports the reference's tool-definition expectations for
// save_scene over the real stdio surface.
func TestSaveSceneSchema(t *testing.T) {
	schema := requireSchema(t, "save_scene", "Save changes to a scene file")
	schema.requireRequired(t, "projectPath", "scenePath")
	schema.requirePropCount(t, 3)
	schema.requireStringProps(t, "projectPath", "scenePath", "newPath")
}

// TestSaveSceneTool drives the real save_scene tool against Godot, including
// the optional variant path and the snake_case argument aliases.
func TestSaveSceneTool(t *testing.T) {
	session := connect(t)
	requireGodot(t, session)
	root := seedScene(t, session, "scenes/main.tscn")

	t.Run("saves in place", func(t *testing.T) {
		text := requireToolText(t, session, "save_scene", map[string]any{
			"projectPath": root,
			"scenePath":   "scenes/main.tscn",
		})
		requirePrefix(t, text, "Scene saved successfully to: scenes/main.tscn")
	})

	t.Run("saves a variant", func(t *testing.T) {
		text := requireToolText(t, session, "save_scene", map[string]any{
			"projectPath": root,
			"scenePath":   "scenes/main.tscn",
			"newPath":     "variants/main_variant.tscn",
		})
		requirePrefix(t, text, "Scene saved successfully to: variants/main_variant.tscn")
		if _, err := os.Stat(filepath.Join(root, "variants", "main_variant.tscn")); err != nil {
			t.Errorf("stat saved variant: %v", err)
		}
	})

	t.Run("snake_case aliases", func(t *testing.T) {
		text := requireToolText(t, session, "save_scene", map[string]any{
			"project_path": root,
			"scene_path":   "scenes/main.tscn",
			"new_path":     "variants/aliased_variant.tscn",
		})
		requirePrefix(t, text, "Scene saved successfully to: variants/aliased_variant.tscn")
	})
}

// TestSaveSceneErrors covers the reference's validation messages.
func TestSaveSceneErrors(t *testing.T) {
	session := connect(t)

	t.Run("missing params", func(t *testing.T) {
		requireToolError(t, session, "save_scene", map[string]any{}, "Missing required parameters")
	})

	t.Run("invalid path", func(t *testing.T) {
		requireToolError(t, session, "save_scene", map[string]any{
			"projectPath": "../escape",
			"scenePath":   "scenes/main.tscn",
		}, "Invalid path")
	})

	t.Run("invalid new path", func(t *testing.T) {
		root := t.TempDir()
		seedProject(t, root)
		requireToolError(t, session, "save_scene", map[string]any{
			"projectPath": root,
			"scenePath":   "scenes/main.tscn",
			"newPath":     "../escape",
		}, "Invalid new path")
	})

	t.Run("not a project", func(t *testing.T) {
		root := t.TempDir()
		requireToolError(t, session, "save_scene", map[string]any{
			"projectPath": root,
			"scenePath":   "scenes/main.tscn",
		}, "Not a valid Godot project: "+root)
	})

	t.Run("missing scene", func(t *testing.T) {
		root := t.TempDir()
		seedProject(t, root)
		requireToolError(t, session, "save_scene", map[string]any{
			"projectPath": root,
			"scenePath":   "missing.tscn",
		}, "Scene file does not exist: missing.tscn")
	})
}
