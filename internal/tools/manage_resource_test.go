package tools

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/vilppuvuorinen/go-dot-mcp/internal/godot"
)

func TestManageResourceSchema(t *testing.T) {
	tool := manageResource()
	if tool.Name != "manage_resource" {
		t.Errorf("name = %q", tool.Name)
	}
	if tool.Description != "Read or modify .tres/.res resource files" {
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

func TestManageResourceReadSuccess(t *testing.T) {
	t.Setenv(godot.PathEnv, fakeGodotOps(t, `echo "$@"`))
	root := t.TempDir()
	makeProject(t, root)

	result := callToolArgs(t, manageResource(), `{"projectPath":`+jsonString(root)+`,"resourcePath":"resources/mat.tres","action":"read"}`)
	if result.IsError {
		t.Fatalf("unexpected error: %+v", result.Content)
	}
	if len(result.Content) != 1 {
		t.Fatalf("content = %+v", result.Content)
	}
	text := result.Content[0].Text
	if !strings.HasPrefix(text, "manage_resource succeeded.") {
		t.Errorf("text = %q", text)
	}
	for _, want := range []string{`"resource_path":"resources/mat.tres"`, `"action":"read"`} {
		if !strings.Contains(text, want) {
			t.Errorf("text = %q, want it to contain %q", text, want)
		}
	}
	if strings.Contains(text, `"properties"`) {
		t.Errorf("text = %q, want no properties for a read action", text)
	}
}

func TestManageResourceModifySuccess(t *testing.T) {
	t.Setenv(godot.PathEnv, fakeGodotOps(t, `echo "$@"`))
	root := t.TempDir()
	makeProject(t, root)

	result := callToolArgs(t, manageResource(), `{"projectPath":`+jsonString(root)+`,"resourcePath":"resources/mat.tres","action":"modify","properties":{"metallic":0.9}}`)
	if result.IsError {
		t.Fatalf("unexpected error: %+v", result.Content)
	}
	text := result.Content[0].Text
	for _, want := range []string{`"action":"modify"`, `"properties":{"metallic":0.9}`} {
		if !strings.Contains(text, want) {
			t.Errorf("text = %q, want it to contain %q", text, want)
		}
	}
}

func TestManageResourceAliases(t *testing.T) {
	t.Setenv(godot.PathEnv, fakeGodotOps(t, `echo "$@"`))
	root := t.TempDir()
	makeProject(t, root)

	result := callToolArgs(t, manageResource(), `{"project_path":`+jsonString(root)+`,"resource_path":"mat.tres","action":"read"}`)
	if result.IsError {
		t.Fatalf("unexpected error: %+v", result.Content)
	}
	if !strings.Contains(result.Content[0].Text, `"resource_path":"mat.tres"`) {
		t.Errorf("text = %q, want converted snake_case params", result.Content[0].Text)
	}
}

func TestManageResourceValidation(t *testing.T) {
	root := t.TempDir()
	makeProject(t, root)

	tests := []struct {
		name string
		args string
		want string
	}{
		{name: "missing params", args: `{}`, want: "projectPath, resourcePath, and action are required."},
		{name: "missing action", args: `{"projectPath":` + jsonString(root) + `,"resourcePath":"mat.tres"}`, want: "projectPath, resourcePath, and action are required."},
		{name: "missing resource path", args: `{"projectPath":` + jsonString(root) + `,"action":"read"}`, want: "projectPath, resourcePath, and action are required."},
		{name: "invalid project path", args: `{"projectPath":"../escape","resourcePath":"mat.tres","action":"read"}`, want: "Invalid path."},
		{name: "invalid resource path", args: `{"projectPath":` + jsonString(root) + `,"resourcePath":"../escape.tres","action":"read"}`, want: "Invalid path."},
		{name: "not a project", args: `{"projectPath":` + jsonString(t.TempDir()) + `,"resourcePath":"mat.tres","action":"read"}`, want: "Not a valid Godot project"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := callToolArgs(t, manageResource(), tt.args)
			if !result.IsError {
				t.Fatalf("expected an error result: %+v", result.Content)
			}
			if !strings.HasPrefix(result.Content[0].Text, tt.want) {
				t.Errorf("message = %q, want prefix %q", result.Content[0].Text, tt.want)
			}
		})
	}
}

func TestManageResourceScriptFailure(t *testing.T) {
	t.Setenv(godot.PathEnv, fakeGodotOps(t, `echo 'Failed to load resource: boom' >&2; exit 1`))
	root := t.TempDir()
	makeProject(t, root)

	result := callToolArgs(t, manageResource(), `{"projectPath":`+jsonString(root)+`,"resourcePath":"mat.tres","action":"read"}`)
	if !result.IsError {
		t.Fatalf("expected an error result: %+v", result.Content)
	}
	if !strings.HasPrefix(result.Content[0].Text, "manage_resource failed: ") {
		t.Errorf("message = %q", result.Content[0].Text)
	}
	if len(result.Content) != 2 || !strings.HasPrefix(result.Content[1].Text, "Possible solutions:") {
		t.Errorf("solutions = %+v", result.Content)
	}
}

func TestManageResourceSpawnFailure(t *testing.T) {
	t.Setenv(godot.PathEnv, filepath.Join(t.TempDir(), "missing-godot"))
	root := t.TempDir()
	makeProject(t, root)

	result := callToolArgs(t, manageResource(), `{"projectPath":`+jsonString(root)+`,"resourcePath":"mat.tres","action":"read"}`)
	if !result.IsError {
		t.Fatalf("expected an error result: %+v", result.Content)
	}
	if !strings.HasPrefix(result.Content[0].Text, "manage_resource failed: ") {
		t.Errorf("message = %q", result.Content[0].Text)
	}
}
