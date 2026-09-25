package e2e

import (
	"encoding/json"
	"path/filepath"
	"testing"
)

// TestManageExportPresetsSchema ports the reference's tool-definition
// expectations for manage_export_presets over the real stdio surface.
func TestManageExportPresetsSchema(t *testing.T) {
	schema := requireSchema(t, "manage_export_presets", "Create or modify export preset configuration")
	schema.requireRequired(t, "action")
	schema.requirePropCount(t, 4)
	schema.requireStringProps(t, "action", "name", "platform")
	schema.requireType(t, "runnable", "boolean")
}

// TestManageExportPresetsTool drives the real manage_export_presets tool for
// list, add, and remove.
func TestManageExportPresetsTool(t *testing.T) {
	t.Run("list empty", func(t *testing.T) {
		root := newProject(t)
		session := connectProject(t, root)

		text := requireToolText(t, session, "manage_export_presets", map[string]any{
			"action": "list",
		})
		if got, want := text, "{\n  \"presets\": []\n}"; got != want {
			t.Errorf("text = %q, want %q", got, want)
		}
	})

	t.Run("add then list", func(t *testing.T) {
		root := newProject(t)
		session := connectProject(t, root)

		text := requireToolText(t, session, "manage_export_presets", map[string]any{
			"action":   "add",
			"name":     "Linux",
			"platform": "Linux",
			"runnable": true,
		})
		if got, want := text, `Export preset "Linux" added for platform "Linux".`; got != want {
			t.Errorf("text = %q, want %q", got, want)
		}
		requireContains(t, readFile(t, filepath.Join(root, "export_presets.cfg")),
			"[preset.", `name="Linux"`, `platform="Linux"`, "runnable=true")

		list := requireToolText(t, session, "manage_export_presets", map[string]any{
			"action": "list",
		})
		var decoded struct {
			Presets []struct {
				Name     string `json:"name"`
				Platform string `json:"platform"`
			} `json:"presets"`
		}
		if err := json.Unmarshal([]byte(list), &decoded); err != nil {
			t.Fatalf("unmarshal %q: %v", list, err)
		}
		if len(decoded.Presets) != 1 || decoded.Presets[0].Name != "Linux" || decoded.Presets[0].Platform != "Linux" {
			t.Errorf("presets = %+v", decoded.Presets)
		}
	})

	t.Run("remove", func(t *testing.T) {
		root := newProject(t)
		session := connectProject(t, root)
		writeE2EFile(t, filepath.Join(root, "export_presets.cfg"),
			"[preset.0]\n\nname=\"Linux\"\nplatform=\"Linux\"\nrunnable=true\n")

		text := requireToolText(t, session, "manage_export_presets", map[string]any{
			"action": "remove",
			"name":   "Linux",
		})
		if got, want := text, `Export preset "Linux" removed.`; got != want {
			t.Errorf("text = %q, want %q", got, want)
		}
		requireNotContains(t, readFile(t, filepath.Join(root, "export_presets.cfg")), `name="Linux"`)
	})
}

// TestManageExportPresetsErrors covers the reference's validation messages.
func TestManageExportPresetsErrors(t *testing.T) {
	t.Run("missing params", func(t *testing.T) {
		session := connect(t)
		requireToolError(t, session, "manage_export_presets", map[string]any{},
			"action is required.")
	})

	t.Run("not a project", func(t *testing.T) {
		root := t.TempDir()
		session := connectProject(t, root)
		requireToolError(t, session, "manage_export_presets", map[string]any{
			"action": "list",
		}, "Not a valid Godot project: "+root)
	})

	t.Run("add requires name and platform", func(t *testing.T) {
		root := newProject(t)
		session := connectProject(t, root)
		requireToolError(t, session, "manage_export_presets", map[string]any{
			"action": "add",
			"name":   "Linux",
		}, "name and platform are required for add action.")
	})

	t.Run("remove requires name", func(t *testing.T) {
		root := newProject(t)
		session := connectProject(t, root)
		requireToolError(t, session, "manage_export_presets", map[string]any{
			"action": "remove",
		}, "name is required for remove action.")
	})

	t.Run("unknown action", func(t *testing.T) {
		root := newProject(t)
		session := connectProject(t, root)
		requireToolError(t, session, "manage_export_presets", map[string]any{
			"action": "bogus",
		}, `Invalid action. Use "list", "add", or "remove".`)
	})
}
