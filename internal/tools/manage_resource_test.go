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
	if tool.Description != "Create, read, or modify .tres/.res resource files" {
		t.Errorf("description = %q", tool.Description)
	}
	required, ok := tool.InputSchema["required"].([]string)
	if !ok || strings.Join(required, ",") != "resourcePath,action" {
		t.Errorf("required = %v", tool.InputSchema["required"])
	}
	props, ok := tool.InputSchema["properties"].(map[string]any)
	if !ok {
		t.Fatalf("properties = %T", tool.InputSchema["properties"])
	}
	for _, key := range []string{"resourcePath", "action", "resourceType"} {
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
	for _, want := range []string{`"resourcePath":"resources/mat.tres"`, `"action":"read"`} {
		if !strings.Contains(text, want) {
			t.Errorf("text = %q, want it to contain %q", text, want)
		}
	}
	if strings.Contains(text, `"properties"`) {
		t.Errorf("text = %q, want no properties for a read action", text)
	}
}

func TestManageResourceReadStructured(t *testing.T) {
	t.Setenv(godot.PathEnv, fakeGodotOps(t, `printf '<<<GO_DOT_MCP_JSON_START>>>\n{"type":"StandardMaterial3D","path":"res://resources/mat.tres"}\n<<<GO_DOT_MCP_JSON_END>>>\n'`))
	root := t.TempDir()
	makeProject(t, root)

	result := callToolArgs(t, manageResource(), `{"projectPath":`+jsonString(root)+`,"resourcePath":"resources/mat.tres","action":"read"}`)
	if result.IsError {
		t.Fatalf("unexpected error: %+v", result.Content)
	}
	text := result.Content[0].Text
	for _, want := range []string{`"type": "StandardMaterial3D"`, `"path": "res://resources/mat.tres"`} {
		if !strings.Contains(text, want) {
			t.Errorf("text = %q, want it to contain %q", text, want)
		}
	}
	if strings.Contains(text, jsonStartMarker) || strings.Contains(text, jsonEndMarker) {
		t.Errorf("text still contains markers: %q", text)
	}
	if strings.Contains(text, "succeeded.\n\nOutput:") {
		t.Errorf("text = %q, want the structured payload without the legacy wrapper", text)
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

func TestManageResourceCreateSuccess(t *testing.T) {
	t.Setenv(godot.PathEnv, fakeGodotOps(t, `echo "$@"`))
	root := t.TempDir()
	makeProject(t, root)

	result := callToolArgs(t, manageResource(), `{"projectPath":`+jsonString(root)+`,"resourcePath":"materials/red.tres","action":"create","resourceType":"StandardMaterial3D"}`)
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
	for _, want := range []string{`"action":"create"`, `"resourceType":"StandardMaterial3D"`, `"resourcePath":"materials/red.tres"`} {
		if !strings.Contains(text, want) {
			t.Errorf("text = %q, want it to contain %q", text, want)
		}
	}
	if strings.Contains(text, `"properties"`) {
		t.Errorf("text = %q, want no properties", text)
	}
}

func TestManageResourceCreateProperties(t *testing.T) {
	t.Setenv(godot.PathEnv, fakeGodotOps(t, `echo "$@"`))
	root := t.TempDir()
	makeProject(t, root)

	result := callToolArgs(t, manageResource(), `{"projectPath":`+jsonString(root)+`,"resourcePath":"materials/red.tres","action":"create","resourceType":"StandardMaterial3D","properties":{"albedo_color":{"r":1,"g":0,"b":0,"a":1}}}`)
	if result.IsError {
		t.Fatalf("unexpected error: %+v", result.Content)
	}
	text := result.Content[0].Text
	if !strings.Contains(text, `"properties":{"albedo_color":{"a":1,"b":0,"g":0,"r":1}}`) {
		t.Errorf("text = %q, want the properties to round-trip", text)
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
		{name: "missing params", args: `{}`, want: "resourcePath and action are required."},
		{name: "missing action", args: `{"projectPath":` + jsonString(root) + `,"resourcePath":"mat.tres"}`, want: "resourcePath and action are required."},
		{name: "missing resource path", args: `{"projectPath":` + jsonString(root) + `,"action":"read"}`, want: "resourcePath and action are required."},
		{name: "create without resource type", args: `{"projectPath":` + jsonString(root) + `,"resourcePath":"mat.tres","action":"create"}`, want: "resourceType is required for the create action."},
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
