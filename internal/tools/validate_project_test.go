package tools

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vilppuvuorinen/slopfood-godot-mcp/internal/godot"
	"github.com/vilppuvuorinen/slopfood-godot-mcp/internal/mcp"
)

func TestValidateProjectSchema(t *testing.T) {
	tool := validateProject()
	if tool.Name != "validate_project" {
		t.Errorf("name = %q", tool.Name)
	}
	if tool.Annotations == nil || tool.Annotations.ReadOnlyHint == nil || !*tool.Annotations.ReadOnlyHint {
		t.Errorf("annotations = %+v, want read-only", tool.Annotations)
	}
	props, ok := tool.OutputSchema["properties"].(map[string]any)
	if !ok {
		t.Fatalf("output properties = %T", tool.OutputSchema["properties"])
	}
	for _, key := range []string{"valid", "errors"} {
		if _, present := props[key]; !present {
			t.Errorf("output schema has no %q property: %v", key, props)
		}
	}
}

func TestValidateProjectValid(t *testing.T) {
	t.Setenv(godot.PathEnv, fakeGodotOps(t, ":"))

	root := t.TempDir()
	makeProject(t, root)

	result := callToolArgs(t, validateProject(), root, `{}`)
	if result.IsError {
		t.Fatalf("unexpected error: %+v", result.Content)
	}
	payload := decodeValidateProject(t, result)
	if !payload["valid"].(bool) {
		t.Errorf("valid = %v, want true", payload["valid"])
	}
	if errors, ok := payload["errors"].([]any); !ok || len(errors) != 0 {
		t.Errorf("errors = %#v, want empty", payload["errors"])
	}
}

func TestValidateProjectReportsParseDiagnostics(t *testing.T) {
	fake := fakeGodotOps(t, "echo \"ERROR: Error parsing '/proj/project.godot' at line 2\" >&2")
	t.Setenv(godot.PathEnv, fake)

	root := t.TempDir()
	makeProject(t, root)

	result := callToolArgs(t, validateProject(), root, `{}`)
	if result.IsError {
		t.Fatalf("expected a non-error result: %+v", result.Content)
	}
	payload := decodeValidateProject(t, result)
	if payload["valid"].(bool) {
		t.Error("valid = true, want false")
	}
	errors, ok := payload["errors"].([]any)
	if !ok || len(errors) != 1 {
		t.Fatalf("errors = %#v, want one diagnostic", payload["errors"])
	}
	if got, _ := errors[0].(string); !strings.Contains(got, "Error parsing") {
		t.Errorf("error = %q, want the parse diagnostic", got)
	}
}

func TestValidateProjectReportsProbeFailure(t *testing.T) {
	fake := fakeGodotOps(t, "exit 2")
	t.Setenv(godot.PathEnv, fake)

	root := t.TempDir()
	makeProject(t, root)

	result := callToolArgs(t, validateProject(), root, `{}`)
	if result.IsError {
		t.Fatalf("expected a non-error result: %+v", result.Content)
	}
	payload := decodeValidateProject(t, result)
	if payload["valid"].(bool) {
		t.Error("valid = true, want false")
	}
	errors, _ := payload["errors"].([]any)
	if len(errors) != 1 || !strings.Contains(errors[0].(string), "exited with code 2") {
		t.Errorf("errors = %#v, want an exit-code message", payload["errors"])
	}
}

func TestValidateProjectReportsEngineFailure(t *testing.T) {
	t.Setenv(godot.PathEnv, filepath.Join(t.TempDir(), "missing-godot"))

	root := t.TempDir()
	makeProject(t, root)

	result := callToolArgs(t, validateProject(), root, `{}`)
	if !result.IsError {
		t.Fatalf("expected an error result: %+v", result.Content)
	}
	if got := result.Content[0].Text; !strings.HasPrefix(got, "Failed to validate project:") {
		t.Errorf("message = %q", got)
	}
}

func TestValidateProjectErrors(t *testing.T) {
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
			result := callToolArgs(t, validateProject(), tt.project, `{}`)
			if !result.IsError {
				t.Fatalf("expected an error result: %+v", result.Content)
			}
			if got := result.Content[0].Text; !strings.HasPrefix(got, tt.want) {
				t.Errorf("message = %q, want prefix %q", got, tt.want)
			}
		})
	}
}

func decodeValidateProject(t *testing.T, result mcp.ToolResult) map[string]any {
	t.Helper()
	var payload map[string]any
	if err := json.Unmarshal([]byte(result.Content[0].Text), &payload); err != nil {
		t.Fatalf("unmarshal %q: %v", result.Content[0].Text, err)
	}
	return payload
}
