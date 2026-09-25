package e2e

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// TestReadProjectSettingsSchema ports the reference's tool-definition
// expectations for read_project_settings over the real stdio surface.
func TestReadProjectSettingsSchema(t *testing.T) {
	schema := requireSchema(t, "read_project_settings", "Read project.godot as structured JSON")
	schema.requireRequired(t)
	schema.requireStringProps(t, "section")
	schema.requireType(t, "keys", "array")
	schema.requirePropCount(t, 2)
}

// TestReadProjectSettingsTool drives the real read_project_settings tool and
// parses the resulting JSON document. read_project_settings loads the file with
// the real Godot engine, so these subtests are Godot-gated and also prove the
// engine accepts the seeded project.godot. Each subtest binds a session to its
// own project.
func TestReadProjectSettingsTool(t *testing.T) {
	t.Run("parses sections", func(t *testing.T) {
		root := t.TempDir()
		// Seed the autoload script too: a missing one makes the engine log an
		// error to stderr, which the strict stderr check treats as a failure.
		writeE2EFile(t, filepath.Join(root, "game.gd"), "extends Node\n")
		writeE2EFile(t, filepath.Join(root, "project.godot"), `config_version=5

[application]

config/name="Demo"
run/main_scene="res://main.tscn"

[autoload]

Game="*res://game.gd"
`)
		session := connectProject(t, root)
		requireGodot(t, session)
		sections := requireProjectSettingsRoundTrip(t, session)

		if got := sections["application"]["config/name"]; got != "Demo" {
			t.Errorf("config/name = %v", got)
		}
		if got := sections["autoload"]["Game"]; got != "*res://game.gd" {
			t.Errorf("autoload Game = %v", got)
		}
	})

	t.Run("includes settings equal to the engine default", func(t *testing.T) {
		root := t.TempDir()
		// config/name defaults to "" in the engine, so a file that stores it
		// explicitly must still be reported: reading the file's keys, not the
		// engine's non-default settings, is the contract the specialized
		// readers share.
		writeE2EFile(t, filepath.Join(root, "project.godot"), `config_version=5

[application]

config/name=""
`)
		session := connectProject(t, root)
		requireGodot(t, session)
		sections := requireProjectSettingsRoundTrip(t, session)

		got, ok := sections["application"]["config/name"]
		if !ok {
			t.Fatalf("config/name missing from read-back; sections = %#v", sections)
		}
		if got != "" {
			t.Errorf("config/name = %#v, want empty string", got)
		}
	})
}

// TestReadProjectSettingsFilters proves the optional section and keys filters
// bound the returned document over the real stdio surface.
func TestReadProjectSettingsFilters(t *testing.T) {
	root := t.TempDir()
	writeE2EFile(t, filepath.Join(root, "project.godot"), `config_version=5

[application]

config/name="Demo"

[display]

window/size/viewport_width=1920
window/size/viewport_height=1080
`)
	session := connectProject(t, root)
	requireGodot(t, session)

	text := requireToolText(t, session, "read_project_settings", map[string]any{"section": "display"})
	var payload struct {
		SettingCount int                       `json:"settingCount"`
		Sections     map[string]map[string]any `json:"sections"`
	}
	if err := json.Unmarshal([]byte(text), &payload); err != nil {
		t.Fatalf("unmarshal %q: %v", text, err)
	}
	if payload.SettingCount != 2 {
		t.Errorf("settingCount = %d, want 2", payload.SettingCount)
	}
	if _, ok := payload.Sections["application"]; ok {
		t.Errorf("sections = %#v, want application dropped", payload.Sections)
	}
	if got := payload.Sections["display"]["window/size/viewport_width"]; got != float64(1920) {
		t.Errorf("viewport_width = %v, want 1920", got)
	}
}

// TestReadProjectSettingsRejectsUnparseableProject proves the Godot load probe
// is load-bearing: a project.godot the engine rejects must fail rather than
// come back as a parsed document.
func TestReadProjectSettingsRejectsUnparseableProject(t *testing.T) {
	root := t.TempDir()
	writeE2EFile(t, filepath.Join(root, "project.godot"), "config_version=5\n\n[application\n\nconfig/name=\"Demo\"\n")
	session := connectProject(t, root)
	requireGodot(t, session)

	res := callTool(t, session, "read_project_settings", map[string]any{})
	if !res.IsError {
		t.Fatalf("expected an error for an unparseable project.godot, got: %s", firstText(t, res))
	}
	requirePrefix(t, firstText(t, res), "Godot could not parse project.godot:")
}

// TestReadProjectSettingsReportsEngineFailure points GODOT_PATH at a missing
// binary so the probe cannot start, and asserts the tool reports the failure
// rather than silently succeeding with the Go parser's view.
func TestReadProjectSettingsReportsEngineFailure(t *testing.T) {
	root := t.TempDir()
	seedProject(t, root)
	session := connectProjectWithEnv(t, root, "GODOT_PATH="+filepath.Join(t.TempDir(), "missing-godot"))

	res := callTool(t, session, "read_project_settings", map[string]any{})
	if !res.IsError {
		t.Fatalf("expected an error, got: %s", firstText(t, res))
	}
	requirePrefix(t, firstText(t, res), "Failed to read project settings:")
	if _, err := os.Stat(filepath.Join(root, "project.godot")); err != nil {
		t.Errorf("project.godot should be untouched: %v", err)
	}
}

// TestReadProjectSettingsErrors covers the reference's validation and failure
// messages that come from the bound project root.
func TestReadProjectSettingsErrors(t *testing.T) {
	t.Run("not a project", func(t *testing.T) {
		root := t.TempDir()
		session := connectProject(t, root)
		requireToolError(t, session, "read_project_settings", map[string]any{},
			"Not a valid Godot project: "+root)
	})
}
