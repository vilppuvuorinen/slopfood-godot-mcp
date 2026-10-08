package tools

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vilppuvuorinen/slopfood-godot-mcp/internal/godot"
)

type validateReportDoc struct {
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

func decodeValidate(t *testing.T, text string) validateReportDoc {
	t.Helper()
	var doc validateReportDoc
	if err := json.Unmarshal([]byte(text), &doc); err != nil {
		t.Fatalf("unmarshal %q: %v", text, err)
	}
	return doc
}

func resultByPath(t *testing.T, doc validateReportDoc, path string) map[string]any {
	t.Helper()
	for _, result := range doc.Scripts.Results {
		if result["scriptPath"] == path {
			return result
		}
	}
	t.Fatalf("no result for %q in %+v", path, doc.Scripts.Results)
	return nil
}

const fakeValidateGodot = `has_target=0
for arg in "$@"; do
  case "$arg" in res://*) has_target=1 ;; esac
done
status=0
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
if [ "$has_target" = 0 ]; then
  echo "ERROR: Error parsing '/proj/project.godot' at line 2" >&2
fi
exit $status`

func TestValidateSchema(t *testing.T) {
	tool := validate()
	if tool.Name != "validate" {
		t.Errorf("name = %q", tool.Name)
	}
	if tool.Description != "Validate project.godot and every GDScript file in the project, returning all diagnostics" {
		t.Errorf("description = %q", tool.Description)
	}
	if tool.Annotations == nil || tool.Annotations.ReadOnlyHint == nil || !*tool.Annotations.ReadOnlyHint {
		t.Errorf("annotations = %+v, want read-only", tool.Annotations)
	}
	props, ok := tool.OutputSchema["properties"].(map[string]any)
	if !ok {
		t.Fatalf("output properties = %T", tool.OutputSchema["properties"])
	}
	for _, key := range []string{"valid", "project", "scripts"} {
		if _, present := props[key]; !present {
			t.Errorf("output schema has no %q property: %v", key, props)
		}
	}
}

func TestValidateValidProject(t *testing.T) {
	t.Setenv(godot.PathEnv, fakeGodotOps(t, ":"))

	root := t.TempDir()
	makeProject(t, root)

	result := callToolArgs(t, validate(), root, `{}`)
	if result.IsError {
		t.Fatalf("unexpected error: %+v", result.Content)
	}
	doc := decodeValidate(t, result.Content[0].Text)
	if !doc.Valid || !doc.Project.Valid || !doc.Scripts.AllValid {
		t.Errorf("report = %+v, want everything valid", doc)
	}
	if len(doc.Project.Errors) != 0 {
		t.Errorf("project errors = %v, want none", doc.Project.Errors)
	}
	if doc.Scripts.FileCount != 0 || len(doc.Scripts.Results) != 0 {
		t.Errorf("scripts = %+v, want empty", doc.Scripts)
	}
}

func TestValidateReportsProjectParseDiagnostics(t *testing.T) {
	fake := fakeGodotOps(t, `has_target=0
for arg in "$@"; do case "$arg" in res://*) has_target=1 ;; esac; done
if [ "$has_target" = 0 ]; then
  echo "ERROR: Error parsing '/proj/project.godot' at line 2" >&2
fi`)
	t.Setenv(godot.PathEnv, fake)

	root := t.TempDir()
	makeProject(t, root)

	result := callToolArgs(t, validate(), root, `{}`)
	if result.IsError {
		t.Fatalf("expected a non-error result: %+v", result.Content)
	}
	doc := decodeValidate(t, result.Content[0].Text)
	if doc.Valid || doc.Project.Valid {
		t.Errorf("valid = %v / project.valid = %v, want false", doc.Valid, doc.Project.Valid)
	}
	if len(doc.Project.Errors) != 1 || !strings.Contains(doc.Project.Errors[0], "Error parsing") {
		t.Fatalf("project errors = %#v, want one parse diagnostic", doc.Project.Errors)
	}
}

func TestValidateReportsBrokenAndValidScripts(t *testing.T) {
	t.Setenv(godot.PathEnv, fakeGodotOps(t, fakeValidateGodot))

	root := t.TempDir()
	makeProject(t, root)
	seedFile(t, filepath.Join(root, "valid.gd"), "extends Node\n")
	seedFile(t, filepath.Join(root, "invalid.gd"), "extends Node\nfunc b(\n")

	result := callToolArgs(t, validate(), root, `{}`)
	if result.IsError {
		t.Fatalf("unexpected error: %+v", result.Content)
	}
	doc := decodeValidate(t, result.Content[0].Text)
	// The fake reports a project.godot parse error when the probe runs without
	// script targets, so the project section is invalid alongside the scripts.
	if doc.Scripts.FileCount != 2 || doc.Scripts.FilesWithErrors != 1 || doc.Scripts.AllValid {
		t.Errorf("fileCount=%d filesWithErrors=%d allValid=%v", doc.Scripts.FileCount, doc.Scripts.FilesWithErrors, doc.Scripts.AllValid)
	}
	if len(doc.Scripts.Results) != 2 {
		t.Fatalf("results = %+v", doc.Scripts.Results)
	}
	if doc.Scripts.Error != "" {
		t.Errorf("scripts error = %q, want none", doc.Scripts.Error)
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

func TestValidateWalksProject(t *testing.T) {
	fake := fakeGodotOps(t, `status=0
for arg in "$@"; do
  case "$arg" in
    *.gd)
      echo "<<<GODOT_MCP_VALIDATE:$arg>>>" >&2
      ;;
  esac
done
exit $status`)
	t.Setenv(godot.PathEnv, fake)

	root := t.TempDir()
	makeProject(t, root)
	seedFile(t, filepath.Join(root, "a.gd"), "extends Node\n")
	seedFile(t, filepath.Join(root, "nested", "b.gd"), "extends Node\n")
	seedFile(t, filepath.Join(root, ".godot", "c.gd"), "extends Node\n")
	seedFile(t, filepath.Join(root, ".git", "d.gd"), "extends Node\n")
	seedFile(t, filepath.Join(root, "node_modules", "e.gd"), "extends Node\n")
	seedFile(t, filepath.Join(root, ".import", "f.gd"), "extends Node\n")
	seedFile(t, filepath.Join(root, ".hidden", "g.gd"), "extends Node\n")

	result := callToolArgs(t, validate(), root, `{}`)
	if result.IsError {
		t.Fatalf("unexpected error: %+v", result.Content)
	}
	doc := decodeValidate(t, result.Content[0].Text)
	if doc.Scripts.FileCount != 2 || doc.Scripts.FilesWithErrors != 0 || !doc.Scripts.AllValid {
		t.Errorf("fileCount=%d filesWithErrors=%d allValid=%v", doc.Scripts.FileCount, doc.Scripts.FilesWithErrors, doc.Scripts.AllValid)
	}
	resultByPath(t, doc, "a.gd")
	resultByPath(t, doc, "nested/b.gd")
}

func TestValidateReportsScriptSweepFailure(t *testing.T) {
	// The probe runs clean, but the checker fails on any script, so the script
	// section carries the failure while the report is still structured.
	fake := fakeGodotOps(t, `for arg in "$@"; do
  case "$arg" in
    res://*)
      echo "<<<GODOT_MCP_VALIDATE:$arg>>>" >&2
      echo 'ERROR: checker exploded' >&2
      exit 3
      ;;
  esac
done
exit 0`)
	t.Setenv(godot.PathEnv, fake)

	root := t.TempDir()
	makeProject(t, root)
	seedFile(t, filepath.Join(root, "valid.gd"), "extends Node\n")

	result := callToolArgs(t, validate(), root, `{}`)
	if result.IsError {
		t.Fatalf("expected a structured report: %+v", result.Content)
	}
	doc := decodeValidate(t, result.Content[0].Text)
	if doc.Valid || doc.Scripts.AllValid || doc.Scripts.Error == "" {
		t.Errorf("report = %+v, want an invalid scripts section with an error", doc)
	}
	if !doc.Project.Valid {
		t.Errorf("project.valid = false, want true")
	}
}

func TestValidateEngineFailure(t *testing.T) {
	t.Setenv(godot.PathEnv, filepath.Join(t.TempDir(), "missing-godot"))

	root := t.TempDir()
	makeProject(t, root)

	result := callToolArgs(t, validate(), root, `{}`)
	if !result.IsError {
		t.Fatalf("expected an error result: %+v", result.Content)
	}
	if got := result.Content[0].Text; !strings.HasPrefix(got, "Failed to validate:") {
		t.Errorf("message = %q", got)
	}
}

func TestValidateErrors(t *testing.T) {
	notAProject := t.TempDir()
	tests := []struct {
		name    string
		project string
		want    string
	}{
		{name: "missing projectPath", want: "Invalid path."},
		{name: "invalid path", project: "../escape", want: "Invalid path."},
		{name: "not a project", project: notAProject, want: "Not a valid Godot project"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := callToolArgs(t, validate(), tt.project, `{}`)
			if !result.IsError {
				t.Fatalf("expected an error result: %+v", result.Content)
			}
			if got := result.Content[0].Text; !strings.HasPrefix(got, tt.want) {
				t.Errorf("message = %q, want prefix %q", got, tt.want)
			}
		})
	}
}
