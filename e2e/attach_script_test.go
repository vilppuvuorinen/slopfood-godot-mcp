package e2e

import (
	"path/filepath"
	"testing"
)

// TestAttachScriptSchema ports the reference's tool-definition expectations for
// attach_script over the real stdio surface.
func TestAttachScriptSchema(t *testing.T) {
	schema := requireSchema(t, "attach_script", "Attach a GDScript to a scene node (headless)")
	schema.requireRequired(t, "projectPath", "scenePath", "nodePath", "scriptPath")
	schema.requireStringProps(t, "projectPath", "scenePath", "nodePath", "scriptPath")
	schema.requirePropCount(t, 4)
}

// TestAttachScriptTool drives the real attach_script tool against Godot and
// checks the saved scene references the attached script.
func TestAttachScriptTool(t *testing.T) {
	session := connect(t)
	requireGodot(t, session)

	root := seedScene(t, session, "scenes/main.tscn", "Node2D")
	writeE2EFile(t, filepath.Join(root, "scripts", "player.gd"), "extends Node2D\n")

	requireToolText(t, session, "attach_script", map[string]any{
		"projectPath": root,
		"scenePath":   "scenes/main.tscn",
		"nodePath":    "root",
		"scriptPath":  "scripts/player.gd",
	})

	scene := readFile(t, filepath.Join(root, "scenes", "main.tscn"))
	requireContains(t, scene, "script = ExtResource(", "res://scripts/player.gd")
}

// TestAttachScriptErrors covers the reference's validation and failure messages.
func TestAttachScriptErrors(t *testing.T) {
	session := connect(t)

	t.Run("missing paths", func(t *testing.T) {
		requireToolError(t, session, "attach_script", map[string]any{}, "projectPath, scenePath, nodePath, and scriptPath are required.")
	})

	t.Run("invalid path", func(t *testing.T) {
		requireToolError(t, session, "attach_script", map[string]any{
			"projectPath": "../escape",
			"scenePath":   "main.tscn",
			"nodePath":    "root",
			"scriptPath":  "scripts/player.gd",
		}, "Invalid path.")
	})

	t.Run("not a project", func(t *testing.T) {
		root := t.TempDir()
		requireToolError(t, session, "attach_script", map[string]any{
			"projectPath": root,
			"scenePath":   "main.tscn",
			"nodePath":    "root",
			"scriptPath":  "scripts/player.gd",
		}, "Not a valid Godot project: "+root)
	})

	t.Run("missing scene", func(t *testing.T) {
		root := t.TempDir()
		seedProject(t, root)
		requireToolError(t, session, "attach_script", map[string]any{
			"projectPath": root,
			"scenePath":   "missing.tscn",
			"nodePath":    "root",
			"scriptPath":  "scripts/player.gd",
		}, "Scene file does not exist: missing.tscn")
	})
}
