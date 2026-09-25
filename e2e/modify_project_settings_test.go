package e2e

import (
	"path/filepath"
	"strings"
	"testing"
)

// TestModifyProjectSettingsSchema ports the reference's tool-definition
// expectations for modify_project_settings over the real stdio surface.
func TestModifyProjectSettingsSchema(t *testing.T) {
	schema := requireSchema(t, "modify_project_settings", "Modify a project.godot setting")
	schema.requireRequired(t, "section", "key", "value")
	schema.requirePropCount(t, 3)
	schema.requireStringProps(t, "section", "key")
	schema.requireProps(t, "value")
}

// TestModifyProjectSettingsTool drives the real modify_project_settings tool
// against a seeded project.godot. After each write it reads the project back
// through Godot (via read_project_settings), proving the rewritten file is one
// the engine accepts. Each subtest binds a fresh session to its own project.
func TestModifyProjectSettingsTool(t *testing.T) {
	t.Run("update existing key", func(t *testing.T) {
		root := t.TempDir()
		writeE2EFile(t, filepath.Join(root, "project.godot"), `config_version=5

[application]

config/name="Demo"
run/main_scene="res://main.tscn"
`)
		session := connectProject(t, root)
		requireGodot(t, session)
		text := requireToolText(t, session, "modify_project_settings", map[string]any{
			"section": "application",
			"key":     "config/name",
			"value":   "New",
		})
		if want := `Setting updated: [application] config/name="New"`; text != want {
			t.Errorf("text = %q, want %q", text, want)
		}

		sections := requireProjectSettingsRoundTrip(t, session)
		if got := sections["application"]["config/name"]; got != "New" {
			t.Errorf("config/name = %v, want New", got)
		}
	})

	t.Run("add new key", func(t *testing.T) {
		root := t.TempDir()
		writeE2EFile(t, filepath.Join(root, "project.godot"), `config_version=5

[application]

config/name="Demo"
`)
		session := connectProject(t, root)
		requireGodot(t, session)
		requireToolText(t, session, "modify_project_settings", map[string]any{
			"section": "application",
			"key":     "config/description",
			"value":   "Hi",
		})

		sections := requireProjectSettingsRoundTrip(t, session)
		if got := sections["application"]["config/description"]; got != "Hi" {
			t.Errorf("config/description = %v, want Hi", got)
		}
	})
}

// TestModifyProjectSettingsEchoesContractValue proves the structured result
// reports the stored value in the tool value contract, so it can be handed
// straight back to modify_project_settings. The echoed value is written to a
// second key and the file is checked: a formatted string would land as a quoted
// string rather than the vector literal.
func TestModifyProjectSettingsEchoesContractValue(t *testing.T) {
	root := t.TempDir()
	writeE2EFile(t, filepath.Join(root, "project.godot"), "config_version=5\n")
	session := connectProject(t, root)

	res := callTool(t, session, "modify_project_settings", map[string]any{
		"section": "display",
		"key":     "window/size/test",
		"value":   map[string]any{"$type": "Vector2i", "x": 1, "y": 2},
	})
	if res.IsError {
		t.Fatalf("modify_project_settings error: %s", firstText(t, res))
	}
	structured, ok := res.StructuredContent.(map[string]any)
	if !ok {
		t.Fatalf("structuredContent type %T", res.StructuredContent)
	}
	echoed, ok := structured["value"]
	if !ok {
		t.Fatalf("structuredContent has no value: %#v", structured)
	}

	res = callTool(t, session, "modify_project_settings", map[string]any{
		"section": "display",
		"key":     "window/size/copy",
		"value":   echoed,
	})
	if res.IsError {
		t.Fatalf("round-trip modify_project_settings error: %s", firstText(t, res))
	}
	content := readFile(t, filepath.Join(root, "project.godot"))
	if got := strings.Count(content, "=Vector2i(1, 2)"); got != 2 {
		t.Errorf("found %d vector literals, want 2:\n%s", got, content)
	}
}

// TestModifyProjectSettingsErrors covers the reference's validation and failure
// messages.
func TestModifyProjectSettingsErrors(t *testing.T) {
	t.Run("missing args", func(t *testing.T) {
		session := connectProject(t, newProject(t))
		requireToolError(t, session, "modify_project_settings", map[string]any{},
			"section, key, and value are required.")
	})

	t.Run("not a project", func(t *testing.T) {
		root := t.TempDir()
		session := connectProject(t, root)
		requireToolError(t, session, "modify_project_settings", map[string]any{
			"section": "application",
			"key":     "config/name",
			"value":   "x",
		}, "Not a valid Godot project: "+root)
	})
}
