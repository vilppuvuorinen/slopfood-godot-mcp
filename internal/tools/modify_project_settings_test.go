package tools

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestModifyProjectSettingsSchema(t *testing.T) {
	tool := modifyProjectSettings()
	if tool.Name != "modify_project_settings" {
		t.Errorf("name = %q", tool.Name)
	}
	if tool.Description != "Modify a project.godot setting" {
		t.Errorf("description = %q", tool.Description)
	}
	required, ok := tool.InputSchema["required"].([]string)
	if !ok || len(required) != 4 ||
		required[0] != "projectPath" || required[1] != "section" ||
		required[2] != "key" || required[3] != "value" {
		t.Errorf("required = %v", tool.InputSchema["required"])
	}
	props, ok := tool.InputSchema["properties"].(map[string]any)
	if !ok || len(props) != 4 {
		t.Fatalf("properties = %v", tool.InputSchema["properties"])
	}
	for _, key := range []string{"projectPath", "section", "key", "value"} {
		prop, ok := props[key].(map[string]any)
		if !ok || prop["type"] != "string" {
			t.Errorf("%s property = %v", key, props[key])
		}
	}
}

func TestModifyProjectSettingsUpdateExistingKey(t *testing.T) {
	root := t.TempDir()
	makeProject(t, root)
	seedFile(t, filepath.Join(root, "project.godot"), `config_version=5

[application]

config/name="Demo"
run/main_scene="res://main.tscn"
`)
	result := callToolArgs(t, modifyProjectSettings(), `{"projectPath":`+jsonString(root)+
		`,"section":"application","key":"config/name","value":"\"New\""}`)
	if result.IsError {
		t.Fatalf("unexpected error: %+v", result.Content)
	}
	if got, want := result.Content[0].Text, `Setting updated: [application] config/name="New"`; got != want {
		t.Errorf("message = %q, want %q", got, want)
	}
	content, err := os.ReadFile(filepath.Join(root, "project.godot"))
	if err != nil {
		t.Fatalf("read project.godot: %v", err)
	}
	if !strings.Contains(string(content), `config/name="New"`) {
		t.Errorf("config/name not updated:\n%s", content)
	}
	if !strings.Contains(string(content), `run/main_scene="res://main.tscn"`) {
		t.Errorf("unrelated key lost:\n%s", content)
	}
}

func TestModifyProjectSettingsAddKeyToExistingSection(t *testing.T) {
	root := t.TempDir()
	makeProject(t, root)
	seedFile(t, filepath.Join(root, "project.godot"), `config_version=5

[application]

config/name="Demo"
`)
	result := callToolArgs(t, modifyProjectSettings(), `{"projectPath":`+jsonString(root)+
		`,"section":"application","key":"config/description","value":"\"Hi\""}`)
	if result.IsError {
		t.Fatalf("unexpected error: %+v", result.Content)
	}
	content, err := os.ReadFile(filepath.Join(root, "project.godot"))
	if err != nil {
		t.Fatalf("read project.godot: %v", err)
	}
	got := string(content)
	if !strings.Contains(got, "\nconfig/description=\"Hi\"\n") {
		t.Errorf("new key not inserted:\n%s", got)
	}
	if !strings.Contains(got, `config/name="Demo"`) {
		t.Errorf("existing key lost:\n%s", got)
	}
}

func TestModifyProjectSettingsAppendNewSection(t *testing.T) {
	root := t.TempDir()
	makeProject(t, root)
	seedFile(t, filepath.Join(root, "project.godot"), `config_version=5

[application]

config/name="Demo"
`)
	result := callToolArgs(t, modifyProjectSettings(), `{"projectPath":`+jsonString(root)+
		`,"section":"display","key":"window/size/viewport_width","value":"1280"}`)
	if result.IsError {
		t.Fatalf("unexpected error: %+v", result.Content)
	}
	content, err := os.ReadFile(filepath.Join(root, "project.godot"))
	if err != nil {
		t.Fatalf("read project.godot: %v", err)
	}
	got := string(content)
	if !strings.Contains(got, "\n\n[display]\n\nwindow/size/viewport_width=1280\n") {
		t.Errorf("new section not appended:\n%s", got)
	}
}

func TestModifyProjectSettingsEmptyValueCountsAsProvided(t *testing.T) {
	root := t.TempDir()
	makeProject(t, root)
	seedFile(t, filepath.Join(root, "project.godot"), `config_version=5

[application]

config/name="Demo"
`)
	result := callToolArgs(t, modifyProjectSettings(), `{"projectPath":`+jsonString(root)+
		`,"section":"application","key":"config/name","value":""}`)
	if result.IsError {
		t.Fatalf("unexpected error: %+v", result.Content)
	}
	content, err := os.ReadFile(filepath.Join(root, "project.godot"))
	if err != nil {
		t.Fatalf("read project.godot: %v", err)
	}
	if !strings.Contains(string(content), "config/name=\n") {
		t.Errorf("empty value not written:\n%s", content)
	}
}

// TestModifyProjectSettingsEscaping covers encoded section and key names, and
// replacing an existing value that itself contains an escaped quote. The value
// argument stays a raw Godot literal, as documented.
func TestModifyProjectSettingsEscaping(t *testing.T) {
	t.Run("encoded section and key", func(t *testing.T) {
		root := t.TempDir()
		makeProject(t, root)
		seedFile(t, filepath.Join(root, "project.godot"), "config_version=5\n")

		result := callToolArgs(t, modifyProjectSettings(),
			`{"projectPath":`+jsonString(root)+`,"section":`+jsonString("weird]section")+`,"key":`+jsonString("a\nb")+`,"value":"1280"}`)
		if result.IsError {
			t.Fatalf("unexpected error: %+v", result.Content)
		}
		data, err := os.ReadFile(filepath.Join(root, "project.godot"))
		if err != nil {
			t.Fatalf("read project.godot: %v", err)
		}
		content := string(data)
		if !strings.Contains(content, "[weird\\]section]") {
			t.Errorf("project.godot = %q, want the escaped section header", content)
		}
		if !strings.Contains(content, "\"a\nb\"=1280") {
			t.Errorf("project.godot = %q, want the quoted multi-line key", content)
		}
	})

	t.Run("replaces escaped existing value", func(t *testing.T) {
		root := t.TempDir()
		makeProject(t, root)
		seedFile(t, filepath.Join(root, "project.godot"), "config_version=5\n\n[application]\n\nconfig/description=\"say \\\"hi\\\"\"\nconfig/name=\"Demo\"\n")

		result := callToolArgs(t, modifyProjectSettings(),
			`{"projectPath":`+jsonString(root)+`,"section":"application","key":"config/description","value":"\"bye\""}`)
		if result.IsError {
			t.Fatalf("unexpected error: %+v", result.Content)
		}
		data, err := os.ReadFile(filepath.Join(root, "project.godot"))
		if err != nil {
			t.Fatalf("read project.godot: %v", err)
		}
		content := string(data)
		if strings.Contains(content, "say") {
			t.Errorf("old escaped value still present:\n%s", content)
		}
		if !strings.Contains(content, `config/description="bye"`) {
			t.Errorf("project.godot = %q", content)
		}
		if !strings.Contains(content, `config/name="Demo"`) {
			t.Errorf("unrelated key lost:\n%s", content)
		}
	})
}

func TestModifyProjectSettingsErrors(t *testing.T) {
	valid := t.TempDir()
	makeProject(t, valid)

	tests := []struct {
		name string
		args string
		want string
	}{
		{name: "missing args", args: `{}`, want: "projectPath, section, key, and value are required."},
		{name: "missing value", args: `{"projectPath":` + jsonString(valid) + `,"section":"application","key":"config/name"}`, want: "projectPath, section, key, and value are required."},
		{name: "invalid path", args: `{"projectPath":"../escape","section":"application","key":"config/name","value":"x"}`, want: "Invalid path."},
		{name: "not a project", args: `{"projectPath":` + jsonString(t.TempDir()) + `,"section":"application","key":"config/name","value":"x"}`, want: "Not a valid Godot project"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := callToolArgs(t, modifyProjectSettings(), tt.args)
			if !result.IsError {
				t.Fatalf("expected an error result: %+v", result.Content)
			}
			if got := result.Content[0].Text; !strings.HasPrefix(got, tt.want) {
				t.Errorf("message = %q, want prefix %q", got, tt.want)
			}
		})
	}
}
