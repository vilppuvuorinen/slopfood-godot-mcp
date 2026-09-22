package tools

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestManagePluginsSchema(t *testing.T) {
	tool := managePlugins()
	if tool.Name != "manage_plugins" {
		t.Errorf("name = %q", tool.Name)
	}
	if tool.Description != "List/enable/disable editor plugins" {
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
		t.Fatalf("properties = %T", tool.InputSchema["properties"])
	}
	for _, key := range []string{"projectPath", "action", "pluginName"} {
		prop, ok := props[key].(map[string]any)
		if !ok || prop["type"] != "string" {
			t.Errorf("%s property = %v", key, props[key])
		}
	}
}

func TestManagePluginsList(t *testing.T) {
	root := t.TempDir()
	makeProject(t, root)
	seedFile(t, filepath.Join(root, "project.godot"), `config_version=5

[editor_plugins]

MyPlugin/enabled=true
`)
	if err := os.MkdirAll(filepath.Join(root, "addons", "SomeAddon"), 0o755); err != nil {
		t.Fatalf("mkdir addons: %v", err)
	}
	seedFile(t, filepath.Join(root, "addons", "not-a-dir.txt"), "x")

	result := callToolArgs(t, managePlugins(), `{"projectPath":`+jsonString(root)+`,"action":"list"}`)
	if result.IsError {
		t.Fatalf("unexpected error: %+v", result.Content)
	}
	var list pluginList
	if err := json.Unmarshal([]byte(result.Content[0].Text), &list); err != nil {
		t.Fatalf("unmarshal %q: %v", result.Content[0].Text, err)
	}
	if len(list.Enabled) != 1 || list.Enabled[0] != "MyPlugin" {
		t.Errorf("enabled = %v", list.Enabled)
	}
	if len(list.Available) != 1 || list.Available[0] != "SomeAddon" {
		t.Errorf("available = %v", list.Available)
	}
}

func TestManagePluginsListEmpty(t *testing.T) {
	root := t.TempDir()
	makeProject(t, root)

	result := callToolArgs(t, managePlugins(), `{"projectPath":`+jsonString(root)+`,"action":"list"}`)
	if result.IsError {
		t.Fatalf("unexpected error: %+v", result.Content)
	}
	if got := result.Content[0].Text; !strings.Contains(got, `"enabled": []`) || !strings.Contains(got, `"available": []`) {
		t.Errorf("text = %q, want empty arrays", got)
	}
}

func TestManagePluginsEnableAbsent(t *testing.T) {
	root := t.TempDir()
	makeProject(t, root)
	seedFile(t, filepath.Join(root, "project.godot"), `config_version=5

[editor_plugins]
`)

	result := callToolArgs(t, managePlugins(), `{"projectPath":`+jsonString(root)+`,"action":"enable","pluginName":"Foo"}`)
	if result.IsError {
		t.Fatalf("unexpected error: %+v", result.Content)
	}
	if got := result.Content[0].Text; got != "Plugin Foo enabled." {
		t.Errorf("text = %q", got)
	}
	content, err := os.ReadFile(filepath.Join(root, "project.godot"))
	if err != nil {
		t.Fatalf("read project.godot: %v", err)
	}
	if !strings.Contains(string(content), "[editor_plugins]\nFoo/enabled=true") {
		t.Errorf("project.godot = %q", content)
	}
}

func TestManagePluginsEnableAppendsSection(t *testing.T) {
	root := t.TempDir()
	makeProject(t, root)
	seedFile(t, filepath.Join(root, "project.godot"), "config_version=5\n")

	result := callToolArgs(t, managePlugins(), `{"projectPath":`+jsonString(root)+`,"action":"enable","pluginName":"Bar"}`)
	if result.IsError {
		t.Fatalf("unexpected error: %+v", result.Content)
	}
	if got := result.Content[0].Text; got != "Plugin Bar enabled." {
		t.Errorf("text = %q", got)
	}
	content, err := os.ReadFile(filepath.Join(root, "project.godot"))
	if err != nil {
		t.Fatalf("read project.godot: %v", err)
	}
	if !strings.Contains(string(content), "\n[editor_plugins]\nBar/enabled=true") {
		t.Errorf("project.godot = %q", content)
	}
}

func TestManagePluginsDisable(t *testing.T) {
	root := t.TempDir()
	makeProject(t, root)
	seedFile(t, filepath.Join(root, "project.godot"), `config_version=5

[editor_plugins]

MyPlugin/enabled=true
`)

	result := callToolArgs(t, managePlugins(), `{"projectPath":`+jsonString(root)+`,"action":"disable","pluginName":"MyPlugin"}`)
	if result.IsError {
		t.Fatalf("unexpected error: %+v", result.Content)
	}
	if got := result.Content[0].Text; got != "Plugin MyPlugin disabled." {
		t.Errorf("text = %q", got)
	}
	content, err := os.ReadFile(filepath.Join(root, "project.godot"))
	if err != nil {
		t.Fatalf("read project.godot: %v", err)
	}
	if !strings.Contains(string(content), "MyPlugin/enabled=false") {
		t.Errorf("project.godot = %q", content)
	}
}

func TestManagePluginsErrors(t *testing.T) {
	projectRoot := t.TempDir()
	makeProject(t, projectRoot)

	tests := []struct {
		name string
		args string
		want string
	}{
		{
			name: "missing params",
			args: `{}`,
			want: "projectPath and action are required.",
		},
		{
			name: "missing action",
			args: `{"projectPath":` + jsonString(projectRoot) + `}`,
			want: "projectPath and action are required.",
		},
		{
			name: "invalid path",
			args: `{"projectPath":"../escape","action":"list"}`,
			want: "Invalid path.",
		},
		{
			name: "not a project",
			args: `{"projectPath":` + jsonString(t.TempDir()) + `,"action":"list"}`,
			want: "Not a valid Godot project: ",
		},
		{
			name: "pluginName required",
			args: `{"projectPath":` + jsonString(projectRoot) + `,"action":"enable"}`,
			want: "pluginName is required.",
		},
		{
			name: "unknown action",
			args: `{"projectPath":` + jsonString(projectRoot) + `,"action":"bogus"}`,
			want: "Unknown action: bogus",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := callToolArgs(t, managePlugins(), tt.args)
			if !result.IsError {
				t.Fatalf("expected an error result: %+v", result.Content)
			}
			if got := result.Content[0].Text; !strings.HasPrefix(got, tt.want) {
				t.Errorf("message = %q, want prefix %q", got, tt.want)
			}
		})
	}
}
