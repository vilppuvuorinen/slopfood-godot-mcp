package e2e

import (
	"path/filepath"
	"testing"
)

// TestCreateSceneSchema ports the reference's tool-definition expectations for
// create_scene over the real stdio surface.
func TestCreateSceneSchema(t *testing.T) {
	schema := requireSchema(t, "create_scene", "Create a new Godot scene file")
	schema.requireRequired(t, "scenePath")
	schema.requireStringProps(t, "scenePath", "rootNodeType")
}

// TestCreateSceneTool drives the real create_scene tool against Godot,
// exercising the default and custom root node types.
func TestCreateSceneTool(t *testing.T) {
	t.Run("defaults to Node2D", func(t *testing.T) {
		root, _ := seedScene(t, "scenes/main.tscn")
		scene := readFile(t, filepath.Join(root, "scenes", "main.tscn"))
		requireContains(t, scene, `type="Node2D"`)
	})

	t.Run("custom root", func(t *testing.T) {
		root := newProject(t)
		session := connectProject(t, root)
		requireGodot(t, session)

		requireToolText(t, session, "create_scene", map[string]any{
			"scenePath":    "level.tscn",
			"rootNodeType": "Node3D",
		})

		scene := readFile(t, filepath.Join(root, "level.tscn"))
		requireContains(t, scene, `type="Node3D"`)
	})
}

// TestCreateSceneErrors covers the reference's validation and failure messages.
func TestCreateSceneErrors(t *testing.T) {
	t.Run("missing paths", func(t *testing.T) {
		root := newProject(t)
		session := connectProject(t, root)
		requireToolError(t, session, "create_scene", map[string]any{}, "scenePath is required.")
	})

	t.Run("invalid path", func(t *testing.T) {
		root := newProject(t)
		session := connectProject(t, root)
		requireToolError(t, session, "create_scene", map[string]any{
			"scenePath": "../escape",
		}, "Invalid path.")
	})

	t.Run("invalid rootNodeType", func(t *testing.T) {
		root := newProject(t)
		session := connectProject(t, root)
		requireToolError(t, session, "create_scene", map[string]any{
			"scenePath":    "main.tscn",
			"rootNodeType": "res://evil.gd",
		}, "Invalid rootNodeType")
	})

	t.Run("not a project", func(t *testing.T) {
		root := t.TempDir()
		session := connectProject(t, root)
		requireToolError(t, session, "create_scene", map[string]any{
			"scenePath": "main.tscn",
		}, "Not a valid Godot project: "+root)
	})

	t.Run("abstract rootNodeType", func(t *testing.T) {
		session := connectProject(t, newProject(t))
		requireGodot(t, session)

		// CanvasItem is a registered ClassDB class Godot refuses to
		// instantiate.
		res := callTool(t, session, "create_scene", map[string]any{
			"scenePath":    "abstract.tscn",
			"rootNodeType": "CanvasItem",
		})
		if !res.IsError {
			t.Fatal("create_scene: expected an error result")
		}
		requireContains(t, firstText(t, res),
			"Failed to create scene",
			"Class exists but cannot be instantiated: CanvasItem",
			"Failed to instantiate node of type: CanvasItem",
		)
	})
}

// TestCreateSceneFailsWithoutGodot points GODOT_PATH at a missing binary and
// asserts the reference's generic failure message and solutions. It uses its
// own session so the shared environment is untouched.
func TestCreateSceneFailsWithoutGodot(t *testing.T) {
	root := newProject(t)
	session := connectProjectWithEnv(t, root, "GODOT_PATH="+filepath.Join(t.TempDir(), "missing-godot"))

	res := callTool(t, session, "create_scene", map[string]any{
		"scenePath": "main.tscn",
	})
	if !res.IsError {
		t.Fatal("expected an error result")
	}
	requirePrefix(t, firstText(t, res), "Failed to create scene:")
	if len(res.Content) != 2 {
		t.Errorf("content = %+v", res.Content)
	}
}
