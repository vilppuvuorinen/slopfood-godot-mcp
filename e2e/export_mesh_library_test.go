package e2e

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const boxMeshResource = "[gd_resource type=\"BoxMesh\" format=3]\n\n[resource]\nsize = Vector3(1, 1, 1)\n"

// A plain BoxMesh reaches neither branch of export_mesh_library, so these fixtures add a collision shape and an albedo texture.
const (
	gradientTextureResource = `[gd_resource type="GradientTexture2D" format=3]

[resource]
width = 4
height = 4
`

	texturedMaterialResource = `[gd_resource type="StandardMaterial3D" format=3]

[ext_resource type="Texture2D" path="res://gradient.tres" id="1_tex"]

[resource]
albedo_texture = ExtResource("1_tex")
`

	texturedBoxMeshResource = `[gd_resource type="BoxMesh" format=3]

[ext_resource type="Material" path="res://mat.tres" id="1_mat"]

[resource]
material = ExtResource("1_mat")
`

	boxShapeResource = `[gd_resource type="BoxShape3D" format=3]

[resource]
size = Vector3(1, 1, 1)
`
)

func seedMeshScene(t *testing.T, session *mcp.ClientSession, scenePath string) {
	t.Helper()
	createScene(t, session, scenePath, "Node3D")

	res := callTool(t, session, "add_node", map[string]any{
		"scenePath":  scenePath,
		"nodeType":   "MeshInstance3D",
		"nodeName":   "Mesh",
		"properties": map[string]any{"mesh": "res://box.tres"},
	})
	if res.IsError {
		t.Fatalf("add_node failed: %s", firstText(t, res))
	}
}

func TestExportMeshLibraryTool(t *testing.T) {
	t.Parallel()
	root := newProject(t)
	session := connectProject(t, root)
	writeE2EFile(t, filepath.Join(root, "box.tres"), boxMeshResource)

	t.Run("exports a mesh library", func(t *testing.T) {
		const scene = "scenes/main.tscn"
		seedMeshScene(t, session, scene)

		output := filepath.Join(root, "exported", "meshlib.res")
		text := requireToolText(t, session, "export_mesh_library", map[string]any{
			"scenePath":  scene,
			"outputPath": "exported/meshlib.res",
		})
		requirePrefix(t, text, "MeshLibrary exported successfully to: "+output)
		assertNonEmptyFile(t, output)
	})

	t.Run("absolute output path", func(t *testing.T) {
		const scene = "scenes/absolute.tscn"
		seedMeshScene(t, session, scene)

		output := filepath.Join(t.TempDir(), "nested", "meshlib.res")
		text := requireToolText(t, session, "export_mesh_library", map[string]any{
			"scenePath":  scene,
			"outputPath": output,
		})
		requirePrefix(t, text, "MeshLibrary exported successfully to: "+output)
		assertNonEmptyFile(t, output)
	})

	t.Run("specific mesh item names", func(t *testing.T) {
		const scene = "scenes/named.tscn"
		seedMeshScene(t, session, scene)
		// A second mesh the filter must exclude, so an unfiltered export cannot pass.
		requireToolText(t, session, "add_node", map[string]any{
			"scenePath":  scene,
			"nodeType":   "MeshInstance3D",
			"nodeName":   "Other",
			"properties": map[string]any{"mesh": "res://box.tres"},
		})

		output := filepath.Join(root, "exported", "named.tres")
		text := requireToolText(t, session, "export_mesh_library", map[string]any{
			"scenePath":     scene,
			"outputPath":    "exported/named.tres",
			"meshItemNames": []any{"Mesh"},
		})
		requirePrefix(t, text, "MeshLibrary exported successfully to: "+output)

		library := readFile(t, output)
		requireContains(t, library, `item/0/name = "Mesh"`)
		requireNotContains(t, library, `item/1`)
		requireNotContains(t, library, `"Other"`)
	})
}

// Covers a CollisionShape3D child becoming an item shape and an albedo-textured mesh becoming the item preview.
func TestExportMeshLibraryCollisionAndPreview(t *testing.T) {
	t.Parallel()
	root, session := seedScene(t, "scenes/main.tscn", "Node3D")
	writeE2EFile(t, filepath.Join(root, "gradient.tres"), gradientTextureResource)
	writeE2EFile(t, filepath.Join(root, "mat.tres"), texturedMaterialResource)
	writeE2EFile(t, filepath.Join(root, "box.tres"), texturedBoxMeshResource)
	writeE2EFile(t, filepath.Join(root, "box_shape.tres"), boxShapeResource)

	requireToolText(t, session, "add_node", map[string]any{
		"scenePath":  "scenes/main.tscn",
		"nodeType":   "MeshInstance3D",
		"nodeName":   "Mesh",
		"properties": map[string]any{"mesh": "res://box.tres"},
	})
	requireToolText(t, session, "add_node", map[string]any{
		"scenePath":      "scenes/main.tscn",
		"nodeType":       "CollisionShape3D",
		"nodeName":       "Shape",
		"parentNodePath": "root/Mesh",
		"properties":     map[string]any{"shape": "res://box_shape.tres"},
	})

	output := filepath.Join(root, "exported", "meshlib.tres")
	text := requireToolText(t, session, "export_mesh_library", map[string]any{
		"scenePath":  "scenes/main.tscn",
		"outputPath": "exported/meshlib.tres",
	})
	requirePrefix(t, text, "MeshLibrary exported successfully to: "+output)

	library := readFile(t, output)
	requireContains(t, library,
		`item/0/shapes = [`,
		`item/0/preview = ExtResource(`,
		`path="res://gradient.tres"`,
		`path="res://box_shape.tres"`,
	)
}

// Covers a mesh found on a descendant holder and the absence of a preview without an albedo texture.
func TestExportMeshLibraryDescendantAndPreview(t *testing.T) {
	t.Parallel()
	root := newProject(t)
	session := connectProject(t, root)
	writeE2EFile(t, filepath.Join(root, "box.tres"), boxMeshResource)

	t.Run("finds a mesh on a descendant", func(t *testing.T) {
		const scene = "scenes/descendant.tscn"
		createScene(t, session, scene, "Node3D")

		// Holder is not itself a mesh; only the descendant search can collect its child.
		requireToolText(t, session, "add_node", map[string]any{
			"scenePath": scene,
			"nodeType":  "Node3D",
			"nodeName":  "Holder",
		})
		requireToolText(t, session, "add_node", map[string]any{
			"scenePath":      scene,
			"nodeType":       "MeshInstance3D",
			"nodeName":       "Mesh",
			"parentNodePath": "root/Holder",
			"properties":     map[string]any{"mesh": "res://box.tres"},
		})

		output := filepath.Join(root, "exported", "descendant.tres")
		text := requireToolText(t, session, "export_mesh_library", map[string]any{
			"scenePath":  scene,
			"outputPath": "exported/descendant.tres",
		})
		requirePrefix(t, text, "MeshLibrary exported successfully to: "+output)

		// The item is named for the top-level child, not the found descendant.
		requireContains(t, readFile(t, output), `item/0/name = "Holder"`, `path="res://box.tres"`)
	})

	t.Run("omits a preview when the mesh has no albedo texture", func(t *testing.T) {
		const scene = "scenes/plain.tscn"
		seedMeshScene(t, session, scene)

		output := filepath.Join(root, "exported", "plain.tres")
		requireToolText(t, session, "export_mesh_library", map[string]any{
			"scenePath":  scene,
			"outputPath": "exported/plain.tres",
		})
		requireNotContains(t, readFile(t, output), "preview")
	})
}

func TestExportMeshLibraryErrors(t *testing.T) {
	t.Parallel()
	root := newProject(t)
	session := connectProject(t, root)
	writeE2EFile(t, filepath.Join(root, "box.tres"), boxMeshResource)

	t.Run("missing params", func(t *testing.T) {
		requireToolError(t, session, "export_mesh_library", map[string]any{}, "scenePath and outputPath are required.")
	})

	t.Run("invalid path", func(t *testing.T) {
		requireToolError(t, session, "export_mesh_library", map[string]any{
			"scenePath":  "../escape",
			"outputPath": "exported/meshlib.res",
		}, "Invalid path.")
	})

	t.Run("parent output path", func(t *testing.T) {
		requireToolError(t, session, "export_mesh_library", map[string]any{
			"scenePath":  "scenes/main.tscn",
			"outputPath": "../escape.res",
		}, "Invalid path.")
	})

	t.Run("not a project", func(t *testing.T) {
		notProject := t.TempDir()
		notProjectSession := connectProject(t, notProject)
		requireToolError(t, notProjectSession, "export_mesh_library", map[string]any{
			"scenePath":  "scenes/main.tscn",
			"outputPath": "exported/meshlib.res",
		}, "Not a valid Godot project: "+notProject)
	})

	t.Run("missing scene", func(t *testing.T) {
		requireToolError(t, session, "export_mesh_library", map[string]any{
			"scenePath":  "missing.tscn",
			"outputPath": "exported/meshlib.res",
		}, "Scene file does not exist: missing.tscn")
	})

	t.Run("no meshes in scene", func(t *testing.T) {
		createScene(t, session, "scenes/empty.tscn", "Node3D")

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
		seedMeshScene(t, session, "scenes/blocked_dir.tscn")

		// An ancestor file makes make_dir_recursive_absolute fail on any filesystem; a file at the final component is reported OK.
		writeE2EFile(t, filepath.Join(root, "blocked"), "not a directory\n")

		res := callTool(t, session, "export_mesh_library", map[string]any{
			"scenePath":  "scenes/blocked_dir.tscn",
			"outputPath": filepath.Join(root, "blocked", "sub", "meshlib.res"),
		})
		if !res.IsError {
			t.Fatal("export_mesh_library: expected an error result")
		}
		// The handler must stop after the directory failure, not attempt a save that reports a second error.
		text := firstText(t, res)
		requireContains(t, text, "Failed to export mesh library", "Failed to create directory")
		requireNotContains(t, text, "Failed to save MeshLibrary")
	})

	t.Run("save failure", func(t *testing.T) {
		seedMeshScene(t, session, "scenes/save_fail.tscn")

		// A directory at the output path lets directory creation succeed but makes the save fail.
		if err := os.MkdirAll(filepath.Join(root, "exported", "meshlib_savefail.res"), 0o755); err != nil {
			t.Fatalf("seed output directory: %v", err)
		}

		res := callTool(t, session, "export_mesh_library", map[string]any{
			"scenePath":  "scenes/save_fail.tscn",
			"outputPath": "exported/meshlib_savefail.res",
		})
		if !res.IsError {
			t.Fatal("export_mesh_library: expected an error result")
		}
		requireContains(t, firstText(t, res), "Failed to export mesh library", "Failed to save MeshLibrary")
	})
}
