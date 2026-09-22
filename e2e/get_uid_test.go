package e2e

import (
	"path/filepath"
	"testing"
)

// TestGetUIDSchema ports the reference's tool-definition expectations for
// get_uid over the real stdio surface.
func TestGetUIDSchema(t *testing.T) {
	schema := requireSchema(t, "get_uid", "Get the UID for a specific file in a Godot project (for Godot 4.4+)")
	schema.requireRequired(t, "projectPath", "filePath")
	schema.requirePropCount(t, 2)
	schema.requireStringProps(t, "projectPath", "filePath")
}

// TestGetUIDTool drives the real get_uid tool against Godot for a file with a
// UID sidecar and one without.
func TestGetUIDTool(t *testing.T) {
	session := connect(t)
	requireGodot(t, session)

	root := t.TempDir()
	seedProject(t, root)

	t.Run("reads a UID sidecar", func(t *testing.T) {
		writeE2EFile(t, filepath.Join(root, "player.gd"), "extends Node\n")
		writeE2EFile(t, filepath.Join(root, "player.gd.uid"), "uid://e2etestuid")

		text := requireToolText(t, session, "get_uid", map[string]any{
			"projectPath": root,
			"filePath":    "player.gd",
		})
		requirePrefix(t, text, "UID for player.gd: ")
		requireContains(t, text, "uid://e2etestuid")
	})

	t.Run("reports a missing UID sidecar", func(t *testing.T) {
		writeE2EFile(t, filepath.Join(root, "orphan.gd"), "extends Node\n")

		text := requireToolText(t, session, "get_uid", map[string]any{
			"projectPath": root,
			"filePath":    "orphan.gd",
		})
		requireContains(t, text, `"exists":false`)
	})
}

// TestGetUIDVersionGate checks the Godot 4.4 requirement with a fake engine.
func TestGetUIDVersionGate(t *testing.T) {
	session := connectWithFakeGodot(t, "4.3.1.stable")
	root := t.TempDir()
	seedProject(t, root)
	writeE2EFile(t, filepath.Join(root, "player.gd"), "extends Node\n")

	requireToolError(t, session, "get_uid", map[string]any{
		"projectPath": root,
		"filePath":    "player.gd",
	}, "UIDs are only supported in Godot 4.4 or later. Current version: 4.3.1.stable")
}

// TestGetUIDErrors covers the reference's validation and failure messages.
func TestGetUIDErrors(t *testing.T) {
	session := connect(t)

	t.Run("missing params", func(t *testing.T) {
		requireToolError(t, session, "get_uid", map[string]any{}, "Missing required parameters")
	})

	t.Run("invalid path", func(t *testing.T) {
		requireToolError(t, session, "get_uid", map[string]any{
			"projectPath": "../escape",
			"filePath":    "player.gd",
		}, "Invalid path")
	})

	t.Run("not a project", func(t *testing.T) {
		root := t.TempDir()
		requireToolError(t, session, "get_uid", map[string]any{
			"projectPath": root,
			"filePath":    "player.gd",
		}, "Not a valid Godot project: "+root)
	})

	t.Run("missing file", func(t *testing.T) {
		root := t.TempDir()
		seedProject(t, root)
		requireToolError(t, session, "get_uid", map[string]any{
			"projectPath": root,
			"filePath":    "player.gd",
		}, "File does not exist: player.gd")
	})
}
