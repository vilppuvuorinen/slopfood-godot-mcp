package e2e

import (
	"encoding/json"
	"path/filepath"
	"testing"
)

type validateScriptsE2EDoc struct {
	FileCount       int              `json:"fileCount"`
	FilesWithErrors int              `json:"filesWithErrors"`
	AllValid        bool             `json:"allValid"`
	Results         []map[string]any `json:"results"`
}

func e2eResultByPath(t *testing.T, doc validateScriptsE2EDoc, path string) map[string]any {
	t.Helper()
	for _, result := range doc.Results {
		if result["scriptPath"] == path {
			return result
		}
	}
	t.Fatalf("no result for %q in %+v", path, doc.Results)
	return nil
}

func TestValidateScriptsTool(t *testing.T) {
	t.Parallel()
	t.Run("reports valid and broken scripts", func(t *testing.T) {
		root := t.TempDir()
		seedProject(t, root)
		writeE2EFile(t, filepath.Join(root, "scripts", "valid.gd"), "extends Node\n")
		writeE2EFile(t, filepath.Join(root, "scripts", "invalid.gd"), "extends Node\n\nfunc broken(\n")
		session := connectProject(t, root)

		text := requireToolText(t, session, "validate_scripts", map[string]any{})
		var doc validateScriptsE2EDoc
		if err := json.Unmarshal([]byte(text), &doc); err != nil {
			t.Fatalf("unmarshal %q: %v", text, err)
		}
		if doc.FileCount != 2 || doc.FilesWithErrors != 1 || doc.AllValid {
			t.Errorf("fileCount=%d filesWithErrors=%d allValid=%v", doc.FileCount, doc.FilesWithErrors, doc.AllValid)
		}
		if len(doc.Results) != 2 {
			t.Fatalf("results = %+v", doc.Results)
		}
		if got := e2eResultByPath(t, doc, "scripts/valid.gd")["valid"]; got != true {
			t.Errorf("valid.gd valid = %v, want true", got)
		}
		invalid := e2eResultByPath(t, doc, "scripts/invalid.gd")
		if invalid["valid"] != false {
			t.Errorf("invalid.gd result = %+v", invalid)
		}
		errors, ok := invalid["errors"].([]any)
		if !ok || len(errors) == 0 {
			t.Fatalf("invalid.gd errors = %+v", invalid["errors"])
		}
		if got := errors[0].(map[string]any)["file"]; got != "scripts/invalid.gd" {
			t.Errorf("diagnostic file = %v, want the project-relative scripts/invalid.gd", got)
		}
	})

	t.Run("resolves autoload globals", func(t *testing.T) {
		// The checker resolves autoloads; Godot's --check-only reports them as unknown.
		root := t.TempDir()
		writeE2EFile(t, filepath.Join(root, "project.godot"),
			"config_version=5\n\n[autoload]\n\nGameState=\"*res://autoload/game_state.gd\"\n")
		writeE2EFile(t, filepath.Join(root, "autoload", "game_state.gd"), "extends Node\n")
		writeE2EFile(t, filepath.Join(root, "scripts", "player.gd"),
			"extends Node\n\n@onready var state = GameState\n")
		// A non-existent global must still fail, proving identifiers are resolved.
		writeE2EFile(t, filepath.Join(root, "scripts", "broken.gd"),
			"extends Node\n\n@onready var state = NotAnAutoload\n")
		session := connectProject(t, root)

		text := requireToolText(t, session, "validate_scripts", map[string]any{})
		var doc validateScriptsE2EDoc
		if err := json.Unmarshal([]byte(text), &doc); err != nil {
			t.Fatalf("unmarshal %q: %v", text, err)
		}
		if doc.FileCount != 3 || doc.FilesWithErrors != 1 || doc.AllValid {
			t.Errorf("fileCount=%d filesWithErrors=%d allValid=%v", doc.FileCount, doc.FilesWithErrors, doc.AllValid)
		}
		if got := e2eResultByPath(t, doc, "autoload/game_state.gd")["valid"]; got != true {
			t.Errorf("game_state.gd valid = %v, want true", got)
		}
		if got := e2eResultByPath(t, doc, "scripts/player.gd")["valid"]; got != true {
			t.Errorf("player.gd valid = %v, want true", got)
		}
		if got := e2eResultByPath(t, doc, "scripts/broken.gd")["valid"]; got != false {
			t.Errorf("broken.gd valid = %v, want false", got)
		}
	})

	t.Run("skips engine and dependency directories", func(t *testing.T) {
		root := t.TempDir()
		seedProject(t, root)
		writeE2EFile(t, filepath.Join(root, "a.gd"), "extends Node\n")
		writeE2EFile(t, filepath.Join(root, "nested", "b.gd"), "extends Node\n")
		writeE2EFile(t, filepath.Join(root, "node_modules", "dep.gd"), "extends Node\n\nfunc broken(\n")
		session := connectProject(t, root)

		text := requireToolText(t, session, "validate_scripts", map[string]any{})
		var doc validateScriptsE2EDoc
		if err := json.Unmarshal([]byte(text), &doc); err != nil {
			t.Fatalf("unmarshal %q: %v", text, err)
		}
		if doc.FileCount != 2 || doc.FilesWithErrors != 0 || !doc.AllValid {
			t.Errorf("fileCount=%d filesWithErrors=%d allValid=%v", doc.FileCount, doc.FilesWithErrors, doc.AllValid)
		}
		e2eResultByPath(t, doc, "a.gd")
		e2eResultByPath(t, doc, "nested/b.gd")
	})
}

func TestValidateScriptsErrors(t *testing.T) {
	t.Parallel()
	t.Run("not a project", func(t *testing.T) {
		root := t.TempDir()
		session := connectProject(t, root)
		requireToolError(t, session, "validate_scripts", map[string]any{},
			"Not a valid Godot project: "+root)
	})
}
