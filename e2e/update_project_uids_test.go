package e2e

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestUpdateProjectUIDsSchema ports the reference's tool-definition
// expectations for update_project_uids over the real stdio surface.
func TestUpdateProjectUIDsSchema(t *testing.T) {
	schema := requireSchema(t, "update_project_uids", "Update UID references in a Godot project by resaving resources (for Godot 4.4+)")
	schema.requireRequired(t, "projectPath")
	schema.requirePropCount(t, 1)
	schema.requireStringProps(t, "projectPath")
}

// TestUpdateProjectUIDsTool drives the real update_project_uids tool against
// Godot after creating a scene with create_scene.
func TestUpdateProjectUIDsTool(t *testing.T) {
	session := connect(t)
	requireGodot(t, session)
	root := seedScene(t, session, "scenes/main.tscn")

	// Overwrite the scene with a non-canonical load_steps so that a real
	// resave rewrites it. The old absolute-path bug never found the file, so
	// the bogus value would survive a vacuous success.
	scenePath := filepath.Join(root, "scenes", "main.tscn")
	writeE2EFile(t, scenePath, "[gd_scene load_steps=99 format=3]\n\n[node name=\"root\" type=\"Node2D\"]\n")

	text := requireToolText(t, session, "update_project_uids", map[string]any{"projectPath": root})
	requirePrefix(t, text, "Project UIDs updated successfully.")
	requireContains(t, text, "Resave operation complete")

	after, err := os.ReadFile(scenePath)
	if err != nil {
		t.Fatalf("read scene after resave: %v", err)
	}
	if strings.Contains(string(after), "load_steps=99") {
		t.Errorf("scene was not resaved, bogus load_steps survived: %q", after)
	}
	if !strings.Contains(string(after), `[node name="root" type="Node2D"]`) {
		t.Errorf("scene root node missing after resave: %q", after)
	}
}

// TestUpdateProjectUIDsVersionGate checks the Godot 4.4 requirement with a fake
// engine.
func TestUpdateProjectUIDsVersionGate(t *testing.T) {
	session := connectWithFakeGodot(t, "4.3.1.stable")
	root := t.TempDir()
	seedProject(t, root)

	requireToolError(t, session, "update_project_uids", map[string]any{"projectPath": root},
		"UIDs are only supported in Godot 4.4 or later. Current version: 4.3.1.stable")
}

// TestUpdateProjectUIDsErrors covers the reference's validation messages.
func TestUpdateProjectUIDsErrors(t *testing.T) {
	session := connect(t)

	t.Run("missing params", func(t *testing.T) {
		requireToolError(t, session, "update_project_uids", map[string]any{}, "Project path is required")
	})

	t.Run("invalid path", func(t *testing.T) {
		requireToolError(t, session, "update_project_uids", map[string]any{
			"projectPath": "../escape",
		}, "Invalid project path")
	})

	t.Run("not a project", func(t *testing.T) {
		root := t.TempDir()
		requireToolError(t, session, "update_project_uids", map[string]any{
			"projectPath": root,
		}, "Not a valid Godot project: "+root)
	})
}
