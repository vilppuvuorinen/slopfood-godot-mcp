package tools

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSetMainSceneSchema(t *testing.T) {
	tool := setMainScene()
	if tool.Name != "set_main_scene" {
		t.Errorf("name = %q", tool.Name)
	}
	if tool.Description != "Set the main scene in project.godot" {
		t.Errorf("description = %q", tool.Description)
	}
	required, ok := tool.InputSchema["required"].([]string)
	if !ok || strings.Join(required, ",") != "projectPath,scenePath" {
		t.Errorf("required = %v", tool.InputSchema["required"])
	}
	props, ok := tool.InputSchema["properties"].(map[string]any)
	if !ok {
		t.Fatalf("properties = %T", tool.InputSchema["properties"])
	}
	for _, key := range []string{"projectPath", "scenePath"} {
		if _, ok := props[key]; !ok {
			t.Errorf("missing property %q", key)
		}
	}
}

func TestSetMainSceneSuccess(t *testing.T) {
	const withExisting = `config_version=5

[application]

config/name="Demo"
run/main_scene="res://old.tscn"

[autoload]

Game="*res://game.gd"
`
	const withoutKey = `config_version=5

[application]

config/name="Demo"
`
	const withoutSection = "config_version=5\n"

	tests := []struct {
		name     string
		seed     string
		scene    string
		want     string
		wantGone string
		wantKept []string
	}{
		{
			name:     "replaces existing main scene",
			seed:     withExisting,
			scene:    "scenes/main.tscn",
			want:     `run/main_scene="res://scenes/main.tscn"`,
			wantGone: `run/main_scene="res://old.tscn"`,
			wantKept: []string{`config/name="Demo"`, `Game="*res://game.gd"`},
		},
		{
			name:     "inserts into existing application section",
			seed:     withoutKey,
			scene:    "scenes/main.tscn",
			want:     "[application]\n\n" + `run/main_scene="res://scenes/main.tscn"`,
			wantKept: []string{`config/name="Demo"`},
		},
		{
			name:     "appends a new application section",
			seed:     withoutSection,
			scene:    "scenes/main.tscn",
			want:     "\n[application]\n\n" + `run/main_scene="res://scenes/main.tscn"` + "\n",
			wantKept: []string{"config_version=5"},
		},
		{
			name:     "does not double prefix res://",
			seed:     withoutSection,
			scene:    "res://scenes/main.tscn",
			want:     `run/main_scene="res://scenes/main.tscn"`,
			wantKept: []string{"config_version=5"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			makeProject(t, root)
			seedFile(t, filepath.Join(root, "project.godot"), tt.seed)

			result := callToolArgs(t, setMainScene(), `{"projectPath":`+jsonString(root)+`,"scenePath":`+jsonString(tt.scene)+`}`)
			if result.IsError {
				t.Fatalf("unexpected error: %+v", result.Content)
			}
			if len(result.Content) != 1 {
				t.Fatalf("content = %+v", result.Content)
			}
			wantText := "Main scene set to " + tt.scene
			if !strings.HasPrefix(tt.scene, "res://") {
				wantText = "Main scene set to res://" + tt.scene
			}
			if got := result.Content[0].Text; got != wantText {
				t.Errorf("text = %q, want %q", got, wantText)
			}

			data, err := os.ReadFile(filepath.Join(root, "project.godot"))
			if err != nil {
				t.Fatalf("read project.godot: %v", err)
			}
			content := string(data)
			if !strings.Contains(content, tt.want) {
				t.Errorf("content = %q, want it to contain %q", content, tt.want)
			}
			if tt.wantGone != "" && strings.Contains(content, tt.wantGone) {
				t.Errorf("content = %q, want it to drop %q", content, tt.wantGone)
			}
			for _, kept := range tt.wantKept {
				if !strings.Contains(content, kept) {
					t.Errorf("content = %q, want it to keep %q", content, kept)
				}
			}
			if strings.Contains(content, "res://res://") {
				t.Errorf("content = %q, double prefixed", content)
			}
		})
	}
}

func TestSetMainSceneSnakeCaseAliases(t *testing.T) {
	root := t.TempDir()
	makeProject(t, root)

	result := callToolArgs(t, setMainScene(), `{"project_path":`+jsonString(root)+`,"scene_path":"scenes/main.tscn"}`)
	if result.IsError {
		t.Fatalf("unexpected error: %+v", result.Content)
	}
	if result.Content[0].Text != "Main scene set to res://scenes/main.tscn" {
		t.Errorf("text = %q", result.Content[0].Text)
	}
}

func TestSetMainSceneErrors(t *testing.T) {
	root := t.TempDir()
	makeProject(t, root)

	tests := []struct {
		name string
		args string
		want string
	}{
		{name: "missing both", args: `{}`, want: "projectPath and scenePath are required."},
		{name: "missing scenePath", args: `{"projectPath":` + jsonString(root) + `}`, want: "projectPath and scenePath are required."},
		{name: "invalid path", args: `{"projectPath":"../escape","scenePath":"main.tscn"}`, want: "Invalid path."},
		{name: "not a project", args: `{"projectPath":` + jsonString(t.TempDir()) + `,"scenePath":"main.tscn"}`, want: "Not a valid Godot project"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := callToolArgs(t, setMainScene(), tt.args)
			if !result.IsError {
				t.Fatalf("expected an error result: %+v", result.Content)
			}
			if got := result.Content[0].Text; !strings.HasPrefix(got, tt.want) {
				t.Errorf("message = %q, want prefix %q", got, tt.want)
			}
		})
	}
}
