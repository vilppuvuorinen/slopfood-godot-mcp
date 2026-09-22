package tools

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
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
}

func TestReadProjectSettingsSuccess(t *testing.T) {
	root := t.TempDir()
	makeProject(t, root)
	seedFile(t, filepath.Join(root, "project.godot"), `config_version=5

[application]

config/name="Demo"
run/main_scene="res://main.tscn"

[autoload]

Game="*res://game.gd"
`)

	result := callToolArgs(t, readProjectSettings(), `{"projectPath":`+jsonString(root)+`}`)
	if result.IsError {
		t.Fatalf("unexpected error: %+v", result.Content)
	}
	var sections map[string]map[string]string
	if err := json.Unmarshal([]byte(result.Content[0].Text), &sections); err != nil {
		t.Fatalf("unmarshal %q: %v", result.Content[0].Text, err)
	}
	if got := sections["application"]["config/name"]; got != `"Demo"` {
		t.Errorf("config/name = %q", got)
	}
	if got := sections["application"]["run/main_scene"]; got != `"res://main.tscn"` {
		t.Errorf("run/main_scene = %q", got)
	}
	if got := sections["autoload"]["Game"]; got != `"*res://game.gd"` {
		t.Errorf("autoload Game = %q", got)
	}
}

func TestReadProjectSettingsSnakeCaseAlias(t *testing.T) {
	root := t.TempDir()
	makeProject(t, root)

	result := callToolArgs(t, readProjectSettings(), `{"project_path":`+jsonString(root)+`}`)
	if result.IsError {
		t.Fatalf("unexpected error: %+v", result.Content)
	}
}

func TestReadProjectSettingsErrors(t *testing.T) {
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
