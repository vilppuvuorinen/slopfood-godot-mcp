package e2e

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const boxMeshResource = "[gd_resource type=\"BoxMesh\" format=3]\n\n[resource]\nsize = Vector3(1, 1, 1)\n"

// TestExportMeshLibrarySchema ports the reference's tool-definition
// expectations for export_mesh_library over the real stdio surface.
func TestExportMeshLibrarySchema(t *testing.T) {
	schema := requireSchema(t, "export_mesh_library", "Export a scene as a MeshLibrary resource")
	schema.requireRequired(t, "scenePath", "outputPath")
	schema.requireType(t, "meshItemNames", "array")

	items, ok := schema.Properties["meshItemNames"]["items"].(map[string]any)
	if !ok || items["type"] != "string" {
		t.Errorf("meshItemNames items = %v", schema.Properties["meshItemNames"]["items"])
	}
}

// seedMeshScene creates a Node3D scene with a mesh-bearing node so
// export_mesh_library has something to export.
func seedMeshScene(t *testing.T) (string, *mcp.ClientSession) {
	t.Helper()
	root, session := seedScene(t, "scenes/main.tscn", "Node3D")
	writeE2EFile(t, filepath.Join(root, "box.tres"), boxMeshResource)

	res := callTool(t, session, "add_node", map[string]any{
		"scenePath":  "scenes/main.tscn",
		"nodeType":   "MeshInstance3D",
		"nodeName":   "Mesh",
		"properties": map[string]any{"mesh": "res://box.tres"},
	})
	if res.IsError {
		t.Skipf("add_node failed, skipping: %s", firstText(t, res))
	}
	return root, session
}

// TestExportMeshLibraryTool drives the real export_mesh_library tool against
// Godot, including the optional meshItemNames filter.
func TestExportMeshLibraryTool(t *testing.T) {
	t.Run("exports a mesh library", func(t *testing.T) {
		root, session := seedMeshScene(t)

		output := filepath.Join(root, "exported", "meshlib.res")
		text := requireToolText(t, session, "export_mesh_library", map[string]any{
			"scenePath":  "scenes/main.tscn",
			"outputPath": "exported/meshlib.res",
		})
		requirePrefix(t, text, "MeshLibrary exported successfully to: "+output)
		if _, err := os.Stat(output); err != nil {
			t.Errorf("stat exported mesh library: %v", err)
		}
	})

	t.Run("absolute output path", func(t *testing.T) {
		_, session := seedMeshScene(t)

		output := filepath.Join(t.TempDir(), "nested", "meshlib.res")
		text := requireToolText(t, session, "export_mesh_library", map[string]any{
			"scenePath":  "scenes/main.tscn",
			"outputPath": output,
		})
		requirePrefix(t, text, "MeshLibrary exported successfully to: "+output)
		if _, err := os.Stat(output); err != nil {
			t.Errorf("stat exported mesh library: %v", err)
		}
	})

	t.Run("specific mesh item names", func(t *testing.T) {
		root, session := seedMeshScene(t)

		output := filepath.Join(root, "exported", "named.res")
		text := requireToolText(t, session, "export_mesh_library", map[string]any{
			"scenePath":     "scenes/main.tscn",
			"outputPath":    "exported/named.res",
			"meshItemNames": []any{"Mesh"},
		})
		requirePrefix(t, text, "MeshLibrary exported successfully to: "+output)
		if _, err := os.Stat(output); err != nil {
			t.Errorf("stat exported mesh library: %v", err)
		}
	})
}

// TestExportMeshLibraryErrors covers the reference's validation messages.
func TestExportMeshLibraryErrors(t *testing.T) {
	t.Run("missing params", func(t *testing.T) {
		root := newProject(t)
		session := connectProject(t, root)
		requireToolError(t, session, "export_mesh_library", map[string]any{}, "Missing required parameters")
	})

	t.Run("invalid path", func(t *testing.T) {
		root := newProject(t)
		session := connectProject(t, root)
		requireToolError(t, session, "export_mesh_library", map[string]any{
			"scenePath":  "../escape",
			"outputPath": "exported/meshlib.res",
		}, "Invalid path")
	})

	t.Run("parent output path", func(t *testing.T) {
		root := newProject(t)
		session := connectProject(t, root)
		requireToolError(t, session, "export_mesh_library", map[string]any{
			"scenePath":  "scenes/main.tscn",
			"outputPath": "../escape.res",
		}, "Invalid path")
	})

	t.Run("not a project", func(t *testing.T) {
		root := t.TempDir()
		session := connectProject(t, root)
		requireToolError(t, session, "export_mesh_library", map[string]any{
			"scenePath":  "scenes/main.tscn",
			"outputPath": "exported/meshlib.res",
		}, "Not a valid Godot project: "+root)
	})

	t.Run("missing scene", func(t *testing.T) {
		root := newProject(t)
		session := connectProject(t, root)
		requireToolError(t, session, "export_mesh_library", map[string]any{
			"scenePath":  "missing.tscn",
			"outputPath": "exported/meshlib.res",
		}, "Scene file does not exist: missing.tscn")
	})

	t.Run("no meshes in scene", func(t *testing.T) {
		_, session := seedScene(t, "scenes/empty.tscn", "Node3D")

		res := callTool(t, session, "export_mesh_library", map[string]any{
			"scenePath":  "scenes/empty.tscn",
			"outputPath": "exported/empty.res",
		})
		if !res.IsError {
			t.Fatal("export_mesh_library: expected an error result")
		}
		requireContains(t, firstText(t, res), "Failed to export mesh library", "No valid meshes found in the scene")
	})

	t.Run("output directory failure", func(t *testing.T) {
		_, session := seedMeshScene(t)

		// /proc is not writable, so the engine reports both the directory and
		// the save failure.
		res := callTool(t, session, "export_mesh_library", map[string]any{
			"scenePath":  "scenes/main.tscn",
			"outputPath": "/proc/nope/meshlib.res",
		})
		if !res.IsError {
			t.Fatal("export_mesh_library: expected an error result")
		}
		requireContains(t, firstText(t, res), "Failed to export mesh library", "Failed to create directory", "Failed to save MeshLibrary")
	})

	t.Run("save failure", func(t *testing.T) {
		_, session := seedMeshScene(t)

		// The root filesystem is not writable here, so the directory exists
		// but the save itself fails.
		res := callTool(t, session, "export_mesh_library", map[string]any{
			"scenePath":  "scenes/main.tscn",
			"outputPath": "/meshlib.res",
		})
		if !res.IsError {
			t.Fatal("export_mesh_library: expected an error result")
		}
		requireContains(t, firstText(t, res), "Failed to export mesh library", "Failed to save MeshLibrary")
	})
}
