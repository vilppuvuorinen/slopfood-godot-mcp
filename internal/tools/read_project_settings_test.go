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
	props, ok := tool.InputSchema["properties"].(map[string]any)
	if !ok {
		t.Fatalf("properties = %T", tool.InputSchema["properties"])
	}
	if section, isMap := props["section"].(map[string]any); !isMap || section["type"] != "string" {
		t.Errorf("section property = %v, want string", props["section"])
	}
	if keys, isMap := props["keys"].(map[string]any); !isMap || keys["type"] != "array" {
		t.Errorf("keys property = %v, want array", props["keys"])
	}
	output, ok := tool.OutputSchema["properties"].(map[string]any)
	if !ok {
		t.Fatalf("output properties = %T", tool.OutputSchema["properties"])
	}
	if _, ok := output["sections"]; !ok {
		t.Errorf("output schema has no sections property: %v", output)
	}
}

// TestReadProjectSettingsFilters proves the optional section and keys filters
// bound the returned document without changing its shape.
func TestReadProjectSettingsFilters(t *testing.T) {
	t.Setenv(godot.PathEnv, fakeGodotOps(t, ":"))
	root := t.TempDir()
	makeProject(t, root)
	seedFile(t, filepath.Join(root, "project.godot"), `config_version=5

[application]

config/name="Demo"

[display]

window/size/viewport_width=1920
window/size/viewport_height=1080
`)

	t.Run("section", func(t *testing.T) {
		result := callToolArgs(t, readProjectSettings(), root, `{"section":"display"}`)
		if result.IsError {
			t.Fatalf("unexpected error: %+v", result.Content)
		}
		text := result.Content[0].Text
		if !strings.Contains(text, `"settingCount": 2`) {
			t.Errorf("text = %q, want two settings", text)
		}
		if !strings.Contains(text, `"window/size/viewport_width": 1920`) {
			t.Errorf("text = %q, want the selected section's key", text)
		}
		if strings.Contains(text, `"config/name"`) {
			t.Errorf("text = %q, want sections outside the filter dropped", text)
		}
	})

	t.Run("keys", func(t *testing.T) {
		result := callToolArgs(t, readProjectSettings(), root, `{"keys":["config/name","window/size/viewport_width"]}`)
		if result.IsError {
			t.Fatalf("unexpected error: %+v", result.Content)
		}
		text := result.Content[0].Text
		if !strings.Contains(text, `"settingCount": 2`) {
			t.Errorf("text = %q, want two settings", text)
		}
		if !strings.Contains(text, `"window/size/viewport_width": 1920`) || !strings.Contains(text, `"config/name": "Demo"`) {
			t.Errorf("text = %q, want both selected keys", text)
		}
		if strings.Contains(text, `"window/size/viewport_height"`) {
			t.Errorf("text = %q, want unselected keys dropped", text)
		}
	})

	t.Run("no match", func(t *testing.T) {
		result := callToolArgs(t, readProjectSettings(), root, `{"section":"missing"}`)
		if result.IsError {
			t.Fatalf("unexpected error: %+v", result.Content)
		}
		text := result.Content[0].Text
		if !strings.Contains(text, `"settingCount": 0`) || !strings.Contains(text, `"sections": {}`) {
			t.Errorf("text = %q, want an empty document", text)
		}
	})
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

	result := callToolArgs(t, readProjectSettings(), root, `{}`)
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

	result := callToolArgs(t, readProjectSettings(), root, `{}`)
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

	result := callToolArgs(t, readProjectSettings(), root, `{}`)
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

	result := callToolArgs(t, readProjectSettings(), root, `{}`)
	if !result.IsError {
		t.Fatalf("expected an error result: %+v", result.Content)
	}
	if got := result.Content[0].Text; !strings.HasPrefix(got, "Failed to read project settings:") {
		t.Errorf("message = %q", got)
	}
}

func TestReadProjectSettingsErrors(t *testing.T) {
	t.Setenv(godot.PathEnv, fakeGodot(t))
	notAProject := t.TempDir()
	tests := []struct {
		name    string
		project string
		args    string
		want    string
	}{
		{name: "missing projectPath", args: `{}`, want: "Invalid path."},
		{name: "invalid path", project: "../escape", args: `{}`, want: "Invalid path."},
		{name: "not a project", project: notAProject, args: `{}`, want: "Not a valid Godot project"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := callToolArgs(t, readProjectSettings(), tt.project, tt.args)
			if !result.IsError {
				t.Fatalf("expected an error result: %+v", result.Content)
			}
			if got := result.Content[0].Text; !strings.HasPrefix(got, tt.want) {
				t.Errorf("message = %q, want prefix %q", got, tt.want)
			}
		})
	}
}
