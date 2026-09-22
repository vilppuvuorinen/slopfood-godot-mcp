package tools

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestManageAutoloadsSchema(t *testing.T) {
	tool := manageAutoloads()
	if tool.Name != "manage_autoloads" {
		t.Errorf("name = %q", tool.Name)
	}
	if tool.Description != "Add, remove, or list autoloads in a Godot project" {
		t.Errorf("description = %q", tool.Description)
	}
	if tool.InputSchema["type"] != "object" {
		t.Errorf("schema type = %v", tool.InputSchema["type"])
	}
	required, ok := tool.InputSchema["required"].([]string)
	if !ok || len(required) != 2 || required[0] != "projectPath" || required[1] != "action" {
		t.Errorf("required = %v", tool.InputSchema["required"])
	}
	props, ok := tool.InputSchema["properties"].(map[string]any)
	if !ok {
		t.Fatalf("properties type = %T", tool.InputSchema["properties"])
	}
	for _, key := range []string{"projectPath", "action", "name", "path"} {
		if _, ok := props[key]; !ok {
			t.Errorf("missing property %q", key)
		}
	}
}

func TestManageAutoloadsList(t *testing.T) {
	t.Run("parses entries", func(t *testing.T) {
		root := t.TempDir()
		makeProject(t, root)
		seedFile(t, filepath.Join(root, "project.godot"), `config_version=5

[autoload]

Game="*res://game.gd"
Music="*res://music.gd"
`)
		result := callToolArgs(t, manageAutoloads(), `{"projectPath":`+jsonString(root)+`,"action":"list"}`)
		if result.IsError {
			t.Fatalf("unexpected error: %+v", result.Content)
		}
		var autoloads map[string]string
		if err := json.Unmarshal([]byte(result.Content[0].Text), &autoloads); err != nil {
			t.Fatalf("unmarshal %q: %v", result.Content[0].Text, err)
		}
		if got := autoloads["Game"]; got != `"*res://game.gd"` {
			t.Errorf("Game = %q", got)
		}
		if got := autoloads["Music"]; got != `"*res://music.gd"` {
			t.Errorf("Music = %q", got)
		}
	})

	t.Run("absent section is empty object", func(t *testing.T) {
		root := t.TempDir()
		makeProject(t, root)
		seedFile(t, filepath.Join(root, "project.godot"), "config_version=5\n")

		result := callToolArgs(t, manageAutoloads(), `{"projectPath":`+jsonString(root)+`,"action":"list"}`)
		if result.IsError {
			t.Fatalf("unexpected error: %+v", result.Content)
		}
		if got := strings.TrimSpace(result.Content[0].Text); got != "{}" {
			t.Errorf("text = %q, want {}", got)
		}
	})
}

func TestManageAutoloadsAdd(t *testing.T) {
	t.Run("existing section", func(t *testing.T) {
		root := t.TempDir()
		makeProject(t, root)
		seedFile(t, filepath.Join(root, "project.godot"), `config_version=5

[application]

config/name="Demo"

[autoload]

Game="*res://game.gd"
`)
		result := callToolArgs(t, manageAutoloads(), `{"projectPath":`+jsonString(root)+`,"action":"add","name":"Music","path":"res://music.gd"}`)
		if result.IsError {
			t.Fatalf("unexpected error: %+v", result.Content)
		}
		if got, want := result.Content[0].Text, `Autoload "Music" added: res://music.gd`; got != want {
			t.Errorf("text = %q, want %q", got, want)
		}
		data, err := os.ReadFile(filepath.Join(root, "project.godot"))
		if err != nil {
			t.Fatalf("read project.godot: %v", err)
		}
		content := string(data)
		if !strings.Contains(content, `Music="*res://music.gd"`) {
			t.Errorf("missing added autoload in:\n%s", content)
		}
		if !strings.Contains(content, `Game="*res://game.gd"`) {
			t.Errorf("existing autoload lost in:\n%s", content)
		}
		if strings.Count(content, "[autoload]") != 1 {
			t.Errorf("section duplicated in:\n%s", content)
		}
	})

	t.Run("creates missing section", func(t *testing.T) {
		root := t.TempDir()
		makeProject(t, root)
		seedFile(t, filepath.Join(root, "project.godot"), "config_version=5\n")

		result := callToolArgs(t, manageAutoloads(), `{"projectPath":`+jsonString(root)+`,"action":"add","name":"Game","path":"res://game.gd"}`)
		if result.IsError {
			t.Fatalf("unexpected error: %+v", result.Content)
		}
		data, err := os.ReadFile(filepath.Join(root, "project.godot"))
		if err != nil {
			t.Fatalf("read project.godot: %v", err)
		}
		content := string(data)
		if !strings.Contains(content, "[autoload]") {
			t.Errorf("missing section in:\n%s", content)
		}
		if !strings.Contains(content, `Game="*res://game.gd"`) {
			t.Errorf("missing added autoload in:\n%s", content)
		}
	})
}

func TestManageAutoloadsRemove(t *testing.T) {
	root := t.TempDir()
	makeProject(t, root)
	seedFile(t, filepath.Join(root, "project.godot"), `config_version=5

[autoload]

Game="*res://game.gd"
Music="*res://music.gd"
`)
	result := callToolArgs(t, manageAutoloads(), `{"projectPath":`+jsonString(root)+`,"action":"remove","name":"Music"}`)
	if result.IsError {
		t.Fatalf("unexpected error: %+v", result.Content)
	}
	if got, want := result.Content[0].Text, `Autoload "Music" removed.`; got != want {
		t.Errorf("text = %q, want %q", got, want)
	}
	data, err := os.ReadFile(filepath.Join(root, "project.godot"))
	if err != nil {
		t.Fatalf("read project.godot: %v", err)
	}
	content := string(data)
	if strings.Contains(content, "Music=") {
		t.Errorf("autoload not removed in:\n%s", content)
	}
	if !strings.Contains(content, `Game="*res://game.gd"`) {
		t.Errorf("other autoload lost in:\n%s", content)
	}
}

func TestManageAutoloadsErrors(t *testing.T) {
	project := t.TempDir()
	makeProject(t, project)

	tests := []struct {
		name string
		args string
		want string
	}{
		{name: "missing args", args: `{}`, want: "projectPath and action are required."},
		{name: "missing action", args: `{"projectPath":` + jsonString(project) + `}`, want: "projectPath and action are required."},
		{name: "invalid path", args: `{"projectPath":"../escape","action":"list"}`, want: "Invalid path."},
		{name: "not a project", args: `{"projectPath":` + jsonString(t.TempDir()) + `,"action":"list"}`, want: "Not a valid Godot project"},
		{name: "invalid action", args: `{"projectPath":` + jsonString(project) + `,"action":"bogus"}`, want: `Invalid action. Use "list", "add", or "remove".`},
		{name: "add missing name and path", args: `{"projectPath":` + jsonString(project) + `,"action":"add"}`, want: "name and path are required for add action."},
		{name: "remove missing name", args: `{"projectPath":` + jsonString(project) + `,"action":"remove"}`, want: "name is required for remove action."},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := callToolArgs(t, manageAutoloads(), tt.args)
			if !result.IsError {
				t.Fatalf("expected an error result: %+v", result.Content)
			}
			if got := result.Content[0].Text; !strings.HasPrefix(got, tt.want) {
				t.Errorf("message = %q, want prefix %q", got, tt.want)
			}
		})
	}
}
