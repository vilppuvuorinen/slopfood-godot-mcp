package tools

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/vilppuvuorinen/slopfood-godot-mcp/internal/godot"
)

func TestExportMeshLibrarySchema(t *testing.T) {
	tool := newToolset("", nil, nil).exportMeshLibrary()
	if tool.Name != "export_mesh_library" {
		t.Errorf("name = %q", tool.Name)
	}
	if tool.Description != "Export a scene as a MeshLibrary resource, overwriting the output file if it exists" {
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

	result := callToolArgs(t, (*toolset).exportMeshLibrary, root, `{"scenePath":"scenes/main.tscn","outputPath":"exported/meshlib.res"}`)
	if result.IsError {
		t.Fatalf("unexpected error: %+v", result.Content)
	}
	if len(result.Content) != 1 {
		t.Fatalf("content = %+v", result.Content)
	}
	output := filepath.Join(root, "exported", "meshlib.res")
	text := result.Content[0].Text
	if !strings.HasPrefix(text, "MeshLibrary exported successfully to: "+output) {
		t.Errorf("text = %q", text)
	}
	structured, ok := result.StructuredContent.(map[string]any)
	if !ok {
		t.Fatalf("structuredContent = %T, want object", result.StructuredContent)
	}
	if got := structured["outputPath"]; got != "exported/meshlib.res" {
		t.Errorf("outputPath = %v, want the caller's relative path", got)
	}
	if got := structured["resolvedOutputPath"]; got != output {
		t.Errorf("resolvedOutputPath = %v, want %q", got, output)
	}
	if strings.Contains(text, "meshItemNames") {
		t.Errorf("text = %q, want it not to contain mesh_item_names", text)
	}
}

func TestExportMeshLibraryAbsoluteOutputPath(t *testing.T) {
	t.Setenv(godot.PathEnv, fakeGodotOps(t, `echo "$@"`))
	root := t.TempDir()
	makeProject(t, root)
	seedFile(t, filepath.Join(root, "scenes", "main.tscn"), "scene")

	output := filepath.Join(t.TempDir(), "build", "meshlib.res")
	result := callToolArgs(t, (*toolset).exportMeshLibrary, root,
		`{"scenePath":"scenes/main.tscn","outputPath":`+jsonString(output)+`}`)
	if result.IsError {
		t.Fatalf("unexpected error: %+v", result.Content)
	}
	text := result.Content[0].Text
	if !strings.HasPrefix(text, "MeshLibrary exported successfully to: "+output) {
		t.Errorf("text = %q", text)
	}
	structured, ok := result.StructuredContent.(map[string]any)
	if !ok {
		t.Fatalf("structuredContent = %T, want object", result.StructuredContent)
	}
	if got := structured["outputPath"]; got != output {
		t.Errorf("outputPath = %v, want it to echo the absolute output path", got)
	}
	if got := structured["resolvedOutputPath"]; got != output {
		t.Errorf("resolvedOutputPath = %v, want the absolute output path", got)
	}
}

func TestExportMeshLibraryMeshItemNames(t *testing.T) {
	t.Setenv(godot.PathEnv, fakeGodotOps(t, `echo "$@"`))
	root := t.TempDir()
	makeProject(t, root)
	seedFile(t, filepath.Join(root, "scenes", "main.tscn"), "scene")

	result := callToolArgs(t, (*toolset).exportMeshLibrary, root, `{"scenePath":"scenes/main.tscn","outputPath":"exported/meshlib.res","meshItemNames":["A","B"]}`)
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
	notAProject := t.TempDir()

	tests := []struct {
		name    string
		project string
		args    string
		want    string
	}{
		{name: "missing params", args: `{}`, want: "scenePath and outputPath are required."},
		{name: "invalid path", project: "../escape", args: `{"scenePath":"scenes/main.tscn","outputPath":"exported/meshlib.res"}`, want: "Invalid path"},
		{name: "parent output path", project: root, args: `{"scenePath":"scenes/main.tscn","outputPath":"../escape.res"}`, want: "Path escapes the project root"},
		{name: "not a project", project: notAProject, args: `{"scenePath":"scenes/main.tscn","outputPath":"exported/meshlib.res"}`, want: "Not a valid Godot project"},
		{name: "missing scene", project: root, args: `{"scenePath":"missing.tscn","outputPath":"exported/meshlib.res"}`, want: "Scene file does not exist: "},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := callToolArgs(t, (*toolset).exportMeshLibrary, tt.project, tt.args)
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

	result := callToolArgs(t, (*toolset).exportMeshLibrary, root, `{"scenePath":"scenes/main.tscn","outputPath":"exported/meshlib.res"}`)
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

	result := callToolArgs(t, (*toolset).exportMeshLibrary, root, `{"scenePath":"scenes/main.tscn","outputPath":"exported/meshlib.res"}`)
	if !result.IsError {
		t.Fatalf("expected an error result: %+v", result.Content)
	}
	if !strings.HasPrefix(result.Content[0].Text, "Failed to export mesh library: ") {
		t.Errorf("message = %q", result.Content[0].Text)
	}
}
