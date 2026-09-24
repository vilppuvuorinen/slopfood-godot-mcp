package tools

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/vilppuvuorinen/go-dot-mcp/internal/godot"
)

func TestReadProjectSettingsSchema(t *testing.T) {
	tool := readProjectSettings()
	if tool.Name != "read_project_settings" {
		t.Errorf("name = %q", tool.Name)
	}
	if tool.Description != "Read project.godot as structured JSON" {
		t.Errorf("description = %q", tool.Description)
	}
	required, ok := tool.InputSchema["required"].([]string)
	if !ok || len(required) != 1 || required[0] != "projectPath" {
		t.Errorf("required = %v", tool.InputSchema["required"])
	}
	output, ok := tool.OutputSchema["properties"].(map[string]any)
	if !ok {
		t.Fatalf("output properties = %T", tool.OutputSchema["properties"])
	}
	if _, ok := output["sections"]; !ok {
		t.Errorf("output schema has no sections property: %v", output)
	}
}

func TestReadProjectSettingsSuccess(t *testing.T) {
	fake := fakeGodotOps(t, "printf '<<<GO_DOT_MCP_JSON_START>>>\\n{\"projectPath\":\"/proj/\",\"settingCount\":1,\"sections\":{\"application\":{\"config/name\":\"Demo\"}}}\\n<<<GO_DOT_MCP_JSON_END>>>\\n'")
	t.Setenv(godot.PathEnv, fake)

	root := t.TempDir()
	makeProject(t, root)

	result := callToolArgs(t, readProjectSettings(), `{"projectPath":`+jsonString(root)+`}`)
	if result.IsError {
		t.Fatalf("unexpected error: %+v", result.Content)
	}
	if got := result.Content[0].Text; !strings.Contains(got, `"settingCount": 1`) {
		t.Errorf("text = %q, want pretty settingCount", got)
	}
	if got := result.Content[0].Text; !strings.Contains(got, `"config/name": "Demo"`) {
		t.Errorf("text = %q, want the structured setting", got)
	}
}

func TestReadProjectSettingsSnakeCaseAlias(t *testing.T) {
	fake := fakeGodotOps(t, "printf '<<<GO_DOT_MCP_JSON_START>>>\\n{\"settingCount\":0,\"sections\":{}}\\n<<<GO_DOT_MCP_JSON_END>>>\\n'")
	t.Setenv(godot.PathEnv, fake)
	root := t.TempDir()
	makeProject(t, root)

	result := callToolArgs(t, readProjectSettings(), `{"project_path":`+jsonString(root)+`}`)
	if result.IsError {
		t.Fatalf("unexpected error: %+v", result.Content)
	}
}

// TestReadProjectSettingsRejectsUnparseableProject confirms the Godot load
// probe surfaces a project.godot the engine rejects instead of returning the
// Go parser's view. The fake engine reports a fixed parse diagnostic.
func TestReadProjectSettingsRejectsUnparseableProject(t *testing.T) {
	fake := fakeGodotOps(t, "echo \"ERROR: Error parsing '/proj/project.godot' at line 2\" >&2")
	t.Setenv(godot.PathEnv, fake)

	root := t.TempDir()
	makeProject(t, root)

	result := callToolArgs(t, readProjectSettings(), `{"projectPath":`+jsonString(root)+`}`)
	if !result.IsError {
		t.Fatalf("expected an error result: %+v", result.Content)
	}
	if got := result.Content[0].Text; !strings.HasPrefix(got, "Godot could not parse project.godot:") {
		t.Errorf("message = %q", got)
	}
}

// TestReadProjectSettingsReportsProbeFailure confirms a failed probe is
// reported: the exit code and stderr are combined, so a non-zero exit is a
// failure even without a project.godot parse diagnostic.
func TestReadProjectSettingsReportsProbeFailure(t *testing.T) {
	fake := fakeGodotOps(t, "echo 'validate_project crashed' >&2; exit 2")
	t.Setenv(godot.PathEnv, fake)

	root := t.TempDir()
	makeProject(t, root)

	result := callToolArgs(t, readProjectSettings(), `{"projectPath":`+jsonString(root)+`}`)
	if !result.IsError {
		t.Fatalf("expected an error result: %+v", result.Content)
	}
	if got := result.Content[0].Text; !strings.HasPrefix(got, "Failed to read project settings:") || !strings.Contains(got, "validate_project crashed") {
		t.Errorf("message = %q", got)
	}
}

// TestReadProjectSettingsReportsEngineFailure points GODOT_PATH at a missing
// binary so the probe cannot start, and asserts the tool reports the failure
// instead of silently falling back to the Go parser's view.
func TestReadProjectSettingsReportsEngineFailure(t *testing.T) {
	t.Setenv(godot.PathEnv, filepath.Join(t.TempDir(), "missing-godot"))

	root := t.TempDir()
	makeProject(t, root)

	result := callToolArgs(t, readProjectSettings(), `{"projectPath":`+jsonString(root)+`}`)
	if !result.IsError {
		t.Fatalf("expected an error result: %+v", result.Content)
	}
	if got := result.Content[0].Text; !strings.HasPrefix(got, "Failed to read project settings:") {
		t.Errorf("message = %q", got)
	}
}

func TestReadProjectSettingsErrors(t *testing.T) {
	t.Setenv(godot.PathEnv, fakeGodot(t))
	tests := []struct {
		name string
		args string
		want string
	}{
		{name: "missing projectPath", args: `{}`, want: "projectPath is required."},
		{name: "invalid path", args: `{"projectPath":"../escape"}`, want: "Invalid path."},
		{name: "not a project", args: `{"projectPath":` + jsonString(t.TempDir()) + `}`, want: "Not a valid Godot project"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := callToolArgs(t, readProjectSettings(), tt.args)
			if !result.IsError {
				t.Fatalf("expected an error result: %+v", result.Content)
			}
			if got := result.Content[0].Text; !strings.HasPrefix(got, tt.want) {
				t.Errorf("message = %q, want prefix %q", got, tt.want)
			}
		})
	}
}
