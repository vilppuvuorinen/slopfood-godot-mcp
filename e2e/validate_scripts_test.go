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
	schema := requireSchema(t, "validate_scripts", "Batch-check GDScript files (git-changed by default, or all)")
	schema.requireRequired(t, "projectPath")
	schema.requireProps(t, "projectPath", "scope", "scriptPaths")
	schema.requireStringProps(t, "projectPath", "scope")
	schema.requireType(t, "scriptPaths", "array")
	schema.requirePropCount(t, 3)
}

// TestValidateScriptsTool drives the real validate_scripts tool against Godot.
func TestValidateScriptsTool(t *testing.T) {
	session := connect(t)
	requireGodot(t, session)

	t.Run("explicit scriptPaths", func(t *testing.T) {
		root := t.TempDir()
		seedProject(t, root)
		writeE2EFile(t, filepath.Join(root, "scripts", "valid.gd"), "extends Node\n")
		writeE2EFile(t, filepath.Join(root, "scripts", "invalid.gd"), "extends Node\n\nfunc broken(\n")

		text := requireToolText(t, session, "validate_scripts", map[string]any{
			"projectPath": root,
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
	})

	t.Run("scope all", func(t *testing.T) {
		root := t.TempDir()
		seedProject(t, root)
		writeE2EFile(t, filepath.Join(root, "a.gd"), "extends Node\n")
		writeE2EFile(t, filepath.Join(root, "nested", "b.gd"), "extends Node\n")

		text := requireToolText(t, session, "validate_scripts", map[string]any{
			"projectPath": root,
			"scope":       "all",
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
	session := connect(t)

	t.Run("missing projectPath", func(t *testing.T) {
		requireToolError(t, session, "validate_scripts", map[string]any{}, "projectPath is required.")
	})

	t.Run("not a project", func(t *testing.T) {
		root := t.TempDir()
		requireToolError(t, session, "validate_scripts", map[string]any{
			"projectPath": root,
		}, "Not a valid Godot project: "+root)
	})
}
