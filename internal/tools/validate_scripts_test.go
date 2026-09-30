package tools

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/vilppuvuorinen/slopfood-godot-mcp/internal/godot"
)

type validateScriptsDoc struct {
	Scope           string           `json:"scope"`
	FileCount       int              `json:"fileCount"`
	FilesWithErrors int              `json:"filesWithErrors"`
	AllValid        bool             `json:"allValid"`
	Results         []map[string]any `json:"results"`
}

const fakeValidateGodot = `for arg in "$@"; do
  case "$arg" in
    *invalid*)
      echo 'SCRIPT ERROR: Parse Error: Unexpected token' >&2
      echo '    at: (res://invalid.gd:3)' >&2
      exit 1
      ;;
  esac
done
exit 0`

func decodeValidateScripts(t *testing.T, text string) validateScriptsDoc {
	t.Helper()
	var doc validateScriptsDoc
	if err := json.Unmarshal([]byte(text), &doc); err != nil {
		t.Fatalf("unmarshal %q: %v", text, err)
	}
	return doc
}

func TestValidateScriptsSchema(t *testing.T) {
	tool := validateScripts()
	if tool.Name != "validate_scripts" {
		t.Errorf("name = %q", tool.Name)
	}
	if tool.Description != "Batch-check GDScript files (git-changed by default, or all)" {
		t.Errorf("description = %q", tool.Description)
	}
	if tool.InputSchema["type"] != "object" {
		t.Errorf("schema type = %v", tool.InputSchema["type"])
	}
	required, ok := tool.InputSchema["required"].([]string)
	if !ok || len(required) != 0 {
		t.Errorf("required = %v", tool.InputSchema["required"])
	}
	props, ok := tool.InputSchema["properties"].(map[string]any)
	if !ok {
		t.Fatalf("properties = %T", tool.InputSchema["properties"])
	}
	scope, ok := props["scope"].(map[string]any)
	if !ok || scope["type"] != "string" {
		t.Errorf("scope property = %v", props["scope"])
	}
	if scope["default"] != "changed" {
		t.Errorf("scope default = %v, want changed", scope["default"])
	}
	scriptPaths, ok := props["scriptPaths"].(map[string]any)
	if !ok || scriptPaths["type"] != "array" {
		t.Errorf("scriptPaths property = %v", props["scriptPaths"])
	}
}

func TestValidateScriptsExplicit(t *testing.T) {
	t.Setenv(godot.PathEnv, fakeGodotOps(t, fakeValidateGodot))

	root := t.TempDir()
	makeProject(t, root)
	seedFile(t, filepath.Join(root, "valid.gd"), "extends Node\n")
	seedFile(t, filepath.Join(root, "invalid.gd"), "extends Node\nfunc b(\n")

	result := callToolArgs(t, validateScripts(), root, `{"scriptPaths":["valid.gd","invalid.gd"]}`)
	if result.IsError {
		t.Fatalf("unexpected error: %+v", result.Content)
	}
	doc := decodeValidateScripts(t, result.Content[0].Text)
	if doc.Scope != "explicit" {
		t.Errorf("scope = %q", doc.Scope)
	}
	if doc.FileCount != 2 || doc.FilesWithErrors != 1 || doc.AllValid {
		t.Errorf("fileCount=%d filesWithErrors=%d allValid=%v", doc.FileCount, doc.FilesWithErrors, doc.AllValid)
	}
	if len(doc.Results) != 2 {
		t.Fatalf("results = %+v", doc.Results)
	}
	valid := doc.Results[0]
	if valid["scriptPath"] != "valid.gd" || valid["checked"] != true || valid["valid"] != true {
		t.Errorf("valid result = %+v", valid)
	}
	if errors, ok := valid["errors"].([]any); !ok || len(errors) != 0 {
		t.Errorf("valid errors = %+v", valid["errors"])
	}
	invalid := doc.Results[1]
	if invalid["scriptPath"] != "invalid.gd" || invalid["checked"] != true || invalid["valid"] != false {
		t.Errorf("invalid result = %+v", invalid)
	}
	if invalid["errorCount"] != float64(1) {
		t.Errorf("errorCount = %v", invalid["errorCount"])
	}
	errors, ok := invalid["errors"].([]any)
	if !ok || len(errors) != 1 {
		t.Fatalf("errors = %+v", invalid["errors"])
	}
	diagnostic := errors[0].(map[string]any)
	if diagnostic["message"] != "Unexpected token" {
		t.Errorf("message = %v", diagnostic["message"])
	}
	if diagnostic["file"] != "invalid.gd" || diagnostic["line"] != float64(3) {
		t.Errorf("location = %v", diagnostic)
	}
}

func TestValidateScriptsScopeAll(t *testing.T) {
	t.Setenv(godot.PathEnv, fakeGodotOps(t, fakeValidateGodot))

	root := t.TempDir()
	makeProject(t, root)
	seedFile(t, filepath.Join(root, "a.gd"), "extends Node\n")
	seedFile(t, filepath.Join(root, "nested", "b.gd"), "extends Node\n")
	seedFile(t, filepath.Join(root, ".godot", "c.gd"), "extends Node\n")
	seedFile(t, filepath.Join(root, ".git", "d.gd"), "extends Node\n")
	seedFile(t, filepath.Join(root, "node_modules", "e.gd"), "extends Node\n")
	seedFile(t, filepath.Join(root, ".import", "f.gd"), "extends Node\n")
	seedFile(t, filepath.Join(root, ".hidden", "g.gd"), "extends Node\n")

	result := callToolArgs(t, validateScripts(), root, `{"scope":"all"}`)
	if result.IsError {
		t.Fatalf("unexpected error: %+v", result.Content)
	}
	doc := decodeValidateScripts(t, result.Content[0].Text)
	if doc.Scope != "all" {
		t.Errorf("scope = %q", doc.Scope)
	}
	if doc.FileCount != 2 || doc.FilesWithErrors != 0 || !doc.AllValid {
		t.Errorf("fileCount=%d filesWithErrors=%d allValid=%v", doc.FileCount, doc.FilesWithErrors, doc.AllValid)
	}
	if len(doc.Results) != 2 {
		t.Fatalf("results = %+v", doc.Results)
	}
	if doc.Results[0]["scriptPath"] != "a.gd" || doc.Results[1]["scriptPath"] != "nested/b.gd" {
		t.Errorf("paths = %v, %v", doc.Results[0]["scriptPath"], doc.Results[1]["scriptPath"])
	}
}

func TestValidateScriptsChangedOutsideGit(t *testing.T) {
	root := t.TempDir()
	makeProject(t, root)

	result := callToolArgs(t, validateScripts(), root, `{}`)
	if !result.IsError {
		t.Fatalf("expected an error result: %+v", result.Content)
	}
	want := `Not a git repository (or git is unavailable). Use scope: "all" or pass scriptPaths.`
	if result.Content[0].Text != want {
		t.Errorf("message = %q, want %q", result.Content[0].Text, want)
	}
}

func TestValidateScriptsExplicitSkips(t *testing.T) {
	t.Setenv(godot.PathEnv, fakeGodotOps(t, fakeValidateGodot))

	root := t.TempDir()
	makeProject(t, root)
	seedFile(t, filepath.Join(root, "valid.gd"), "extends Node\n")

	result := callToolArgs(t, validateScripts(), root, `{"scriptPaths":["notes.txt","missing.gd","../escape.gd","valid.gd"]}`)
	if result.IsError {
		t.Fatalf("unexpected error: %+v", result.Content)
	}
	doc := decodeValidateScripts(t, result.Content[0].Text)
	if doc.FileCount != 4 || doc.FilesWithErrors != 0 || doc.AllValid {
		t.Errorf("fileCount=%d filesWithErrors=%d allValid=%v", doc.FileCount, doc.FilesWithErrors, doc.AllValid)
	}
	if got := doc.Results[0]["error"]; got != "Not a valid .gd path" {
		t.Errorf("notes.txt error = %v", got)
	}
	if got := doc.Results[1]["error"]; got != "Script does not exist" {
		t.Errorf("missing.gd error = %v", got)
	}
	if got := doc.Results[2]["error"]; got != "Not a valid .gd path" {
		t.Errorf("escape.gd error = %v", got)
	}
	if got := doc.Results[3]["checked"]; got != true {
		t.Errorf("valid.gd checked = %v", got)
	}
}

func TestValidateScriptsErrors(t *testing.T) {
	t.Run("missing projectPath", func(t *testing.T) {
		result := callToolArgs(t, validateScripts(), "", `{}`)
		if !result.IsError || result.Content[0].Text != "Invalid path." {
			t.Fatalf("result = %+v", result.Content)
		}
	})

	t.Run("invalid path", func(t *testing.T) {
		result := callToolArgs(t, validateScripts(), "../escape", `{}`)
		if !result.IsError || result.Content[0].Text != "Invalid path." {
			t.Fatalf("result = %+v", result.Content)
		}
	})

	t.Run("not a project", func(t *testing.T) {
		root := t.TempDir()
		result := callToolArgs(t, validateScripts(), root, `{}`)
		if !result.IsError || result.Content[0].Text != "Not a valid Godot project: "+root {
			t.Fatalf("result = %+v", result.Content)
		}
	})

	t.Run("invalid scope", func(t *testing.T) {
		root := t.TempDir()
		makeProject(t, root)
		result := callToolArgs(t, validateScripts(), root, `{"scope":"bogus"}`)
		want := `Invalid scope "bogus". Use "changed" or "all", or pass scriptPaths.`
		if !result.IsError || result.Content[0].Text != want {
			t.Fatalf("result = %+v", result.Content)
		}
	})
}
