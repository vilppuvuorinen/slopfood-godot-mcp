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
	if !ok || len(required) != 0 {
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
	// The engine probe only validates the project; the settings come from the
	// typed store, so the fake engine must simply succeed.
	fake := fakeGodotOps(t, ":")
	t.Setenv(godot.PathEnv, fake)

	root := t.TempDir()
	makeProject(t, root)
	seedFile(t, filepath.Join(root, "project.godot"), `config_version=5

[application]

config/name="Demo"
`)

	result := callToolArgs(t, readProjectSettings(), `{"projectPath":`+jsonString(root)+`}`)
	if result.IsError {
		t.Fatalf("unexpected error: %+v", result.Content)
	}
	// config_version lives in the section-less prefix and must be reported too.
	if got := result.Content[0].Text; !strings.Contains(got, `"settingCount": 2`) {
		t.Errorf("text = %q, want pretty settingCount", got)
	}
	if got := result.Content[0].Text; !strings.Contains(got, `"config/name": "Demo"`) {
		t.Errorf("text = %q, want the structured setting", got)
	}
	if got := result.Content[0].Text; !strings.Contains(got, `"config_version": 5`) {
		t.Errorf("text = %q, want the stored config_version", got)
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
		{name: "missing projectPath", args: `{}`, want: "Invalid path."},
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
