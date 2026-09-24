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
	schema.requireRequired(t, "projectPath")
	schema.requirePropCount(t, 1)
	schema.requireStringProps(t, "projectPath")
}

// TestReadProjectSettingsTool drives the real read_project_settings tool and
// parses the resulting JSON document. read_project_settings loads the file with
// the real Godot engine, so these subtests are Godot-gated and also prove the
// engine accepts the seeded project.godot.
func TestReadProjectSettingsTool(t *testing.T) {
	session := connect(t)
	requireGodot(t, session)

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
		sections := requireProjectSettingsRoundTrip(t, session, root)

		if got := sections["application"]["config/name"]; got != "Demo" {
			t.Errorf("config/name = %v", got)
		}
		if got := sections["autoload"]["Game"]; got != "*res://game.gd" {
			t.Errorf("autoload Game = %v", got)
		}
	})

	t.Run("snake_case aliases", func(t *testing.T) {
		root := t.TempDir()
		seedProject(t, root)

		requireToolText(t, session, "read_project_settings", map[string]any{
			"project_path": root,
		})
	})
}

// TestReadProjectSettingsRejectsUnparseableProject proves the Godot load probe
// is load-bearing: a project.godot the engine rejects must fail rather than
// come back as a parsed document.
func TestReadProjectSettingsRejectsUnparseableProject(t *testing.T) {
	session := connect(t)
	requireGodot(t, session)

	root := t.TempDir()
	writeE2EFile(t, filepath.Join(root, "project.godot"), "config_version=5\n\n[application\n\nconfig/name=\"Demo\"\n")

	res := callTool(t, session, "read_project_settings", map[string]any{
		"projectPath": root,
	})
	if !res.IsError {
		t.Fatalf("expected an error for an unparseable project.godot, got: %s", firstText(t, res))
	}
	requirePrefix(t, firstText(t, res), "Godot could not parse project.godot:")
}

// TestReadProjectSettingsReportsEngineFailure points GODOT_PATH at a missing
// binary so the probe cannot start, and asserts the tool reports the failure
// rather than silently succeeding with the Go parser's view.
func TestReadProjectSettingsReportsEngineFailure(t *testing.T) {
	session := connectWithEnv(t, "GODOT_PATH="+filepath.Join(t.TempDir(), "missing-godot"))
	root := t.TempDir()
	seedProject(t, root)

	res := callTool(t, session, "read_project_settings", map[string]any{
		"projectPath": root,
	})
	if !res.IsError {
		t.Fatalf("expected an error, got: %s", firstText(t, res))
	}
	requirePrefix(t, firstText(t, res), "Failed to read project settings:")
	if _, err := os.Stat(filepath.Join(root, "project.godot")); err != nil {
		t.Errorf("project.godot should be untouched: %v", err)
	}
}

// TestReadProjectSettingsErrors covers the reference's validation and failure
// messages.
func TestReadProjectSettingsErrors(t *testing.T) {
	session := connect(t)

	t.Run("missing projectPath", func(t *testing.T) {
		requireToolError(t, session, "read_project_settings", map[string]any{},
			"projectPath is required.")
	})

	t.Run("invalid path", func(t *testing.T) {
		requireToolError(t, session, "read_project_settings", map[string]any{
			"projectPath": "../escape",
		}, "Invalid path.")
	})

	t.Run("not a project", func(t *testing.T) {
		root := t.TempDir()
		requireToolError(t, session, "read_project_settings", map[string]any{
			"projectPath": root,
		}, "Not a valid Godot project: "+root)
	})
}
