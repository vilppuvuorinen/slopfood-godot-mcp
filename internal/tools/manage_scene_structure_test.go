package tools

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/vilppuvuorinen/go-dot-mcp/internal/godot"
)

func TestManageSceneStructureSchema(t *testing.T) {
	tool := manageSceneStructure()
	if tool.Name != "manage_scene_structure" {
		t.Errorf("name = %q", tool.Name)
	}
	if tool.Description != "Rename/duplicate/move nodes within .tscn scenes" {
		t.Errorf("description = %q", tool.Description)
	}
	required, ok := tool.InputSchema["required"].([]string)
	if !ok || strings.Join(required, ",") != "scenePath,action,nodePath" {
		t.Errorf("required = %v", tool.InputSchema["required"])
	}
	props, ok := tool.InputSchema["properties"].(map[string]any)
	if !ok {
		t.Fatalf("properties = %T", tool.InputSchema["properties"])
	}
	for _, key := range []string{"scenePath", "action", "nodePath", "newName", "newParentPath"} {
		prop, ok := props[key].(map[string]any)
		if !ok || prop["type"] != "string" {
			t.Errorf("property %q = %v, want string", key, props[key])
		}
	}
}

func TestManageSceneStructureSuccess(t *testing.T) {
	tests := []struct {
		name    string
		args    string
		want    []string
		notWant []string
	}{
		{
			name:    "rename",
			args:    `{"scenePath":"scenes/main.tscn","action":"rename","nodePath":"root/Player","newName":"Hero"}`,
			want:    []string{`"action":"rename"`, `"nodePath":"root/Player"`, `"newName":"Hero"`},
			notWant: []string{`"newParentPath"`},
		},
		{
			name:    "duplicate",
			args:    `{"scenePath":"scenes/main.tscn","action":"duplicate","nodePath":"root/Player"}`,
			want:    []string{`"action":"duplicate"`, `"nodePath":"root/Player"`},
			notWant: []string{`"newName"`, `"newParentPath"`},
		},
		{
			name:    "move",
			args:    `{"scenePath":"scenes/main.tscn","action":"move","nodePath":"root/Player","newParentPath":"root/World"}`,
			want:    []string{`"action":"move"`, `"nodePath":"root/Player"`, `"newParentPath":"root/World"`},
			notWant: []string{`"newName"`},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv(godot.PathEnv, fakeGodotOps(t, `echo "$@"`))
			root := t.TempDir()
			makeProject(t, root)
			seedFile(t, filepath.Join(root, "scenes", "main.tscn"), "[gd_scene]\n")

			result := callToolArgs(t, manageSceneStructure(), root, tt.args)
			if result.IsError {
				t.Fatalf("unexpected error: %+v", result.Content)
			}
			text := result.Content[0].Text
			for _, want := range tt.want {
				if !strings.Contains(text, want) {
					t.Errorf("text = %q, want it to contain %q", text, want)
				}
			}
			for _, notWant := range tt.notWant {
				if strings.Contains(text, notWant) {
					t.Errorf("text = %q, want it to omit %q", text, notWant)
				}
			}
		})
	}
}

func TestManageSceneStructureValidation(t *testing.T) {
	root := t.TempDir()
	makeProject(t, root)
	notAProject := t.TempDir()

	tests := []struct {
		name    string
		project string
		args    string
		want    string
	}{
		{name: "missing params", args: `{}`, want: "scenePath, action, and nodePath are required."},
		{name: "missing action", project: root, args: `{"scenePath":"main.tscn","nodePath":"root"}`, want: "scenePath, action, and nodePath are required."},
		{name: "missing node path", project: root, args: `{"scenePath":"main.tscn","action":"rename"}`, want: "scenePath, action, and nodePath are required."},
		{name: "invalid project path", project: "../escape", args: `{"scenePath":"main.tscn","action":"rename","nodePath":"root"}`, want: "Invalid path."},
		{name: "invalid scene path", project: root, args: `{"scenePath":"../escape.tscn","action":"rename","nodePath":"root"}`, want: "Invalid path."},
		{name: "not a project", project: notAProject, args: `{"scenePath":"main.tscn","action":"rename","nodePath":"root"}`, want: "Not a valid Godot project"},
		{name: "missing scene file", project: root, args: `{"scenePath":"missing.tscn","action":"rename","nodePath":"root"}`, want: "Scene file does not exist: missing.tscn"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := callToolArgs(t, manageSceneStructure(), tt.project, tt.args)
			if !result.IsError {
				t.Fatalf("expected an error result: %+v", result.Content)
			}
			if !strings.HasPrefix(result.Content[0].Text, tt.want) {
				t.Errorf("message = %q, want prefix %q", result.Content[0].Text, tt.want)
			}
		})
	}
}

func TestManageSceneStructureScriptFailure(t *testing.T) {
	t.Setenv(godot.PathEnv, fakeGodotOps(t, `echo 'Failed to load scene: boom' >&2; exit 1`))
	root := t.TempDir()
	makeProject(t, root)
	seedFile(t, filepath.Join(root, "main.tscn"), "[gd_scene]\n")

	result := callToolArgs(t, manageSceneStructure(), root, `{"scenePath":"main.tscn","action":"rename","nodePath":"root"}`)
	if !result.IsError {
		t.Fatalf("expected an error result: %+v", result.Content)
	}
	if !strings.HasPrefix(result.Content[0].Text, "manage_scene_structure failed: ") {
		t.Errorf("message = %q", result.Content[0].Text)
	}
	if len(result.Content) != 2 || !strings.HasPrefix(result.Content[1].Text, "Possible solutions:") {
		t.Errorf("solutions = %+v", result.Content)
	}
}

func TestManageSceneStructureSpawnFailure(t *testing.T) {
	t.Setenv(godot.PathEnv, filepath.Join(t.TempDir(), "missing-godot"))
	root := t.TempDir()
	makeProject(t, root)
	seedFile(t, filepath.Join(root, "main.tscn"), "[gd_scene]\n")

	result := callToolArgs(t, manageSceneStructure(), root, `{"scenePath":"main.tscn","action":"rename","nodePath":"root"}`)
	if !result.IsError {
		t.Fatalf("expected an error result: %+v", result.Content)
	}
	if !strings.HasPrefix(result.Content[0].Text, "manage_scene_structure failed: ") {
		t.Errorf("message = %q", result.Content[0].Text)
	}
}
