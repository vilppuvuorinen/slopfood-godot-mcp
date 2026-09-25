package tools

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/vilppuvuorinen/go-dot-mcp/internal/godot"
)

func TestManageSceneSignalsSchema(t *testing.T) {
	tool := manageSceneSignals()
	if tool.Name != "manage_scene_signals" {
		t.Errorf("name = %q", tool.Name)
	}
	if tool.Description != "List/add/remove signal connections in .tscn files" {
		t.Errorf("description = %q", tool.Description)
	}
	required, ok := tool.InputSchema["required"].([]string)
	if !ok || strings.Join(required, ",") != "projectPath,scenePath,action" {
		t.Errorf("required = %v", tool.InputSchema["required"])
	}
	props, ok := tool.InputSchema["properties"].(map[string]any)
	if !ok {
		t.Fatalf("properties = %T", tool.InputSchema["properties"])
	}
	for _, key := range []string{"projectPath", "scenePath", "action", "signalName", "sourcePath", "targetPath", "method"} {
		prop, ok := props[key].(map[string]any)
		if !ok || prop["type"] != "string" {
			t.Errorf("property %q = %v, want string", key, props[key])
		}
	}
}

func TestManageSceneSignalsSuccess(t *testing.T) {
	t.Setenv(godot.PathEnv, fakeGodotOps(t, `echo "$@"`))
	root := t.TempDir()
	makeProject(t, root)
	seedFile(t, filepath.Join(root, "scenes", "main.tscn"), "[gd_scene]\n")

	tests := []struct {
		name    string
		args    string
		present []string
		omitted []string
	}{
		{
			name:    "list",
			args:    `{"projectPath":` + jsonString(root) + `,"scenePath":"scenes/main.tscn","action":"list"}`,
			present: []string{`"action":"list"`, `"scenePath":"scenes/main.tscn"`},
			omitted: []string{"signalName", "sourcePath", "targetPath", "method"},
		},
		{
			name: "add",
			args: `{"projectPath":` + jsonString(root) + `,"scenePath":"scenes/main.tscn","action":"add","signalName":"pressed","sourcePath":"root","targetPath":"root","method":"on_pressed"}`,
			present: []string{
				`"action":"add"`, `"scenePath":"scenes/main.tscn"`,
				`"signalName":"pressed"`, `"sourcePath":"root"`,
				`"targetPath":"root"`, `"method":"on_pressed"`,
			},
		},
		{
			name: "remove",
			args: `{"projectPath":` + jsonString(root) + `,"scenePath":"scenes/main.tscn","action":"remove","signalName":"released","sourcePath":"root","targetPath":"root/Player"}`,
			present: []string{
				`"action":"remove"`, `"signalName":"released"`,
				`"sourcePath":"root"`, `"targetPath":"root/Player"`,
			},
			omitted: []string{"method"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := callToolArgs(t, manageSceneSignals(), tt.args)
			if result.IsError {
				t.Fatalf("unexpected error: %+v", result.Content)
			}
			text := result.Content[0].Text
			if !strings.HasPrefix(text, "manage_scene_signals succeeded.\n\nOutput: ") {
				t.Errorf("text = %q", text)
			}
			for _, want := range tt.present {
				if !strings.Contains(text, want) {
					t.Errorf("text = %q, want it to contain %q", text, want)
				}
			}
			for _, unwanted := range tt.omitted {
				if strings.Contains(text, unwanted) {
					t.Errorf("text = %q, want it to omit %q", text, unwanted)
				}
			}
		})
	}
}

func TestManageSceneSignalsListStructured(t *testing.T) {
	t.Setenv(godot.PathEnv, fakeGodotOps(t, `printf '<<<GO_DOT_MCP_JSON_START>>>\n{"connections":["[connection signal=\"pressed\" from=\"root\" to=\"root\" method=\"on_pressed\"]"]}\n<<<GO_DOT_MCP_JSON_END>>>\n'`))
	root := t.TempDir()
	makeProject(t, root)
	seedFile(t, filepath.Join(root, "scenes", "main.tscn"), "[gd_scene]\n")

	result := callToolArgs(t, manageSceneSignals(), `{"projectPath":`+jsonString(root)+`,"scenePath":"scenes/main.tscn","action":"list"}`)
	if result.IsError {
		t.Fatalf("unexpected error: %+v", result.Content)
	}
	text := result.Content[0].Text
	for _, want := range []string{`"connections"`, `[connection signal=\"pressed\" from=\"root\" to=\"root\" method=\"on_pressed\"]`} {
		if !strings.Contains(text, want) {
			t.Errorf("text = %q, want it to contain %q", text, want)
		}
	}
	if strings.Contains(text, jsonStartMarker) || strings.Contains(text, jsonEndMarker) {
		t.Errorf("text still contains markers: %q", text)
	}
}

func TestManageSceneSignalsValidation(t *testing.T) {
	root := t.TempDir()
	makeProject(t, root)
	seedFile(t, filepath.Join(root, "scenes", "main.tscn"), "[gd_scene]\n")

	tests := []struct {
		name string
		args string
		want string
	}{
		{name: "missing everything", args: `{}`, want: "projectPath, scenePath, and action are required."},
		{name: "missing scene", args: `{"projectPath":` + jsonString(root) + `,"action":"list"}`, want: "projectPath, scenePath, and action are required."},
		{name: "missing action", args: `{"projectPath":` + jsonString(root) + `,"scenePath":"main.tscn"}`, want: "projectPath, scenePath, and action are required."},
		{name: "invalid project path", args: `{"projectPath":"../escape","scenePath":"main.tscn","action":"list"}`, want: "Invalid path."},
		{name: "invalid scene path", args: `{"projectPath":` + jsonString(root) + `,"scenePath":"../escape.tscn","action":"list"}`, want: "Invalid path."},
		{name: "not a project", args: `{"projectPath":` + jsonString(t.TempDir()) + `,"scenePath":"main.tscn","action":"list"}`, want: "Not a valid Godot project"},
		{name: "missing scene file", args: `{"projectPath":` + jsonString(root) + `,"scenePath":"missing.tscn","action":"list"}`, want: "Scene file does not exist: missing.tscn"},
		{name: "add missing fields", args: `{"projectPath":` + jsonString(root) + `,"scenePath":"main.tscn","action":"add","signalName":"pressed"}`, want: "signalName, sourcePath, targetPath, and method are required for add."},
		{name: "remove missing signal", args: `{"projectPath":` + jsonString(root) + `,"scenePath":"main.tscn","action":"remove"}`, want: "signalName, sourcePath, and targetPath are required for remove."},
		{name: "remove missing source", args: `{"projectPath":` + jsonString(root) + `,"scenePath":"main.tscn","action":"remove","signalName":"released","targetPath":"root"}`, want: "signalName, sourcePath, and targetPath are required for remove."},
		{name: "remove missing target", args: `{"projectPath":` + jsonString(root) + `,"scenePath":"main.tscn","action":"remove","signalName":"released","sourcePath":"root"}`, want: "signalName, sourcePath, and targetPath are required for remove."},
		{name: "unknown action", args: `{"projectPath":` + jsonString(root) + `,"scenePath":"main.tscn","action":"frobnicate"}`, want: "Unknown action: frobnicate. Use list, add, or remove."},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := callToolArgs(t, manageSceneSignals(), tt.args)
			if !result.IsError {
				t.Fatalf("expected an error result: %+v", result.Content)
			}
			if !strings.HasPrefix(result.Content[0].Text, tt.want) {
				t.Errorf("message = %q, want prefix %q", result.Content[0].Text, tt.want)
			}
		})
	}
}

func TestManageSceneSignalsScriptFailure(t *testing.T) {
	t.Setenv(godot.PathEnv, fakeGodotOps(t, `echo 'Failed to connect signal' >&2; exit 1`))
	root := t.TempDir()
	makeProject(t, root)
	seedFile(t, filepath.Join(root, "scenes", "main.tscn"), "[gd_scene]\n")

	result := callToolArgs(t, manageSceneSignals(), `{"projectPath":`+jsonString(root)+`,"scenePath":"scenes/main.tscn","action":"add","signalName":"pressed","sourcePath":"root","targetPath":"root","method":"on_pressed"}`)
	if !result.IsError {
		t.Fatalf("expected an error result: %+v", result.Content)
	}
	if !strings.HasPrefix(result.Content[0].Text, "manage_scene_signals failed: ") {
		t.Errorf("message = %q", result.Content[0].Text)
	}
	if len(result.Content) != 2 || !strings.HasPrefix(result.Content[1].Text, "Possible solutions:") {
		t.Errorf("solutions = %+v", result.Content)
	}
}

func TestManageSceneSignalsSpawnFailure(t *testing.T) {
	t.Setenv(godot.PathEnv, filepath.Join(t.TempDir(), "missing-godot"))
	root := t.TempDir()
	makeProject(t, root)
	seedFile(t, filepath.Join(root, "scenes", "main.tscn"), "[gd_scene]\n")

	result := callToolArgs(t, manageSceneSignals(), `{"projectPath":`+jsonString(root)+`,"scenePath":"scenes/main.tscn","action":"list"}`)
	if !result.IsError {
		t.Fatalf("expected an error result: %+v", result.Content)
	}
	if !strings.HasPrefix(result.Content[0].Text, "manage_scene_signals failed: ") {
		t.Errorf("message = %q", result.Content[0].Text)
	}
}
