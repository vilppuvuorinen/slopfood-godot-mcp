package e2e

import (
	"encoding/json"
	"path/filepath"
	"testing"
)

type validateE2EDoc struct {
	Valid   bool `json:"valid"`
	Project struct {
		Valid  bool     `json:"valid"`
		Errors []string `json:"errors"`
	} `json:"project"`
	Scripts struct {
		FileCount       int              `json:"fileCount"`
		FilesWithErrors int              `json:"filesWithErrors"`
		AllValid        bool             `json:"allValid"`
		Error           string           `json:"error"`
		Results         []map[string]any `json:"results"`
	} `json:"scripts"`
}

func decodeValidateE2E(t *testing.T, text string) validateE2EDoc {
	t.Helper()
	var doc validateE2EDoc
	if err := json.Unmarshal([]byte(text), &doc); err != nil {
		t.Fatalf("unmarshal %q: %v", text, err)
	}
	return doc
}

func e2eResultByPath(t *testing.T, doc validateE2EDoc, path string) map[string]any {
	t.Helper()
	for _, result := range doc.Scripts.Results {
		if result["scriptPath"] == path {
			return result
		}
	}
	t.Fatalf("no result for %q in %+v", path, doc.Scripts.Results)
	return nil
}

func TestValidateTool(t *testing.T) {
	t.Parallel()

	t.Run("valid project", func(t *testing.T) {
		root := t.TempDir()
		writeE2EFile(t, filepath.Join(root, "project.godot"), "config_version=5\n\n[application]\n\nconfig/name=\"Demo\"\n")
		session := connectProject(t, root)

		doc := decodeValidateE2E(t, requireToolText(t, session, "validate", map[string]any{}))
		if !doc.Valid || !doc.Project.Valid {
			t.Errorf("valid = %v / project.valid = %v, want true", doc.Valid, doc.Project.Valid)
		}
		if len(doc.Project.Errors) != 0 {
			t.Errorf("project errors = %v, want none", doc.Project.Errors)
		}
		if !doc.Scripts.AllValid {
			t.Errorf("scripts = %+v, want all valid", doc.Scripts)
		}
	})

	t.Run("malformed project", func(t *testing.T) {
		root := t.TempDir()
		writeE2EFile(t, filepath.Join(root, "project.godot"), "config_version=5\n\n[application\n\nconfig/name=\"Demo\"\n")
		session := connectProject(t, root)

		res := callTool(t, session, "validate", map[string]any{})
		if res.IsError {
			t.Fatalf("expected a structured report, got an error: %s", firstText(t, res))
		}
		doc := decodeValidateE2E(t, firstText(t, res))
		if doc.Valid || doc.Project.Valid {
			t.Errorf("valid = %v / project.valid = %v, want false", doc.Valid, doc.Project.Valid)
		}
		if len(doc.Project.Errors) == 0 {
			t.Fatal("project errors is empty, want the engine's parse diagnostic")
		}
		requireContains(t, doc.Project.Errors[0], "Error parsing", "project.godot")
	})

	t.Run("reports valid and broken scripts", func(t *testing.T) {
		root := t.TempDir()
		seedProject(t, root)
		writeE2EFile(t, filepath.Join(root, "scripts", "valid.gd"), "extends Node\n")
		writeE2EFile(t, filepath.Join(root, "scripts", "invalid.gd"), "extends Node\n\nfunc broken(\n")
		session := connectProject(t, root)

		doc := decodeValidateE2E(t, requireToolText(t, session, "validate", map[string]any{}))
		if !doc.Project.Valid {
			t.Errorf("project.valid = false, errors = %v", doc.Project.Errors)
		}
		if doc.Valid || doc.Scripts.FileCount != 2 || doc.Scripts.FilesWithErrors != 1 || doc.Scripts.AllValid {
			t.Errorf("valid=%v fileCount=%d filesWithErrors=%d allValid=%v", doc.Valid, doc.Scripts.FileCount, doc.Scripts.FilesWithErrors, doc.Scripts.AllValid)
		}
		if doc.Scripts.Error != "" {
			t.Errorf("scripts error = %q, want none", doc.Scripts.Error)
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

		doc := decodeValidateE2E(t, requireToolText(t, session, "validate", map[string]any{}))
		if doc.Scripts.FileCount != 3 || doc.Scripts.FilesWithErrors != 1 || doc.Scripts.AllValid {
			t.Errorf("fileCount=%d filesWithErrors=%d allValid=%v", doc.Scripts.FileCount, doc.Scripts.FilesWithErrors, doc.Scripts.AllValid)
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

		doc := decodeValidateE2E(t, requireToolText(t, session, "validate", map[string]any{}))
		if doc.Scripts.FileCount != 2 || doc.Scripts.FilesWithErrors != 0 || !doc.Scripts.AllValid {
			t.Errorf("fileCount=%d filesWithErrors=%d allValid=%v", doc.Scripts.FileCount, doc.Scripts.FilesWithErrors, doc.Scripts.AllValid)
		}
		e2eResultByPath(t, doc, "a.gd")
		e2eResultByPath(t, doc, "nested/b.gd")
	})

	t.Run("engine failure", func(t *testing.T) {
		root := t.TempDir()
		seedProject(t, root)
		session := connectProjectWithEnv(t, root, "GODOT_PATH="+filepath.Join(t.TempDir(), "missing-godot"))

		res := callTool(t, session, "validate", map[string]any{})
		if !res.IsError {
			t.Fatalf("expected an error, got: %s", firstText(t, res))
		}
		requirePrefix(t, firstText(t, res), "Failed to validate:")
	})

	t.Run("not a project", func(t *testing.T) {
		root := t.TempDir()
		session := connectProject(t, root)
		requireToolError(t, session, "validate", map[string]any{},
			"Not a valid Godot project: "+root)
	})
}
