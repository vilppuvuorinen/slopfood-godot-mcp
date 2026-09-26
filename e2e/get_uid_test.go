package e2e

import (
	"path/filepath"
	"testing"
)

// TestGetUIDSchema ports the reference's tool-definition expectations for
// get_uid over the real stdio surface.
func TestGetUIDSchema(t *testing.T) {
	schema := requireSchema(t, "get_uid", "Get the UID for a specific file in a Godot project (for Godot 4.4+)")
	schema.requireRequired(t, "filePath")
	schema.requirePropCount(t, 1)
	schema.requireStringProps(t, "filePath")
}

// TestGetUIDTool drives the real get_uid tool against Godot for a file with a
// UID sidecar and one without.
func TestGetUIDTool(t *testing.T) {
	root := newProject(t)
	session := connectProject(t, root)
	requireGodot(t, session)

	t.Run("reads a UID sidecar", func(t *testing.T) {
		writeE2EFile(t, filepath.Join(root, "player.gd"), "extends Node\n")
		writeE2EFile(t, filepath.Join(root, "player.gd.uid"), "uid://e2etestuid")

		text := requireToolText(t, session, "get_uid", map[string]any{
			"filePath": "player.gd",
		})
		requireContains(t, text, `"uid": "uid://e2etestuid"`, `"file": "player.gd"`, `"absolutePath":`)
	})

	t.Run("missing UID sidecar is an error", func(t *testing.T) {
		writeE2EFile(t, filepath.Join(root, "orphan.gd"), "extends Node\n")

		res := callTool(t, session, "get_uid", map[string]any{
			"filePath": "orphan.gd",
		})
		if !res.IsError {
			t.Fatal("expected an error for a file without a UID sidecar")
		}
		requirePrefix(t, firstText(t, res), "Failed to get UID:")
		requireContains(t, firstText(t, res), "update_project_uids")
	})
}

// TestGetUIDVersionGate checks the Godot 4.4 requirement with a fake engine.
func TestGetUIDVersionGate(t *testing.T) {
	root := newProject(t)
	writeE2EFile(t, filepath.Join(root, "player.gd"), "extends Node\n")

	path := filepath.Join(t.TempDir(), "godot")
	writeFileMode(t, path, "#!/bin/sh\necho \"4.3.1.stable\"\nexit 0\n", 0o755)
	session := connectProjectWithEnv(t, root, "GODOT_PATH="+path)

	requireToolError(t, session, "get_uid", map[string]any{
		"filePath": "player.gd",
	}, "UIDs are only supported in Godot 4.4 or later. Current version: 4.3.1.stable")
}

// TestGetUIDErrors covers the reference's validation and failure messages.
func TestGetUIDErrors(t *testing.T) {
	t.Run("missing params", func(t *testing.T) {
		session := connect(t)
		requireToolError(t, session, "get_uid", map[string]any{}, "filePath is required.")
	})

	t.Run("not a project", func(t *testing.T) {
		root := t.TempDir()
		session := connectProject(t, root)
		requireToolError(t, session, "get_uid", map[string]any{
			"filePath": "player.gd",
		}, "Not a valid Godot project: "+root)
	})

	t.Run("missing file", func(t *testing.T) {
		root := newProject(t)
		session := connectProject(t, root)
		requireToolError(t, session, "get_uid", map[string]any{
			"filePath": "player.gd",
		}, "File does not exist: player.gd")
	})
}
