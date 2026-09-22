package tools

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/vilppuvuorinen/go-dot-mcp/internal/godot"
)

func TestCreateResourceSchema(t *testing.T) {
	tool := createResource()
	if tool.Name != "create_resource" {
		t.Errorf("name = %q", tool.Name)
	}
	if tool.Description != "Create a .tres resource file (headless)" {
		t.Errorf("description = %q", tool.Description)
	}
	required, ok := tool.InputSchema["required"].([]string)
	if !ok || strings.Join(required, ",") != "projectPath,resourceType,resourcePath" {
		t.Errorf("required = %v", tool.InputSchema["required"])
	}
	props, ok := tool.InputSchema["properties"].(map[string]any)
	if !ok {
		t.Fatalf("properties = %T", tool.InputSchema["properties"])
	}
	for _, key := range []string{"projectPath", "resourceType", "resourcePath"} {
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

func TestCreateResourceSuccess(t *testing.T) {
	t.Setenv(godot.PathEnv, fakeGodotOps(t, `echo "$@"`))
	root := t.TempDir()
	makeProject(t, root)

	result := callToolArgs(t, createResource(), `{"projectPath":`+jsonString(root)+`,"resourceType":"StandardMaterial3D","resourcePath":"materials/red.tres"}`)
	if result.IsError {
		t.Fatalf("unexpected error: %+v", result.Content)
	}
	if len(result.Content) != 1 {
		t.Fatalf("content = %+v", result.Content)
	}
	text := result.Content[0].Text
	if !strings.HasPrefix(text, "create_resource succeeded.") {
		t.Errorf("text = %q", text)
	}
	for _, want := range []string{`"resource_type":"StandardMaterial3D"`, `"resource_path":"materials/red.tres"`} {
		if !strings.Contains(text, want) {
			t.Errorf("text = %q, want it to contain %q", text, want)
		}
	}
	if strings.Contains(text, `"properties"`) {
		t.Errorf("text = %q, want no properties", text)
	}
}

func TestCreateResourceProperties(t *testing.T) {
	t.Setenv(godot.PathEnv, fakeGodotOps(t, `echo "$@"`))
	root := t.TempDir()
	makeProject(t, root)

	result := callToolArgs(t, createResource(), `{"projectPath":`+jsonString(root)+`,"resourceType":"StandardMaterial3D","resourcePath":"materials/red.tres","properties":{"albedo_color":{"r":1,"g":0,"b":0,"a":1}}}`)
	if result.IsError {
		t.Fatalf("unexpected error: %+v", result.Content)
	}
	text := result.Content[0].Text
	if !strings.Contains(text, `"properties":{"albedo_color":{"a":1,"b":0,"g":0,"r":1}}`) {
		t.Errorf("text = %q, want the properties to round-trip", text)
	}
}

func TestCreateResourceAliases(t *testing.T) {
	t.Setenv(godot.PathEnv, fakeGodotOps(t, `echo "$@"`))
	root := t.TempDir()
	makeProject(t, root)

	result := callToolArgs(t, createResource(), `{"project_path":`+jsonString(root)+`,"resource_type":"Theme","resource_path":"themes/dark.tres"}`)
	if result.IsError {
		t.Fatalf("unexpected error: %+v", result.Content)
	}
	if !strings.Contains(result.Content[0].Text, `"resource_type":"Theme"`) {
		t.Errorf("text = %q, want converted snake_case params", result.Content[0].Text)
	}
}

func TestCreateResourceValidation(t *testing.T) {
	root := t.TempDir()
	makeProject(t, root)

	tests := []struct {
		name string
		args string
		want string
	}{
		{name: "missing params", args: `{}`, want: "projectPath, resourceType, and resourcePath are required."},
		{name: "missing resource type", args: `{"projectPath":` + jsonString(root) + `,"resourcePath":"materials/red.tres"}`, want: "projectPath, resourceType, and resourcePath are required."},
		{name: "invalid project path", args: `{"projectPath":"../escape","resourceType":"Theme","resourcePath":"themes/dark.tres"}`, want: "Invalid path."},
		{name: "invalid resource path", args: `{"projectPath":` + jsonString(root) + `,"resourceType":"Theme","resourcePath":"../escape.tres"}`, want: "Invalid path."},
		{name: "not a project", args: `{"projectPath":` + jsonString(t.TempDir()) + `,"resourceType":"Theme","resourcePath":"themes/dark.tres"}`, want: "Not a valid Godot project"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := callToolArgs(t, createResource(), tt.args)
			if !result.IsError {
				t.Fatalf("expected an error result: %+v", result.Content)
			}
			if !strings.HasPrefix(result.Content[0].Text, tt.want) {
				t.Errorf("message = %q, want prefix %q", result.Content[0].Text, tt.want)
			}
		})
	}
}

func TestCreateResourceScriptFailure(t *testing.T) {
	t.Setenv(godot.PathEnv, fakeGodotOps(t, `echo 'Failed to save resource: 31' >&2; exit 1`))
	root := t.TempDir()
	makeProject(t, root)

	result := callToolArgs(t, createResource(), `{"projectPath":`+jsonString(root)+`,"resourceType":"StandardMaterial3D","resourcePath":"materials/red.tres"}`)
	if !result.IsError {
		t.Fatalf("expected an error result: %+v", result.Content)
	}
	if !strings.HasPrefix(result.Content[0].Text, "create_resource failed: ") {
		t.Errorf("message = %q", result.Content[0].Text)
	}
	if len(result.Content) != 2 || !strings.HasPrefix(result.Content[1].Text, "Possible solutions:") {
		t.Errorf("solutions = %+v", result.Content)
	}
}

func TestCreateResourceSpawnFailure(t *testing.T) {
	t.Setenv(godot.PathEnv, filepath.Join(t.TempDir(), "missing-godot"))
	root := t.TempDir()
	makeProject(t, root)

	result := callToolArgs(t, createResource(), `{"projectPath":`+jsonString(root)+`,"resourceType":"StandardMaterial3D","resourcePath":"materials/red.tres"}`)
	if !result.IsError {
		t.Fatalf("expected an error result: %+v", result.Content)
	}
	if !strings.HasPrefix(result.Content[0].Text, "create_resource failed: ") {
		t.Errorf("message = %q", result.Content[0].Text)
	}
}
