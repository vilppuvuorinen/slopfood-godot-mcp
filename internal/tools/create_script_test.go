package tools

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCreateScriptSchema(t *testing.T) {
	tool := createScript()
	if tool.Name != "create_script" {
		t.Errorf("name = %q", tool.Name)
	}
	if tool.Description != "Create a GDScript file from a template" {
		t.Errorf("description = %q", tool.Description)
	}
	required, ok := tool.InputSchema["required"].([]string)
	if !ok || strings.Join(required, ",") != "projectPath,scriptPath" {
		t.Errorf("required = %v", tool.InputSchema["required"])
	}
	props, ok := tool.InputSchema["properties"].(map[string]any)
	if !ok {
		t.Fatalf("properties = %T", tool.InputSchema["properties"])
	}
	if len(props) != 6 {
		t.Errorf("properties = %v", props)
	}
	for _, key := range []string{"projectPath", "scriptPath", "extends", "className", "methods", "source"} {
		if _, ok := props[key]; !ok {
			t.Errorf("missing property %q", key)
		}
	}
	for _, key := range []string{"projectPath", "scriptPath", "extends", "className", "source"} {
		if prop, ok := props[key].(map[string]any); !ok || prop["type"] != "string" {
			t.Errorf("%s property = %v", key, props[key])
		}
	}
	if methods, ok := props["methods"].(map[string]any); !ok || methods["type"] != "array" {
		t.Errorf("methods property = %v", props["methods"])
	}
}

func TestCreateScriptDefaultTemplate(t *testing.T) {
	root := t.TempDir()
	makeProject(t, root)

	result := callToolArgs(t, createScript(),
		`{"projectPath":`+jsonString(root)+`,"scriptPath":"player.gd"}`)
	if result.IsError {
		t.Fatalf("unexpected error: %+v", result.Content)
	}
	if got := result.Content[0].Text; got != "Script created at player.gd" {
		t.Errorf("message = %q", got)
	}
	data, err := os.ReadFile(filepath.Join(root, "player.gd"))
	if err != nil {
		t.Fatalf("read script: %v", err)
	}
	if got, want := string(data), "extends Node\n\n"; got != want {
		t.Errorf("content = %q, want %q", got, want)
	}
}

func TestCreateScriptClassNameAndMethods(t *testing.T) {
	root := t.TempDir()
	makeProject(t, root)

	result := callToolArgs(t, createScript(),
		`{"projectPath":`+jsonString(root)+`,"scriptPath":"scripts/player.gd","extends":"Node2D","className":"Player","methods":["_ready","_process"]}`)
	if result.IsError {
		t.Fatalf("unexpected error: %+v", result.Content)
	}
	data, err := os.ReadFile(filepath.Join(root, "scripts", "player.gd"))
	if err != nil {
		t.Fatalf("read script: %v", err)
	}
	want := "extends Node2D\nclass_name Player\n\n\nfunc _ready():\n\tpass\n\nfunc _process():\n\tpass\n"
	if got := string(data); got != want {
		t.Errorf("content = %q, want %q", got, want)
	}
}

func TestCreateScriptQuotedExtendsPath(t *testing.T) {
	root := t.TempDir()
	makeProject(t, root)

	result := callToolArgs(t, createScript(),
		`{"projectPath":`+jsonString(root)+`,"scriptPath":"child.gd","extends":`+jsonString(`"res://base.gd"`)+`}`)
	if result.IsError {
		t.Fatalf("unexpected error: %+v", result.Content)
	}
	data, err := os.ReadFile(filepath.Join(root, "child.gd"))
	if err != nil {
		t.Fatalf("read script: %v", err)
	}
	if got, want := string(data), "extends \"res://base.gd\"\n\n"; got != want {
		t.Errorf("content = %q, want %q", got, want)
	}
}

func TestCreateScriptValidatesClassNameWithSource(t *testing.T) {
	root := t.TempDir()
	makeProject(t, root)

	result := callToolArgs(t, createScript(),
		`{"projectPath":`+jsonString(root)+`,"scriptPath":"a.gd","className":"2Bad","source":"extends Node\n"}`)
	if !result.IsError {
		t.Fatalf("expected an error result: %+v", result.Content)
	}
	if !strings.HasPrefix(result.Content[0].Text, "Invalid className") {
		t.Errorf("message = %q", result.Content[0].Text)
	}
}

func TestCreateScriptSourceOverride(t *testing.T) {
	root := t.TempDir()
	makeProject(t, root)

	source := "extends Area2D\n\nfunc _ready() -> void:\n\tprint(\"hi\")\n"
	result := callToolArgs(t, createScript(),
		`{"projectPath":`+jsonString(root)+`,"scriptPath":"override.gd","source":`+jsonString(source)+`}`)
	if result.IsError {
		t.Fatalf("unexpected error: %+v", result.Content)
	}
	data, err := os.ReadFile(filepath.Join(root, "override.gd"))
	if err != nil {
		t.Fatalf("read script: %v", err)
	}
	if got := string(data); got != source {
		t.Errorf("content = %q, want %q", got, source)
	}
}

func TestCreateScriptSnakeCaseAliases(t *testing.T) {
	root := t.TempDir()
	makeProject(t, root)

	result := callToolArgs(t, createScript(),
		`{"project_path":`+jsonString(root)+`,"script_path":"aliased.gd","class_name":"Aliased"}`)
	if result.IsError {
		t.Fatalf("unexpected error: %+v", result.Content)
	}
	if got := result.Content[0].Text; got != "Script created at aliased.gd" {
		t.Errorf("message = %q", got)
	}
	data, err := os.ReadFile(filepath.Join(root, "aliased.gd"))
	if err != nil {
		t.Fatalf("read script: %v", err)
	}
	if got, want := string(data), "extends Node\nclass_name Aliased\n\n"; got != want {
		t.Errorf("content = %q, want %q", got, want)
	}
}

func TestCreateScriptErrors(t *testing.T) {
	root := t.TempDir()
	makeProject(t, root)

	tests := []struct {
		name string
		args string
		want string
	}{
		{name: "missing params", args: `{}`, want: "projectPath and scriptPath are required."},
		{name: "missing script path", args: `{"projectPath":` + jsonString(root) + `}`, want: "projectPath and scriptPath are required."},
		{name: "invalid project path", args: `{"projectPath":"../escape","scriptPath":"a.gd"}`, want: "Invalid path."},
		{name: "invalid script path", args: `{"projectPath":` + jsonString(root) + `,"scriptPath":"../escape.gd"}`, want: "Invalid path."},
		{name: "invalid class name", args: `{"projectPath":` + jsonString(root) + `,"scriptPath":"a.gd","className":"2Bad"}`, want: "Invalid className"},
		{name: "injecting class name", args: `{"projectPath":` + jsonString(root) + `,"scriptPath":"a.gd","className":` + jsonString("Player\nfunc evil():") + `}`, want: "Invalid className"},
		{name: "invalid extends", args: `{"projectPath":` + jsonString(root) + `,"scriptPath":"a.gd","extends":"res://base.gd"}`, want: "Invalid extends"},
		{name: "injecting extends", args: `{"projectPath":` + jsonString(root) + `,"scriptPath":"a.gd","extends":` + jsonString("Node2D\nfunc evil():") + `}`, want: "Invalid extends"},
		{name: "not a project", args: `{"projectPath":` + jsonString(t.TempDir()) + `,"scriptPath":"a.gd"}`, want: "Not a valid Godot project: "},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := callToolArgs(t, createScript(), tt.args)
			if !result.IsError {
				t.Fatalf("expected an error result: %+v", result.Content)
			}
			if !strings.HasPrefix(result.Content[0].Text, tt.want) {
				t.Errorf("message = %q, want prefix %q", result.Content[0].Text, tt.want)
			}
		})
	}
}
