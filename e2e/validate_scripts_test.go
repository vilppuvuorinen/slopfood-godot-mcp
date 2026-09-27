package e2e

import (
	"encoding/json"
	"path/filepath"
	"testing"
)

// validateScriptsE2EDoc mirrors the JSON document validate_scripts returns.
type validateScriptsE2EDoc struct {
	Scope           string           `json:"scope"`
	FileCount       int              `json:"fileCount"`
	FilesWithErrors int              `json:"filesWithErrors"`
	AllValid        bool             `json:"allValid"`
	Results         []map[string]any `json:"results"`
}

// TestValidateScriptsSchema ports the reference's tool-definition expectations
// for validate_scripts over the real stdio surface.
func TestValidateScriptsSchema(t *testing.T) {
	t.Parallel()
	schema := requireSchema(t, "validate_scripts", "Batch-check GDScript files (git-changed by default, or all)")
	schema.requireRequired(t)
	schema.requireProps(t, "scope", "scriptPaths")
	schema.requireStringProps(t, "scope")
	schema.requireType(t, "scriptPaths", "array")
	schema.requirePropCount(t, 2)
}

// TestValidateScriptsTool drives the real validate_scripts tool against Godot.
// Each subtest binds a session to its own project.
func TestValidateScriptsTool(t *testing.T) {
	t.Parallel()
	t.Run("explicit scriptPaths", func(t *testing.T) {
		root := t.TempDir()
		seedProject(t, root)
		writeE2EFile(t, filepath.Join(root, "scripts", "valid.gd"), "extends Node\n")
		writeE2EFile(t, filepath.Join(root, "scripts", "invalid.gd"), "extends Node\n\nfunc broken(\n")
		session := connectProject(t, root)

		text := requireToolText(t, session, "validate_scripts", map[string]any{
			"scriptPaths": []string{"scripts/valid.gd", "scripts/invalid.gd"},
		})
		var doc validateScriptsE2EDoc
		if err := json.Unmarshal([]byte(text), &doc); err != nil {
			t.Fatalf("unmarshal %q: %v", text, err)
		}
		if doc.Scope != "explicit" {
			t.Errorf("scope = %q", doc.Scope)
		}
		if doc.FileCount != 2 || doc.FilesWithErrors != 1 || doc.AllValid {
			t.Errorf("fileCount=%d filesWithErrors=%d allValid=%v", doc.FileCount, doc.FilesWithErrors, doc.AllValid)
		}
		if len(doc.Results) != 2 {
			t.Fatalf("results = %+v", doc.Results)
		}
		if got := doc.Results[0]["valid"]; got != true {
			t.Errorf("valid.gd result = %+v", doc.Results[0])
		}
		if got := doc.Results[1]["valid"]; got != false {
			t.Errorf("invalid.gd result = %+v", doc.Results[1])
		}
		errors, ok := doc.Results[1]["errors"].([]any)
		if !ok || len(errors) == 0 {
			t.Fatalf("invalid.gd errors = %+v", doc.Results[1]["errors"])
		}
		if got := errors[0].(map[string]any)["file"]; got != "scripts/invalid.gd" {
			t.Errorf("diagnostic file = %v, want the project-relative scripts/invalid.gd", got)
		}
	})

	t.Run("scope changed by default", func(t *testing.T) {
		root := t.TempDir()
		seedProject(t, root)
		session := connectProject(t, root)

		// Outside a work tree the default scope is an explicit error, not a
		// silent fallback to validating every script.
		requireToolError(t, session, "validate_scripts", map[string]any{},
			`Not a git repository (or git is unavailable). Use scope: "all" or pass scriptPaths.`)

		writeE2EFile(t, filepath.Join(root, "committed.gd"), "extends Node\n")
		runGit(t, root, "init", "-q")
		runGit(t, root, "add", "committed.gd")
		runGit(t, root, "commit", "-q", "-m", "baseline")
		// Untracked, so it is one of the three change classes git reports.
		writeE2EFile(t, filepath.Join(root, "changed.gd"), "extends Node\n")

		text := requireToolText(t, session, "validate_scripts", map[string]any{})
		var doc validateScriptsE2EDoc
		if err := json.Unmarshal([]byte(text), &doc); err != nil {
			t.Fatalf("unmarshal %q: %v", text, err)
		}
		if doc.Scope != "changed" {
			t.Errorf("scope = %q, want changed", doc.Scope)
		}
		if doc.FileCount != 1 || !doc.AllValid {
			t.Errorf("fileCount=%d allValid=%v, want 1/true", doc.FileCount, doc.AllValid)
		}
		if len(doc.Results) != 1 || doc.Results[0]["scriptPath"] != "changed.gd" {
			t.Errorf("results = %+v, want only the untracked changed.gd", doc.Results)
		}
	})

	t.Run("resolves autoload globals", func(t *testing.T) {
		// The checker exists so a script referencing a project autoload validates
		// cleanly; Godot's --check-only mode reports the autoload as an unknown
		// identifier because it compiles before autoloads are registered.
		root := t.TempDir()
		writeE2EFile(t, filepath.Join(root, "project.godot"),
			"config_version=5\n\n[autoload]\n\nGameState=\"*res://autoload/game_state.gd\"\n")
		writeE2EFile(t, filepath.Join(root, "autoload", "game_state.gd"), "extends Node\n")
		writeE2EFile(t, filepath.Join(root, "scripts", "player.gd"),
			"extends Node\n\n@onready var state = GameState\n")
		// A reference to a global that does not exist must still fail, proving
		// the checker resolves identifiers rather than accepting anything.
		writeE2EFile(t, filepath.Join(root, "scripts", "broken.gd"),
			"extends Node\n\n@onready var state = NotAnAutoload\n")
		session := connectProject(t, root)

		text := requireToolText(t, session, "validate_scripts", map[string]any{
			"scriptPaths": []string{"scripts/player.gd", "scripts/broken.gd"},
		})
		var doc validateScriptsE2EDoc
		if err := json.Unmarshal([]byte(text), &doc); err != nil {
			t.Fatalf("unmarshal %q: %v", text, err)
		}
		if doc.FileCount != 2 || doc.FilesWithErrors != 1 || doc.AllValid {
			t.Errorf("fileCount=%d filesWithErrors=%d allValid=%v", doc.FileCount, doc.FilesWithErrors, doc.AllValid)
		}
		if got := doc.Results[0]["valid"]; got != true {
			t.Errorf("player.gd result = %+v, want valid", doc.Results[0])
		}
		if got := doc.Results[1]["valid"]; got != false {
			t.Errorf("broken.gd result = %+v, want invalid", doc.Results[1])
		}
	})

	t.Run("scope all", func(t *testing.T) {
		root := t.TempDir()
		seedProject(t, root)
		writeE2EFile(t, filepath.Join(root, "a.gd"), "extends Node\n")
		writeE2EFile(t, filepath.Join(root, "nested", "b.gd"), "extends Node\n")
		session := connectProject(t, root)

		text := requireToolText(t, session, "validate_scripts", map[string]any{
			"scope": "all",
		})
		var doc validateScriptsE2EDoc
		if err := json.Unmarshal([]byte(text), &doc); err != nil {
			t.Fatalf("unmarshal %q: %v", text, err)
		}
		if doc.Scope != "all" {
			t.Errorf("scope = %q", doc.Scope)
		}
		if doc.FileCount != 2 || doc.FilesWithErrors != 0 || !doc.AllValid {
			t.Errorf("fileCount=%d filesWithErrors=%d allValid=%v", doc.FileCount, doc.FilesWithErrors, doc.AllValid)
		}
	})
}

// TestValidateScriptsErrors covers the reference's validation messages.
func TestValidateScriptsErrors(t *testing.T) {
	t.Parallel()
	t.Run("not a project", func(t *testing.T) {
		root := t.TempDir()
		session := connectProject(t, root)
		requireToolError(t, session, "validate_scripts", map[string]any{},
			"Not a valid Godot project: "+root)
	})
}
