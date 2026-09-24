package e2e

import (
	"encoding/json"
	"path/filepath"
	"testing"
)

// resourceTres is a minimal StandardMaterial3D resource so the manage_resource
// e2e tests do not depend on create_resource.
const resourceTres = `[gd_resource type="StandardMaterial3D" format=3]

[resource]
metallic = 0.5
`

// TestManageResourceSchema ports the reference's tool-definition expectations
// for manage_resource over the real stdio surface.
func TestManageResourceSchema(t *testing.T) {
	schema := requireSchema(t, "manage_resource", "Read or modify .tres/.res resource files")
	schema.requireRequired(t, "projectPath", "resourcePath", "action")
	schema.requireStringProps(t, "projectPath", "resourcePath", "action")
	schema.requireType(t, "properties", "object")
	schema.requirePropCount(t, 4)
}

// TestManageResourceTool drives the real manage_resource tool against Godot.
func TestManageResourceTool(t *testing.T) {
	session := connect(t)
	requireGodot(t, session)

	t.Run("reads a resource", func(t *testing.T) {
		root := t.TempDir()
		seedProject(t, root)
		writeE2EFile(t, filepath.Join(root, "resources", "mat.tres"), resourceTres)

		text := requireToolText(t, session, "manage_resource", map[string]any{
			"projectPath":  root,
			"resourcePath": "resources/mat.tres",
			"action":       "read",
		})
		var payload map[string]any
		if err := json.Unmarshal([]byte(text), &payload); err != nil {
			t.Fatalf("unmarshal resource payload: %v\n%s", err, text)
		}
		if payload["type"] != "StandardMaterial3D" {
			t.Errorf("type = %v, want StandardMaterial3D", payload["type"])
		}
		requireContains(t, text, `"type": "StandardMaterial3D"`)
	})

	t.Run("modifies then re-reads a resource", func(t *testing.T) {
		root := t.TempDir()
		seedProject(t, root)
		writeE2EFile(t, filepath.Join(root, "resources", "mat.tres"), resourceTres)

		modifyText := requireToolText(t, session, "manage_resource", map[string]any{
			"projectPath":  root,
			"resourcePath": "resources/mat.tres",
			"action":       "modify",
			"properties":   map[string]any{"metallic": 0.75},
		})
		requirePrefix(t, modifyText, "manage_resource succeeded.")

		readText := requireToolText(t, session, "manage_resource", map[string]any{
			"projectPath":  root,
			"resourcePath": "resources/mat.tres",
			"action":       "read",
		})
		requireContains(t, readText, `"metallic": 0.75`)
	})
}

// TestManageResourceErrors covers the reference's validation and failure
// messages.
func TestManageResourceErrors(t *testing.T) {
	session := connect(t)

	t.Run("missing params", func(t *testing.T) {
		requireToolError(t, session, "manage_resource", map[string]any{}, "projectPath, resourcePath, and action are required.")
	})

	t.Run("invalid path", func(t *testing.T) {
		requireToolError(t, session, "manage_resource", map[string]any{
			"projectPath":  "../escape",
			"resourcePath": "mat.tres",
			"action":       "read",
		}, "Invalid path.")
	})

	t.Run("not a project", func(t *testing.T) {
		root := t.TempDir()
		requireToolError(t, session, "manage_resource", map[string]any{
			"projectPath":  root,
			"resourcePath": "mat.tres",
			"action":       "read",
		}, "Not a valid Godot project: "+root)
	})

	t.Run("unknown action", func(t *testing.T) {
		requireGodot(t, session)
		root := t.TempDir()
		seedProject(t, root)

		res := callTool(t, session, "manage_resource", map[string]any{
			"projectPath":  root,
			"resourcePath": "resources/mat.tres",
			"action":       "bogus",
		})
		if !res.IsError {
			t.Fatal("expected an error result")
		}
		requireContains(t, firstText(t, res), "Unknown manage_resource action: bogus")
	})
}
