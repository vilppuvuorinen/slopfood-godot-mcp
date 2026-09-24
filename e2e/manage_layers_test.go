package e2e

import (
	"path/filepath"
	"testing"
)

// TestManageLayersSchema ports the reference's tool-definition expectations for
// manage_layers over the real stdio surface.
func TestManageLayersSchema(t *testing.T) {
	schema := requireSchema(t, "manage_layers", "List/set named layer definitions in project")
	schema.requireRequired(t, "projectPath", "action")
	schema.requirePropCount(t, 5)
	schema.requireStringProps(t, "projectPath", "action", "layerType", "name")
	schema.requireType(t, "layer", "number")
}

// TestManageLayersTool drives the real manage_layers list and set actions.
func TestManageLayersTool(t *testing.T) {
	session := connect(t)

	t.Run("lists layers", func(t *testing.T) {
		root := t.TempDir()
		writeE2EFile(t, filepath.Join(root, "project.godot"), "config_version=5\n\n[layer_names]\n\n"+`layer_names/2d_render/layer_1="Ground"`+"\n")

		text := requireToolText(t, session, "manage_layers", map[string]any{
			"projectPath": root,
			"action":      "list",
		})
		requireContains(t, text, `"layers"`, `"type": "2d_render"`, `"layer": 1`, `"name": "Ground"`)
	})

	t.Run("replaces an existing layer", func(t *testing.T) {
		requireGodot(t, session)
		root := t.TempDir()
		writeE2EFile(t, filepath.Join(root, "project.godot"), "config_version=5\n\n[layer_names]\n\n"+`layer_names/2d_render/layer_1="Old"`+"\n")

		text := requireToolText(t, session, "manage_layers", map[string]any{
			"projectPath": root,
			"action":      "set",
			"layerType":   "2d_render",
			"layer":       1,
			"name":        "Ground",
		})
		if want := `Layer set: layer_names/2d_render/layer_1="Ground"`; text != want {
			t.Errorf("message = %q, want %q", text, want)
		}
		sections := requireProjectSettingsRoundTrip(t, session, root)
		if got := sections["layer_names"]["layer_names/2d_render/layer_1"]; got != `"Ground"` {
			t.Errorf("layer_1 = %q, want %q", got, `"Ground"`)
		}
	})

	t.Run("appends a missing section", func(t *testing.T) {
		requireGodot(t, session)
		root := t.TempDir()
		writeE2EFile(t, filepath.Join(root, "project.godot"), "config_version=5\n")

		requireToolText(t, session, "manage_layers", map[string]any{
			"projectPath": root,
			"action":      "set",
			"layerType":   "3d_physics",
			"layer":       2,
			"name":        "Water",
		})
		sections := requireProjectSettingsRoundTrip(t, session, root)
		if got := sections["layer_names"]["layer_names/3d_physics/layer_2"]; got != `"Water"` {
			t.Errorf("layer_2 = %q", got)
		}
	})
}

// TestManageLayersErrors covers the reference's validation and failure messages.
func TestManageLayersErrors(t *testing.T) {
	session := connect(t)
	root := t.TempDir()
	seedProject(t, root)

	t.Run("missing params", func(t *testing.T) {
		requireToolError(t, session, "manage_layers", map[string]any{},
			"projectPath and action are required.")
	})

	t.Run("invalid path", func(t *testing.T) {
		requireToolError(t, session, "manage_layers", map[string]any{
			"projectPath": "../escape",
			"action":      "list",
		}, "Invalid path.")
	})

	t.Run("not a project", func(t *testing.T) {
		missing := t.TempDir()
		requireToolError(t, session, "manage_layers", map[string]any{
			"projectPath": missing,
			"action":      "list",
		}, "Not a valid Godot project: "+missing)
	})

	t.Run("set missing fields", func(t *testing.T) {
		requireToolError(t, session, "manage_layers", map[string]any{
			"projectPath": root,
			"action":      "set",
		}, "layerType, layer, and name are required for set.")
	})

	t.Run("unknown action", func(t *testing.T) {
		requireToolError(t, session, "manage_layers", map[string]any{
			"projectPath": root,
			"action":      "bogus",
		}, "Unknown action: bogus")
	})
}
