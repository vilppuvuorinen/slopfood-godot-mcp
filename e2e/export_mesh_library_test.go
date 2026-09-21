package e2e

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestExportMeshLibrarySchema ports the reference's tool-definition
// expectations for export_mesh_library over the real stdio surface.
func TestExportMeshLibrarySchema(t *testing.T) {
	tool := findTool(t, connect(t), "export_mesh_library")
	if tool.Description != "Export a scene as a MeshLibrary resource" {
		t.Errorf("description = %q", tool.Description)
	}

	data, err := json.Marshal(tool.InputSchema)
	if err != nil {
		t.Fatalf("marshal schema: %v", err)
	}
	var schema struct {
		Type       string                    `json:"type"`
		Required   []string                  `json:"required"`
		Properties map[string]map[string]any `json:"properties"`
	}
	if err := json.Unmarshal(data, &schema); err != nil {
		t.Fatalf("unmarshal schema %s: %v", data, err)
	}
	if schema.Type != "object" {
		t.Errorf("schema type = %q", schema.Type)
	}
	if strings.Join(schema.Required, ",") != "projectPath,scenePath,outputPath" {
		t.Errorf("required = %v", schema.Required)
	}
	if schema.Properties["meshItemNames"]["type"] != "array" {
		t.Errorf("meshItemNames type = %v", schema.Properties["meshItemNames"]["type"])
	}
	items, ok := schema.Properties["meshItemNames"]["items"].(map[string]any)
	if !ok || items["type"] != "string" {
		t.Errorf("meshItemNames items = %v", schema.Properties["meshItemNames"]["items"])
	}
}

// TestExportMeshLibraryTool drives the real export_mesh_library tool against
// Godot, including the optional meshItemNames filter.
func TestExportMeshLibraryTool(t *testing.T) {
	session := connect(t)
	requireGodot(t, session)

	t.Run("exports a mesh library", func(t *testing.T) {
		root := t.TempDir()
		seedProject(t, root)

		res := callTool(t, session, "create_scene", map[string]any{
			"projectPath":  root,
			"scenePath":    "scenes/main.tscn",
			"rootNodeType": "Node3D",
		})
		if res.IsError {
			t.Skipf("create_scene failed, skipping: %s", firstText(t, res))
		}

		writeE2EFile(t, filepath.Join(root, "box.tres"), "[gd_resource type=\"BoxMesh\" format=3]\n\n[resource]\nsize = Vector3(1, 1, 1)\n")

		res = callTool(t, session, "add_node", map[string]any{
			"projectPath": root,
			"scenePath":   "scenes/main.tscn",
			"nodeType":    "MeshInstance3D",
			"nodeName":    "Mesh",
			"properties":  map[string]any{"mesh": "res://box.tres"},
		})
		if res.IsError {
			t.Skipf("add_node failed, skipping: %s", firstText(t, res))
		}

		res = callTool(t, session, "export_mesh_library", map[string]any{
			"projectPath": root,
			"scenePath":   "scenes/main.tscn",
			"outputPath":  "exported/meshlib.res",
		})
		if res.IsError {
			t.Fatalf("unexpected error: %s", firstText(t, res))
		}
		if got := firstText(t, res); !strings.HasPrefix(got, "MeshLibrary exported successfully to: exported/meshlib.res") {
			t.Errorf("text = %q", got)
		}
		if _, err := os.Stat(filepath.Join(root, "exported", "meshlib.res")); err != nil {
			t.Errorf("stat exported mesh library: %v", err)
		}
	})

	t.Run("specific mesh item names", func(t *testing.T) {
		root := t.TempDir()
		seedProject(t, root)

		res := callTool(t, session, "create_scene", map[string]any{
			"projectPath":  root,
			"scenePath":    "scenes/main.tscn",
			"rootNodeType": "Node3D",
		})
		if res.IsError {
			t.Skipf("create_scene failed, skipping: %s", firstText(t, res))
		}

		writeE2EFile(t, filepath.Join(root, "box.tres"), "[gd_resource type=\"BoxMesh\" format=3]\n\n[resource]\nsize = Vector3(1, 1, 1)\n")

		res = callTool(t, session, "add_node", map[string]any{
			"projectPath": root,
			"scenePath":   "scenes/main.tscn",
			"nodeType":    "MeshInstance3D",
			"nodeName":    "Mesh",
			"properties":  map[string]any{"mesh": "res://box.tres"},
		})
		if res.IsError {
			t.Skipf("add_node failed, skipping: %s", firstText(t, res))
		}

		res = callTool(t, session, "export_mesh_library", map[string]any{
			"projectPath":   root,
			"scenePath":     "scenes/main.tscn",
			"outputPath":    "exported/named.res",
			"meshItemNames": []any{"Mesh"},
		})
		if res.IsError {
			t.Fatalf("unexpected error: %s", firstText(t, res))
		}
		if got := firstText(t, res); !strings.HasPrefix(got, "MeshLibrary exported successfully to: exported/named.res") {
			t.Errorf("text = %q", got)
		}
		if _, err := os.Stat(filepath.Join(root, "exported", "named.res")); err != nil {
			t.Errorf("stat exported mesh library: %v", err)
		}
	})
}

// TestExportMeshLibraryErrors covers the reference's validation messages.
func TestExportMeshLibraryErrors(t *testing.T) {
	session := connect(t)

	t.Run("missing params", func(t *testing.T) {
		res := callTool(t, session, "export_mesh_library", map[string]any{})
		if !res.IsError {
			t.Fatal("expected an error result")
		}
		if got := firstText(t, res); got != "Missing required parameters" {
			t.Errorf("message = %q", got)
		}
	})

	t.Run("invalid path", func(t *testing.T) {
		res := callTool(t, session, "export_mesh_library", map[string]any{
			"projectPath": "../escape",
			"scenePath":   "scenes/main.tscn",
			"outputPath":  "exported/meshlib.res",
		})
		if !res.IsError {
			t.Fatal("expected an error result")
		}
		if got := firstText(t, res); got != "Invalid path" {
			t.Errorf("message = %q", got)
		}
	})

	t.Run("not a project", func(t *testing.T) {
		root := t.TempDir()
		res := callTool(t, session, "export_mesh_library", map[string]any{
			"projectPath": root,
			"scenePath":   "scenes/main.tscn",
			"outputPath":  "exported/meshlib.res",
		})
		if !res.IsError {
			t.Fatal("expected an error result")
		}
		if got := firstText(t, res); got != "Not a valid Godot project: "+root {
			t.Errorf("message = %q", got)
		}
	})

	t.Run("missing scene", func(t *testing.T) {
		root := t.TempDir()
		seedProject(t, root)
		res := callTool(t, session, "export_mesh_library", map[string]any{
			"projectPath": root,
			"scenePath":   "missing.tscn",
			"outputPath":  "exported/meshlib.res",
		})
		if !res.IsError {
			t.Fatal("expected an error result")
		}
		if got := firstText(t, res); got != "Scene file does not exist: missing.tscn" {
			t.Errorf("message = %q", got)
		}
	})
}
