package tools

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vilppuvuorinen/slopfood-godot-mcp/internal/godot"
)

type validateScriptsDoc struct {
	FileCount       int              `json:"fileCount"`
	FilesWithErrors int              `json:"filesWithErrors"`
	AllValid        bool             `json:"allValid"`
	Results         []map[string]any `json:"results"`
}

const fakeValidateGodot = `status=0
for arg in "$@"; do
  case "$arg" in
    *.gd)
      echo "<<<GODOT_MCP_VALIDATE:$arg>>>" >&2
      case "$arg" in
        *invalid*)
          echo 'SCRIPT ERROR: Parse Error: Unexpected token' >&2
          echo '    at: (res://invalid.gd:3)' >&2
          status=1
          ;;
      esac
      ;;
  esac
done
exit $status`

func decodeValidateScripts(t *testing.T, text string) validateScriptsDoc {
	t.Helper()
	var doc validateScriptsDoc
	if err := json.Unmarshal([]byte(text), &doc); err != nil {
		t.Fatalf("unmarshal %q: %v", text, err)
	}
	return doc
}

func resultByPath(t *testing.T, doc validateScriptsDoc, path string) map[string]any {
	t.Helper()
	for _, result := range doc.Results {
		if result["scriptPath"] == path {
			return result
		}
	}
	t.Fatalf("no result for %q in %+v", path, doc.Results)
	return nil
}

func TestValidateScriptsSchema(t *testing.T) {
	tool := validateScripts()
	if tool.Name != "validate_scripts" {
		t.Errorf("name = %q", tool.Name)
	}
	if tool.Description != "Validate every GDScript file in the project" {
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
	if !ok || len(props) != 0 {
		t.Errorf("properties = %v, want none", tool.InputSchema["properties"])
	}
}

func TestValidateScriptsReportsBrokenAndValid(t *testing.T) {
	t.Setenv(godot.PathEnv, fakeGodotOps(t, fakeValidateGodot))

	root := t.TempDir()
	makeProject(t, root)
	seedFile(t, filepath.Join(root, "valid.gd"), "extends Node\n")
	seedFile(t, filepath.Join(root, "invalid.gd"), "extends Node\nfunc b(\n")

	result := callToolArgs(t, validateScripts(), root, `{}`)
	if result.IsError {
		t.Fatalf("unexpected error: %+v", result.Content)
	}
	doc := decodeValidateScripts(t, result.Content[0].Text)
	if doc.FileCount != 2 || doc.FilesWithErrors != 1 || doc.AllValid {
		t.Errorf("fileCount=%d filesWithErrors=%d allValid=%v", doc.FileCount, doc.FilesWithErrors, doc.AllValid)
	}
	if len(doc.Results) != 2 {
		t.Fatalf("results = %+v", doc.Results)
	}

	valid := resultByPath(t, doc, "valid.gd")
	if valid["checked"] != true || valid["valid"] != true {
		t.Errorf("valid result = %+v", valid)
	}
	if errors, ok := valid["errors"].([]any); !ok || len(errors) != 0 {
		t.Errorf("valid errors = %+v", valid["errors"])
	}

	invalid := resultByPath(t, doc, "invalid.gd")
	if invalid["checked"] != true || invalid["valid"] != false || invalid["errorCount"] != float64(1) {
		t.Errorf("invalid result = %+v", invalid)
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

func TestValidateScriptsWalksProject(t *testing.T) {
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

	result := callToolArgs(t, validateScripts(), root, `{}`)
	if result.IsError {
		t.Fatalf("unexpected error: %+v", result.Content)
	}
	doc := decodeValidateScripts(t, result.Content[0].Text)
	if doc.FileCount != 2 || doc.FilesWithErrors != 0 || !doc.AllValid {
		t.Errorf("fileCount=%d filesWithErrors=%d allValid=%v", doc.FileCount, doc.FilesWithErrors, doc.AllValid)
	}
	resultByPath(t, doc, "a.gd")
	resultByPath(t, doc, "nested/b.gd")
}

func TestValidateScriptsEngineFailure(t *testing.T) {
	t.Setenv(godot.PathEnv, filepath.Join(t.TempDir(), "missing-godot"))

	root := t.TempDir()
	makeProject(t, root)
	seedFile(t, filepath.Join(root, "valid.gd"), "extends Node\n")

	result := callToolArgs(t, validateScripts(), root, `{}`)
	if !result.IsError {
		t.Fatalf("expected an error result: %+v", result.Content)
	}
	if !strings.HasPrefix(result.Content[0].Text, "Failed to validate scripts:") {
		t.Errorf("message = %q", result.Content[0].Text)
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
}
