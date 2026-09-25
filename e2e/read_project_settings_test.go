package e2e

import (
	"os"
	"path/filepath"
	"testing"
)

// TestReadProjectSettingsSchema ports the reference's tool-definition
// expectations for read_project_settings over the real stdio surface.
func TestReadProjectSettingsSchema(t *testing.T) {
	schema := requireSchema(t, "read_project_settings", "Read project.godot as structured JSON")
	schema.requireRequired(t)
	schema.requirePropCount(t, 0)
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
