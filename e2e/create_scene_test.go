package e2e

import (
	"path/filepath"
	"testing"
)

func TestCreateSceneTool(t *testing.T) {
	t.Parallel()
	root := newProject(t)
	session := connectProject(t, root)

	t.Run("defaults to Node2D", func(t *testing.T) {
		createScene(t, session, "scenes/main.tscn")
		scene := readFile(t, filepath.Join(root, "scenes", "main.tscn"))
		requireContains(t, scene, `type="Node2D"`)
	})

	t.Run("custom root", func(t *testing.T) {
		requireToolText(t, session, "create_scene", map[string]any{
			"scenePath":    "level.tscn",
			"rootNodeType": "Node3D",
		})

		scene := readFile(t, filepath.Join(root, "level.tscn"))
		requireContains(t, scene, `type="Node3D"`)
	})
}

func TestCreateSceneErrors(t *testing.T) {
	t.Parallel()
	root := newProject(t)
	session := connectProject(t, root)

	t.Run("missing paths", func(t *testing.T) {
		requireToolError(t, session, "create_scene", map[string]any{}, "scenePath is required.")
	})

	t.Run("invalid path", func(t *testing.T) {
		requireToolError(t, session, "create_scene", map[string]any{
			"scenePath": "../escape",
		}, "Invalid path.")
	})

	t.Run("invalid rootNodeType", func(t *testing.T) {
		requireToolError(t, session, "create_scene", map[string]any{
			"scenePath":    "main.tscn",
			"rootNodeType": "res://evil.gd",
		}, "Invalid rootNodeType")
	})

	t.Run("not a project", func(t *testing.T) {
		notProject := t.TempDir()
		notProjectSession := connectProject(t, notProject)
		requireToolError(t, notProjectSession, "create_scene", map[string]any{
			"scenePath": "main.tscn",
		}, "Not a valid Godot project: "+notProject)
	})

	t.Run("abstract rootNodeType", func(t *testing.T) {
		// CanvasItem is a registered ClassDB class Godot refuses to instantiate.
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

// GODOT_PATH points at a missing binary; its own session keeps the shared environment untouched.
func TestCreateSceneFailsWithoutGodot(t *testing.T) {
	t.Parallel()
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
