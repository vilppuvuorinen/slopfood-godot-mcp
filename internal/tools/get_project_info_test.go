package tools

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vilppuvuorinen/go-dot-mcp/internal/godot"
)

// seedFile writes content to path, creating parent directories as needed.
func seedFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil { //nolint:gosec // test fixture.
		t.Fatalf("write %s: %v", path, err)
	}
}

func TestGetProjectInfoSchema(t *testing.T) {
	tool := getProjectInfo()
	if tool.Name != "get_project_info" {
		t.Errorf("name = %q", tool.Name)
	}
	if tool.Description != "Retrieve metadata about a Godot project" {
		t.Errorf("description = %q", tool.Description)
	}
	if tool.InputSchema["type"] != "object" {
		t.Errorf("schema type = %v", tool.InputSchema["type"])
	}
	required, ok := tool.InputSchema["required"].([]string)
	if !ok || len(required) != 1 || required[0] != "projectPath" {
		t.Errorf("required = %v", tool.InputSchema["required"])
	}
	props, ok := tool.InputSchema["properties"].(map[string]any)
	if !ok {
		t.Fatalf("properties = %T", tool.InputSchema["properties"])
	}
	projectPath, ok := props["projectPath"].(map[string]any)
	if !ok || projectPath["type"] != "string" {
		t.Errorf("projectPath property = %v", props["projectPath"])
	}
}

func TestGetProjectInfoSuccess(t *testing.T) {
	t.Setenv(godot.PathEnv, fakeGodot(t))

	root := t.TempDir()
	makeProject(t, root)
	seedFile(t, filepath.Join(root, "project.godot"), "config_version=5\n\n[application]\n\nconfig/name=\"My Game\"\n")
	seedFile(t, filepath.Join(root, "main.tscn"), "scene")
	seedFile(t, filepath.Join(root, "player.gd"), "script")
	seedFile(t, filepath.Join(root, "enemy.cs"), "script")
	seedFile(t, filepath.Join(root, "icon.png"), "asset")
	seedFile(t, filepath.Join(root, "sound.ogg"), "asset")
	seedFile(t, filepath.Join(root, "notes.txt"), "other")
	seedFile(t, filepath.Join(root, ".hidden", "secret.tscn"), "hidden")
	seedFile(t, filepath.Join(root, "nested", "level.tscn"), "scene")

	result := callToolArgs(t, getProjectInfo(), `{"projectPath":`+jsonString(root)+`}`)
	if result.IsError {
		t.Fatalf("unexpected error: %+v", result.Content)
	}
	if len(result.Content) != 1 {
		t.Fatalf("content = %+v", result.Content)
	}

	var info projectInfo
	if err := json.Unmarshal([]byte(result.Content[0].Text), &info); err != nil {
		t.Fatalf("unmarshal %q: %v", result.Content[0].Text, err)
	}
	if info.Name != "My Game" {
		t.Errorf("name = %q, want My Game", info.Name)
	}
	if info.Path != root {
		t.Errorf("path = %q, want %q", info.Path, root)
	}
	if info.GodotVersion != "4.7.2.stable.official.fake" {
		t.Errorf("godotVersion = %q", info.GodotVersion)
	}
	want := projectStructure{Scenes: 2, Scripts: 2, Assets: 2, Other: 2}
	if info.Structure != want {
		t.Errorf("structure = %+v, want %+v", info.Structure, want)
	}
}

func TestGetProjectInfoNameDecodesEscapes(t *testing.T) {
	t.Setenv(godot.PathEnv, fakeGodot(t))

	root := t.TempDir()
	makeProject(t, root)
	seedFile(t, filepath.Join(root, "project.godot"), "config_version=5\n\n[application]\n\nconfig/name=\"My \\\"Game\\\"\"\n")

	result := callToolArgs(t, getProjectInfo(), `{"projectPath":`+jsonString(root)+`}`)
	if result.IsError {
		t.Fatalf("unexpected error: %+v", result.Content)
	}
	var info projectInfo
	if err := json.Unmarshal([]byte(result.Content[0].Text), &info); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if info.Name != `My "Game"` {
		t.Errorf("name = %q, want %q", info.Name, `My "Game"`)
	}
}

func TestGetProjectInfoSnakeCaseAlias(t *testing.T) {
	t.Setenv(godot.PathEnv, fakeGodot(t))

	root := t.TempDir()
	makeProject(t, root)

	result := callToolArgs(t, getProjectInfo(), `{"project_path":`+jsonString(root)+`}`)
	if result.IsError {
		t.Fatalf("unexpected error: %+v", result.Content)
	}
	var info projectInfo
	if err := json.Unmarshal([]byte(result.Content[0].Text), &info); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if info.Path != root {
		t.Errorf("path = %q, want %q", info.Path, root)
	}
}

func TestGetProjectInfoNameFallsBackToBasename(t *testing.T) {
	t.Setenv(godot.PathEnv, fakeGodot(t))

	root := filepath.Join(t.TempDir(), "fallback")
	makeProject(t, root)

	result := callToolArgs(t, getProjectInfo(), `{"projectPath":`+jsonString(root)+`}`)
	if result.IsError {
		t.Fatalf("unexpected error: %+v", result.Content)
	}
	var info projectInfo
	if err := json.Unmarshal([]byte(result.Content[0].Text), &info); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if info.Name != "fallback" {
		t.Errorf("name = %q, want fallback", info.Name)
	}
}

func TestGetProjectInfoMissingPath(t *testing.T) {
	result := callToolArgs(t, getProjectInfo(), `{}`)
	if !result.IsError {
		t.Fatalf("expected an error result: %+v", result.Content)
	}
	if len(result.Content) != 2 {
		t.Fatalf("content = %+v", result.Content)
	}
	if result.Content[0].Text != "Project path is required" {
		t.Errorf("message = %q", result.Content[0].Text)
	}
	if !strings.HasPrefix(result.Content[1].Text, "Possible solutions:") {
		t.Errorf("solutions = %q", result.Content[1].Text)
	}
}

func TestGetProjectInfoInvalidPath(t *testing.T) {
	result := callToolArgs(t, getProjectInfo(), `{"projectPath":"../escape"}`)
	if !result.IsError {
		t.Fatalf("expected an error result: %+v", result.Content)
	}
	if result.Content[0].Text != "Invalid project path" {
		t.Errorf("message = %q", result.Content[0].Text)
	}
}

func TestGetProjectInfoNotAProject(t *testing.T) {
	root := t.TempDir()
	result := callToolArgs(t, getProjectInfo(), `{"projectPath":`+jsonString(root)+`}`)
	if !result.IsError {
		t.Fatalf("expected an error result: %+v", result.Content)
	}
	want := "Not a valid Godot project: " + root
	if result.Content[0].Text != want {
		t.Errorf("message = %q, want %q", result.Content[0].Text, want)
	}
	if len(result.Content) != 2 {
		t.Fatalf("content = %+v", result.Content)
	}
}

func TestGetProjectInfoGodotFailure(t *testing.T) {
	t.Setenv(godot.PathEnv, filepath.Join(t.TempDir(), "missing-godot"))

	root := t.TempDir()
	makeProject(t, root)

	result := callToolArgs(t, getProjectInfo(), `{"projectPath":`+jsonString(root)+`}`)
	if !result.IsError {
		t.Fatalf("expected an error result: %+v", result.Content)
	}
	if !strings.HasPrefix(result.Content[0].Text, "Failed to get project info:") {
		t.Errorf("message = %q", result.Content[0].Text)
	}
	if len(result.Content) != 2 {
		t.Fatalf("content = %+v", result.Content)
	}
	if !strings.HasPrefix(result.Content[1].Text, "Possible solutions:") {
		t.Errorf("solutions = %q", result.Content[1].Text)
	}
}
