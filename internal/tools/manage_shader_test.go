package tools

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestManageShaderSchema(t *testing.T) {
	tool := manageShader()
	if tool.Name != "manage_shader" {
		t.Errorf("name = %q", tool.Name)
	}
	if tool.Description != "Create or read .gdshader files" {
		t.Errorf("description = %q", tool.Description)
	}
	required, ok := tool.InputSchema["required"].([]string)
	if !ok || strings.Join(required, ",") != "projectPath,shaderPath,action" {
		t.Errorf("required = %v", tool.InputSchema["required"])
	}
	props, ok := tool.InputSchema["properties"].(map[string]any)
	if !ok {
		t.Fatalf("properties = %T", tool.InputSchema["properties"])
	}
	if len(props) != 5 {
		t.Errorf("properties = %v", props)
	}
	for _, key := range []string{"projectPath", "shaderPath", "action", "shaderType", "source"} {
		prop, ok := props[key].(map[string]any)
		if !ok || prop["type"] != "string" {
			t.Errorf("%s property = %v", key, props[key])
		}
	}
}

func TestManageShaderCreateDefault(t *testing.T) {
	root := t.TempDir()
	makeProject(t, root)

	result := callToolArgs(t, manageShader(),
		`{"projectPath":`+jsonString(root)+`,"shaderPath":"shaders/effect.gdshader","action":"create"}`)
	if result.IsError {
		t.Fatalf("unexpected error: %+v", result.Content)
	}
	if got := result.Content[0].Text; got != "Shader created at shaders/effect.gdshader" {
		t.Errorf("message = %q", got)
	}
	data, err := os.ReadFile(filepath.Join(root, "shaders", "effect.gdshader"))
	if err != nil {
		t.Fatalf("read shader: %v", err)
	}
	want := "shader_type spatial;\n\nvoid fragment() {\n\t// Called for every pixel the material is visible on.\n}\n"
	if got := string(data); got != want {
		t.Errorf("content = %q, want %q", got, want)
	}
}

func TestManageShaderCreateExplicitType(t *testing.T) {
	root := t.TempDir()
	makeProject(t, root)

	result := callToolArgs(t, manageShader(),
		`{"projectPath":`+jsonString(root)+`,"shaderPath":"ui.gdshader","action":"create","shaderType":"canvas_item"}`)
	if result.IsError {
		t.Fatalf("unexpected error: %+v", result.Content)
	}
	data, err := os.ReadFile(filepath.Join(root, "ui.gdshader"))
	if err != nil {
		t.Fatalf("read shader: %v", err)
	}
	want := "shader_type canvas_item;\n\nvoid fragment() {\n\t// Called for every pixel the material is visible on.\n}\n"
	if got := string(data); got != want {
		t.Errorf("content = %q, want %q", got, want)
	}
}

func TestManageShaderCreateCustomSource(t *testing.T) {
	root := t.TempDir()
	makeProject(t, root)

	source := "shader_type spatial;\n\nvoid vertex() {\n\tVERTEX.y += 1.0;\n}\n"
	result := callToolArgs(t, manageShader(),
		`{"projectPath":`+jsonString(root)+`,"shaderPath":"custom.gdshader","action":"create","source":`+jsonString(source)+`}`)
	if result.IsError {
		t.Fatalf("unexpected error: %+v", result.Content)
	}
	data, err := os.ReadFile(filepath.Join(root, "custom.gdshader"))
	if err != nil {
		t.Fatalf("read shader: %v", err)
	}
	if got := string(data); got != source {
		t.Errorf("content = %q, want %q", got, source)
	}
}

func TestManageShaderRead(t *testing.T) {
	root := t.TempDir()
	makeProject(t, root)
	source := "shader_type canvas_item;\n"
	seedFile(t, filepath.Join(root, "read.gdshader"), source)

	result := callToolArgs(t, manageShader(),
		`{"projectPath":`+jsonString(root)+`,"shaderPath":"read.gdshader","action":"read"}`)
	if result.IsError {
		t.Fatalf("unexpected error: %+v", result.Content)
	}
	if got := result.Content[0].Text; got != source {
		t.Errorf("text = %q, want %q", got, source)
	}
}

func TestManageShaderReadMissing(t *testing.T) {
	root := t.TempDir()
	makeProject(t, root)

	result := callToolArgs(t, manageShader(),
		`{"projectPath":`+jsonString(root)+`,"shaderPath":"missing.gdshader","action":"read"}`)
	if !result.IsError {
		t.Fatalf("expected an error result: %+v", result.Content)
	}
	if got := result.Content[0].Text; got != "Shader not found: missing.gdshader" {
		t.Errorf("message = %q", got)
	}
}

func TestManageShaderSnakeCaseAliases(t *testing.T) {
	root := t.TempDir()
	makeProject(t, root)

	result := callToolArgs(t, manageShader(),
		`{"project_path":`+jsonString(root)+`,"shader_path":"aliased.gdshader","action":"create","shader_type":"sky"}`)
	if result.IsError {
		t.Fatalf("unexpected error: %+v", result.Content)
	}
	data, err := os.ReadFile(filepath.Join(root, "aliased.gdshader"))
	if err != nil {
		t.Fatalf("read shader: %v", err)
	}
	if !strings.HasPrefix(string(data), "shader_type sky;") {
		t.Errorf("content = %q", data)
	}
}

func TestManageShaderErrors(t *testing.T) {
	root := t.TempDir()
	makeProject(t, root)

	tests := []struct {
		name string
		args string
		want string
	}{
		{name: "missing params", args: `{}`, want: "projectPath, shaderPath, and action are required."},
		{name: "missing action", args: `{"projectPath":` + jsonString(root) + `,"shaderPath":"a.gdshader"}`, want: "projectPath, shaderPath, and action are required."},
		{name: "invalid project path", args: `{"projectPath":"../escape","shaderPath":"a.gdshader","action":"read"}`, want: "Invalid path."},
		{name: "invalid shader path", args: `{"projectPath":` + jsonString(root) + `,"shaderPath":"../escape.gdshader","action":"read"}`, want: "Invalid path."},
		{name: "not a project", args: `{"projectPath":` + jsonString(t.TempDir()) + `,"shaderPath":"a.gdshader","action":"read"}`, want: "Not a valid Godot project: "},
		{name: "unknown action", args: `{"projectPath":` + jsonString(root) + `,"shaderPath":"a.gdshader","action":"bogus"}`, want: "Unknown action: bogus"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := callToolArgs(t, manageShader(), tt.args)
			if !result.IsError {
				t.Fatalf("expected an error result: %+v", result.Content)
			}
			if !strings.HasPrefix(result.Content[0].Text, tt.want) {
				t.Errorf("message = %q, want prefix %q", result.Content[0].Text, tt.want)
			}
		})
	}
}
