package tools

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/vilppuvuorinen/go-dot-mcp/internal/godot"
)

func TestExportMeshLibrarySchema(t *testing.T) {
	tool := exportMeshLibrary()
	if tool.Name != "export_mesh_library" {
		t.Errorf("name = %q", tool.Name)
	}
	if tool.Description != "Export a scene as a MeshLibrary resource" {
		t.Errorf("description = %q", tool.Description)
	}
	required, ok := tool.InputSchema["required"].([]string)
	if !ok || strings.Join(required, ",") != "scenePath,outputPath" {
		t.Errorf("required = %v", tool.InputSchema["required"])
	}
	props, ok := tool.InputSchema["properties"].(map[string]any)
	if !ok {
		t.Fatalf("properties = %T", tool.InputSchema["properties"])
	}
	mesh, ok := props["meshItemNames"].(map[string]any)
	if !ok || mesh["type"] != "array" {
		t.Fatalf("meshItemNames property = %v", props["meshItemNames"])
	}
	items, ok := mesh["items"].(map[string]any)
	if !ok || items["type"] != "string" {
		t.Errorf("meshItemNames items = %v", mesh["items"])
	}
}

func TestExportMeshLibrarySuccess(t *testing.T) {
	t.Setenv(godot.PathEnv, fakeGodotOps(t, `echo "$@"`))
	root := t.TempDir()
	makeProject(t, root)
	seedFile(t, filepath.Join(root, "scenes", "main.tscn"), "scene")

	result := callToolArgs(t, exportMeshLibrary(), `{"projectPath":`+jsonString(root)+`,"scenePath":"scenes/main.tscn","outputPath":"exported/meshlib.res"}`)
	if result.IsError {
		t.Fatalf("unexpected error: %+v", result.Content)
	}
	if len(result.Content) != 1 {
		t.Fatalf("content = %+v", result.Content)
	}
	text := result.Content[0].Text
	if !strings.HasPrefix(text, "MeshLibrary exported successfully to: exported/meshlib.res") {
		t.Errorf("text = %q", text)
	}
	for _, want := range []string{`"scenePath":"scenes/main.tscn"`, `"outputPath":"exported/meshlib.res"`} {
		if !strings.Contains(text, want) {
			t.Errorf("text = %q, want it to contain %q", text, want)
		}
	}
	if strings.Contains(text, "meshItemNames") {
		t.Errorf("text = %q, want it not to contain mesh_item_names", text)
	}
}

func TestExportMeshLibraryMeshItemNames(t *testing.T) {
	t.Setenv(godot.PathEnv, fakeGodotOps(t, `echo "$@"`))
	root := t.TempDir()
	makeProject(t, root)
	seedFile(t, filepath.Join(root, "scenes", "main.tscn"), "scene")

	result := callToolArgs(t, exportMeshLibrary(), `{"projectPath":`+jsonString(root)+`,"scenePath":"scenes/main.tscn","outputPath":"exported/meshlib.res","meshItemNames":["A","B"]}`)
	if result.IsError {
		t.Fatalf("unexpected error: %+v", result.Content)
	}
	if !strings.Contains(result.Content[0].Text, `"meshItemNames":["A","B"]`) {
		t.Errorf("text = %q, want it to contain the forwarded mesh item names", result.Content[0].Text)
	}
}

func TestExportMeshLibraryValidation(t *testing.T) {
	root := t.TempDir()
	makeProject(t, root)
	seedFile(t, filepath.Join(root, "scenes", "main.tscn"), "scene")

	tests := []struct {
		name string
		args string
		want string
	}{
		{name: "missing params", args: `{}`, want: "Missing required parameters"},
		{name: "invalid path", args: `{"projectPath":"../escape","scenePath":"scenes/main.tscn","outputPath":"exported/meshlib.res"}`, want: "Invalid path"},
		{name: "not a project", args: `{"projectPath":` + jsonString(t.TempDir()) + `,"scenePath":"scenes/main.tscn","outputPath":"exported/meshlib.res"}`, want: "Not a valid Godot project"},
		{name: "missing scene", args: `{"projectPath":` + jsonString(root) + `,"scenePath":"missing.tscn","outputPath":"exported/meshlib.res"}`, want: "Scene file does not exist: "},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := callToolArgs(t, exportMeshLibrary(), tt.args)
			if !result.IsError {
				t.Fatalf("expected an error result: %+v", result.Content)
			}
			if !strings.HasPrefix(result.Content[0].Text, tt.want) {
				t.Errorf("message = %q, want prefix %q", result.Content[0].Text, tt.want)
			}
		})
	}
}

func TestExportMeshLibraryScriptFailure(t *testing.T) {
	t.Setenv(godot.PathEnv, fakeGodotOps(t, `echo 'Failed to export mesh library: boom' >&2; exit 1`))
	root := t.TempDir()
	makeProject(t, root)
	seedFile(t, filepath.Join(root, "scenes", "main.tscn"), "scene")

	result := callToolArgs(t, exportMeshLibrary(), `{"projectPath":`+jsonString(root)+`,"scenePath":"scenes/main.tscn","outputPath":"exported/meshlib.res"}`)
	if !result.IsError {
		t.Fatalf("expected an error result: %+v", result.Content)
	}
	if len(result.Content) != 2 {
		t.Fatalf("content = %+v", result.Content)
	}
	if !strings.HasPrefix(result.Content[0].Text, "Failed to export mesh library: ") {
		t.Errorf("message = %q", result.Content[0].Text)
	}
	if !strings.HasPrefix(result.Content[1].Text, "Possible solutions:") {
		t.Errorf("solutions = %q", result.Content[1].Text)
	}
}

func TestExportMeshLibrarySpawnFailure(t *testing.T) {
	t.Setenv(godot.PathEnv, filepath.Join(t.TempDir(), "missing-godot"))
	root := t.TempDir()
	makeProject(t, root)
	seedFile(t, filepath.Join(root, "scenes", "main.tscn"), "scene")

	result := callToolArgs(t, exportMeshLibrary(), `{"projectPath":`+jsonString(root)+`,"scenePath":"scenes/main.tscn","outputPath":"exported/meshlib.res"}`)
	if !result.IsError {
		t.Fatalf("expected an error result: %+v", result.Content)
	}
	if !strings.HasPrefix(result.Content[0].Text, "Failed to export mesh library: ") {
		t.Errorf("message = %q", result.Content[0].Text)
	}
}
