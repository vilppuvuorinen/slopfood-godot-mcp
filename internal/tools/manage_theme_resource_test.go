package tools

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/vilppuvuorinen/go-dot-mcp/internal/godot"
)

func TestManageThemeResourceSchema(t *testing.T) {
	tool := manageThemeResource()
	if tool.Name != "manage_theme_resource" {
		t.Errorf("name = %q", tool.Name)
	}
	if tool.Description != "Create/read/modify Theme .tres resources" {
		t.Errorf("description = %q", tool.Description)
	}
	required, ok := tool.InputSchema["required"].([]string)
	if !ok || strings.Join(required, ",") != "projectPath,resourcePath,action" {
		t.Errorf("required = %v", tool.InputSchema["required"])
	}
	props, ok := tool.InputSchema["properties"].(map[string]any)
	if !ok {
		t.Fatalf("properties = %T", tool.InputSchema["properties"])
	}
	for _, key := range []string{"projectPath", "resourcePath", "action"} {
		prop, propOK := props[key].(map[string]any)
		if !propOK || prop["type"] != "string" {
			t.Errorf("property %q = %v, want string", key, props[key])
		}
	}
	properties, ok := props["properties"].(map[string]any)
	if !ok || properties["type"] != "object" {
		t.Errorf("properties property = %v", props["properties"])
	}
}

func TestManageThemeResourceSuccess(t *testing.T) {
	t.Setenv(godot.PathEnv, fakeGodotOps(t, `echo "$@"`))
	root := t.TempDir()
	makeProject(t, root)

	tests := []struct {
		name       string
		args       string
		wantParams []string
		omit       []string
	}{
		{
			name:       "create with properties",
			args:       `{"projectPath":` + jsonString(root) + `,"resourcePath":"themes/main.tres","action":"create","properties":{"default_font_size":16}}`,
			wantParams: []string{`"action":"create"`, `"resource_path":"themes/main.tres"`, `"properties":{"default_font_size":16}`},
		},
		{
			name:       "read without properties",
			args:       `{"projectPath":` + jsonString(root) + `,"resourcePath":"themes/main.tres","action":"read"}`,
			wantParams: []string{`"action":"read"`, `"resource_path":"themes/main.tres"`},
			omit:       []string{`"properties"`},
		},
		{
			name:       "modify with properties",
			args:       `{"projectPath":` + jsonString(root) + `,"resourcePath":"themes/main.tres","action":"modify","properties":{"default_font_size":20}}`,
			wantParams: []string{`"action":"modify"`, `"resource_path":"themes/main.tres"`, `"properties":{"default_font_size":20}`},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := callToolArgs(t, manageThemeResource(), tt.args)
			if result.IsError {
				t.Fatalf("unexpected error: %+v", result.Content)
			}
			if len(result.Content) != 1 {
				t.Fatalf("content = %+v", result.Content)
			}
			text := result.Content[0].Text
			if !strings.HasPrefix(text, "manage_theme_resource succeeded.") {
				t.Errorf("text = %q", text)
			}
			for _, want := range tt.wantParams {
				if !strings.Contains(text, want) {
					t.Errorf("text = %q, want it to contain %q", text, want)
				}
			}
			for _, unwanted := range tt.omit {
				if strings.Contains(text, unwanted) {
					t.Errorf("text = %q, want it to omit %q", text, unwanted)
				}
			}
		})
	}
}

func TestManageThemeResourceValidation(t *testing.T) {
	root := t.TempDir()
	makeProject(t, root)

	tests := []struct {
		name string
		args string
		want string
	}{
		{name: "missing params", args: `{}`, want: "projectPath, resourcePath, and action are required."},
		{name: "missing action", args: `{"projectPath":` + jsonString(root) + `,"resourcePath":"themes/main.tres"}`, want: "projectPath, resourcePath, and action are required."},
		{name: "missing resource path", args: `{"projectPath":` + jsonString(root) + `,"action":"read"}`, want: "projectPath, resourcePath, and action are required."},
		{name: "invalid project path", args: `{"projectPath":"../escape","resourcePath":"themes/main.tres","action":"read"}`, want: "Invalid path."},
		{name: "invalid resource path", args: `{"projectPath":` + jsonString(root) + `,"resourcePath":"../escape.tres","action":"read"}`, want: "Invalid path."},
		{name: "not a project", args: `{"projectPath":` + jsonString(t.TempDir()) + `,"resourcePath":"themes/main.tres","action":"read"}`, want: "Not a valid Godot project"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := callToolArgs(t, manageThemeResource(), tt.args)
			if !result.IsError {
				t.Fatalf("expected an error result: %+v", result.Content)
			}
			if !strings.HasPrefix(result.Content[0].Text, tt.want) {
				t.Errorf("message = %q, want prefix %q", result.Content[0].Text, tt.want)
			}
		})
	}
}

func TestManageThemeResourceScriptFailure(t *testing.T) {
	t.Setenv(godot.PathEnv, fakeGodotOps(t, `echo 'Failed to create theme: boom' >&2; exit 1`))
	root := t.TempDir()
	makeProject(t, root)

	result := callToolArgs(t, manageThemeResource(), `{"projectPath":`+jsonString(root)+`,"resourcePath":"themes/main.tres","action":"create"}`)
	if !result.IsError {
		t.Fatalf("expected an error result: %+v", result.Content)
	}
	if !strings.HasPrefix(result.Content[0].Text, "manage_theme_resource failed: ") {
		t.Errorf("message = %q", result.Content[0].Text)
	}
	if len(result.Content) != 2 || !strings.HasPrefix(result.Content[1].Text, "Possible solutions:") {
		t.Errorf("solutions = %+v", result.Content)
	}
}

func TestManageThemeResourceSpawnFailure(t *testing.T) {
	t.Setenv(godot.PathEnv, filepath.Join(t.TempDir(), "missing-godot"))
	root := t.TempDir()
	makeProject(t, root)

	result := callToolArgs(t, manageThemeResource(), `{"projectPath":`+jsonString(root)+`,"resourcePath":"themes/main.tres","action":"create"}`)
	if !result.IsError {
		t.Fatalf("expected an error result: %+v", result.Content)
	}
	if !strings.HasPrefix(result.Content[0].Text, "manage_theme_resource failed: ") {
		t.Errorf("message = %q", result.Content[0].Text)
	}
}
