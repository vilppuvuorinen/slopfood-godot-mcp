package e2e

import (
	"os"
	"path/filepath"
	"testing"
)

// TestUpdateProjectUIDsSchema ports the reference's tool-definition
// expectations for update_project_uids over the real stdio surface.
func TestUpdateProjectUIDsSchema(t *testing.T) {
	schema := requireSchema(t, "update_project_uids", "Generate missing resource UIDs in a Godot project by scanning it in the editor (for Godot 4.4+)")
	schema.requireRequired(t)
	schema.requirePropCount(t, 0)
}

// TestUpdateProjectUIDsTool drives the real update_project_uids tool against
// Godot. It verifies the two properties the scan must have: it generates a
// missing .uid sidecar for an un-UID'd script, and it leaves existing UIDs
// (a scene's own header uid and an ext_resource uid) untouched.
func TestUpdateProjectUIDsTool(t *testing.T) {
	root, session := seedScene(t, "scenes/main.tscn")
	requireGodot(t, session)

	// An un-UID'd script that the scan must give a sidecar.
	scriptPath := filepath.Join(root, "player.gd")
	writeE2EFile(t, scriptPath, "extends Node\n")

	// A script that already has a .uid sidecar, referenced by a scene that
	// already carries uid= attributes. The scan must not rewrite them.
	usedPath := filepath.Join(root, "used.gd")
	writeE2EFile(t, usedPath, "extends Node\n")
	writeE2EFile(t, usedPath+".uid", "uid://e2euseduid\n")

	scenePath := filepath.Join(root, "scenes", "main.tscn")
	beforeScene := "[gd_scene load_steps=2 format=3 uid=\"uid://e2esceneuid\"]\n\n" +
		"[ext_resource type=\"Script\" uid=\"uid://e2euseduid\" path=\"res://used.gd\" id=\"1_used\"]\n\n" +
		"[node name=\"root\" type=\"Node2D\"]\nscript = ExtResource(\"1_used\")\n"
	writeE2EFile(t, scenePath, beforeScene)

	text := requireToolText(t, session, "update_project_uids", map[string]any{})
	requirePrefix(t, text, "Project UIDs updated successfully.")

	if _, err := os.Stat(scriptPath + ".uid"); err != nil {
		t.Errorf("expected %s.uid to be generated: %v", scriptPath, err)
	}

	afterScene := readFile(t, scenePath)
	if afterScene != beforeScene {
		t.Errorf("existing UIDs were changed by the scan:\nbefore:\n%s\nafter:\n%s", beforeScene, afterScene)
	}
	requireContains(t, afterScene, `uid="uid://e2euseduid"`)
}

// TestUpdateProjectUIDsVersionGate checks the Godot 4.4 requirement with a fake
// engine.
func TestUpdateProjectUIDsVersionGate(t *testing.T) {
	root := t.TempDir()
	seedProject(t, root)
	path := filepath.Join(t.TempDir(), "godot")
	writeFileMode(t, path, "#!/bin/sh\necho \"4.3.1.stable\"\nexit 0\n", 0o755)
	session := connectProjectWithEnv(t, root, "GODOT_PATH="+path)

	requireToolError(t, session, "update_project_uids", map[string]any{},
		"UIDs are only supported in Godot 4.4 or later. Current version: 4.3.1.stable")
}

// TestUpdateProjectUIDsErrors covers the reference's validation messages.
func TestUpdateProjectUIDsErrors(t *testing.T) {
	t.Run("not a project", func(t *testing.T) {
		root := t.TempDir()
		session := connectProject(t, root)
		requireToolError(t, session, "update_project_uids", map[string]any{},
			"Not a valid Godot project: "+root)
	})
}
