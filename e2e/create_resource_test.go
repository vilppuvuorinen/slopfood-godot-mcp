package e2e

import (
	"path/filepath"
	"testing"
)

// TestCreateResourceSchema ports the reference's tool-definition expectations
// for create_resource over the real stdio surface.
func TestCreateResourceSchema(t *testing.T) {
	schema := requireSchema(t, "create_resource", "Create a .tres resource file (headless)")
	schema.requireRequired(t, "projectPath", "resourceType", "resourcePath")
	schema.requireStringProps(t, "projectPath", "resourceType", "resourcePath")
	schema.requireType(t, "properties", "object")
	schema.requirePropCount(t, 4)
}

// TestCreateResourceTool drives the real create_resource tool against Godot and
// reads the saved resource back from disk.
func TestCreateResourceTool(t *testing.T) {
	session := connect(t)
	requireGodot(t, session)

	root := t.TempDir()
	seedProject(t, root)

	requireToolText(t, session, "create_resource", map[string]any{
		"projectPath":  root,
		"resourceType": "StandardMaterial3D",
		"resourcePath": "materials/red.tres",
		"properties": map[string]any{
			"albedo_color": map[string]any{"r": 1, "g": 0, "b": 0, "a": 1},
		},
	})

	content := readFile(t, filepath.Join(root, "materials", "red.tres"))
	requireContains(t, content, `[gd_resource type="StandardMaterial3D"`)
}

// TestCreateResourceErrors covers the reference's validation and failure
// messages.
func TestCreateResourceErrors(t *testing.T) {
	session := connect(t)

	t.Run("not a project", func(t *testing.T) {
		root := t.TempDir()
		requireToolError(t, session, "create_resource", map[string]any{
			"projectPath":  root,
			"resourceType": "StandardMaterial3D",
			"resourcePath": "materials/red.tres",
		}, "Not a valid Godot project: "+root)
	})

	t.Run("invalid path", func(t *testing.T) {
		root := t.TempDir()
		seedProject(t, root)
		requireToolError(t, session, "create_resource", map[string]any{
			"projectPath":  root,
			"resourceType": "StandardMaterial3D",
			"resourcePath": "../escape.tres",
		}, "Invalid path.")
	})
}
