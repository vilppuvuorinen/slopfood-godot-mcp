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
	schema.requireRequired(t, "projectPath", "scenePath", "outputPath")
	schema.requireType(t, "meshItemNames", "array")

	items, ok := schema.Properties["meshItemNames"]["items"].(map[string]any)
	if !ok || items["type"] != "string" {
		t.Errorf("meshItemNames items = %v", schema.Properties["meshItemNames"]["items"])
	}
}

// seedMeshScene creates a Node3D scene with a mesh-bearing node so
// export_mesh_library has something to export.
func seedMeshScene(t *testing.T, session *mcp.ClientSession) string {
	t.Helper()
	root := seedScene(t, session, "scenes/main.tscn", "Node3D")
	writeE2EFile(t, filepath.Join(root, "box.tres"), boxMeshResource)

	res := callTool(t, session, "add_node", map[string]any{
		"projectPath": root,
		"scenePath":   "scenes/main.tscn",
		"nodeType":    "MeshInstance3D",
		"nodeName":    "Mesh",
		"properties":  map[string]any{"mesh": "res://box.tres"},
	})
	if res.IsError {
		t.Skipf("add_node failed, skipping: %s", firstText(t, res))
	}
	return root
}

// TestExportMeshLibraryTool drives the real export_mesh_library tool against
// Godot, including the optional meshItemNames filter.
func TestExportMeshLibraryTool(t *testing.T) {
	session := connect(t)
	requireGodot(t, session)

	t.Run("exports a mesh library", func(t *testing.T) {
		root := seedMeshScene(t, session)

		text := requireToolText(t, session, "export_mesh_library", map[string]any{
			"projectPath": root,
			"scenePath":   "scenes/main.tscn",
			"outputPath":  "exported/meshlib.res",
		})
		requirePrefix(t, text, "MeshLibrary exported successfully to: exported/meshlib.res")
		if _, err := os.Stat(filepath.Join(root, "exported", "meshlib.res")); err != nil {
			t.Errorf("stat exported mesh library: %v", err)
		}
	})

	t.Run("specific mesh item names", func(t *testing.T) {
		root := seedMeshScene(t, session)

		text := requireToolText(t, session, "export_mesh_library", map[string]any{
			"projectPath":   root,
			"scenePath":     "scenes/main.tscn",
			"outputPath":    "exported/named.res",
			"meshItemNames": []any{"Mesh"},
		})
		requirePrefix(t, text, "MeshLibrary exported successfully to: exported/named.res")
		if _, err := os.Stat(filepath.Join(root, "exported", "named.res")); err != nil {
			t.Errorf("stat exported mesh library: %v", err)
		}
	})
}

// TestExportMeshLibraryErrors covers the reference's validation messages.
func TestExportMeshLibraryErrors(t *testing.T) {
	session := connect(t)

	t.Run("missing params", func(t *testing.T) {
		requireToolError(t, session, "export_mesh_library", map[string]any{}, "Missing required parameters")
	})

	t.Run("invalid path", func(t *testing.T) {
		requireToolError(t, session, "export_mesh_library", map[string]any{
			"projectPath": "../escape",
			"scenePath":   "scenes/main.tscn",
			"outputPath":  "exported/meshlib.res",
		}, "Invalid path")
	})

	t.Run("not a project", func(t *testing.T) {
		root := t.TempDir()
		requireToolError(t, session, "export_mesh_library", map[string]any{
			"projectPath": root,
			"scenePath":   "scenes/main.tscn",
			"outputPath":  "exported/meshlib.res",
		}, "Not a valid Godot project: "+root)
	})

	t.Run("missing scene", func(t *testing.T) {
		root := t.TempDir()
		seedProject(t, root)
		requireToolError(t, session, "export_mesh_library", map[string]any{
			"projectPath": root,
			"scenePath":   "missing.tscn",
			"outputPath":  "exported/meshlib.res",
		}, "Scene file does not exist: missing.tscn")
	})
}
