package e2e

import (
	"encoding/json"
	"path/filepath"
	"testing"
)

// validateScriptResponse mirrors validate_script's JSON payload.
type validateScriptResponse struct {
	Valid      bool   `json:"valid"`
	ScriptPath string `json:"scriptPath"`
	ErrorCount int    `json:"errorCount"`
	Errors     []struct {
		Message string `json:"message"`
		File    string `json:"file"`
		Line    int    `json:"line"`
	} `json:"errors"`
}

// TestValidateScriptSchema ports the reference's tool-definition expectations
// for validate_script over the real stdio surface.
func TestValidateScriptSchema(t *testing.T) {
	schema := requireSchema(t, "validate_script", "Check a GDScript file for syntax/type errors (headless, no run)")
	schema.requireRequired(t, "projectPath", "scriptPath")
	schema.requireStringProps(t, "projectPath", "scriptPath")
	schema.requirePropCount(t, 2)
}

// TestValidateScriptTool drives the real validate_script tool against Godot.
func TestValidateScriptTool(t *testing.T) {
	session := connect(t)
	requireGodot(t, session)

	t.Run("valid script", func(t *testing.T) {
		root := t.TempDir()
		seedProject(t, root)
		writeE2EFile(t, filepath.Join(root, "scripts", "player.gd"), "extends Node\n\nfunc _ready():\n\tpass\n")

		text := requireToolText(t, session, "validate_script", map[string]any{
			"projectPath": root,
			"scriptPath":  "scripts/player.gd",
		})

		var payload validateScriptResponse
		if err := json.Unmarshal([]byte(text), &payload); err != nil {
			t.Fatalf("unmarshal %q: %v", text, err)
		}
		if !payload.Valid || payload.ErrorCount != 0 || len(payload.Errors) != 0 {
			t.Errorf("payload = %+v", payload)
		}
	})

	t.Run("invalid script", func(t *testing.T) {
		root := t.TempDir()
		seedProject(t, root)
		writeE2EFile(t, filepath.Join(root, "scripts", "broken.gd"), "extends Node\nfunc _ready(:\n\tpass\n")

		text := requireToolText(t, session, "validate_script", map[string]any{
			"projectPath": root,
			"scriptPath":  "scripts/broken.gd",
		})

		var payload validateScriptResponse
		if err := json.Unmarshal([]byte(text), &payload); err != nil {
			t.Fatalf("unmarshal %q: %v", text, err)
		}
		if payload.Valid || payload.ErrorCount == 0 || len(payload.Errors) == 0 {
			t.Errorf("payload = %+v", payload)
		}
	})
}

// TestValidateScriptErrors covers the reference's validation messages.
func TestValidateScriptErrors(t *testing.T) {
	session := connect(t)

	t.Run("missing paths", func(t *testing.T) {
		requireToolError(t, session, "validate_script", map[string]any{}, "projectPath and scriptPath are required.")
	})

	t.Run("non gd", func(t *testing.T) {
		root := t.TempDir()
		seedProject(t, root)
		requireToolError(t, session, "validate_script", map[string]any{
			"projectPath": root,
			"scriptPath":  "player.cs",
		}, "validate_script only checks GDScript (.gd) files.")
	})

	t.Run("missing script", func(t *testing.T) {
		root := t.TempDir()
		seedProject(t, root)
		requireToolError(t, session, "validate_script", map[string]any{
			"projectPath": root,
			"scriptPath":  "missing.gd",
		}, "Script does not exist: missing.gd")
	})
}
