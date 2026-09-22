package e2e

import (
	"encoding/json"
	"testing"
)

// sceneNode mirrors the JSON node tree read_scene returns for assertions.
type sceneNode struct {
	Name     string      `json:"name"`
	Type     string      `json:"type"`
	Children []sceneNode `json:"children"`
}

// TestReadSceneSchema ports the reference's tool-definition expectations for
// read_scene over the real stdio surface.
func TestReadSceneSchema(t *testing.T) {
	schema := requireSchema(t, "read_scene", "Read scene file as JSON node tree (headless)")
	schema.requireRequired(t, "projectPath", "scenePath")
	schema.requireStringProps(t, "projectPath", "scenePath")
	schema.requirePropCount(t, 2)
}

// TestReadSceneTool drives the real read_scene tool against Godot and parses
// the returned JSON node tree.
func TestReadSceneTool(t *testing.T) {
	session := connect(t)
	requireGodot(t, session)

	t.Run("reads the root node", func(t *testing.T) {
		root := seedScene(t, session, "scenes/main.tscn", "Node2D")

		text := requireToolText(t, session, "read_scene", map[string]any{
			"projectPath": root,
			"scenePath":   "scenes/main.tscn",
		})

		var tree sceneNode
		if err := json.Unmarshal([]byte(text), &tree); err != nil {
			t.Fatalf("unmarshal %q: %v", text, err)
		}
		if tree.Name != "root" || tree.Type != "Node2D" {
			t.Errorf("root = %+v", tree)
		}
	})

	t.Run("reads a child node", func(t *testing.T) {
		root := seedScene(t, session, "scenes/tree.tscn", "Node2D")

		requireToolText(t, session, "add_node", map[string]any{
			"projectPath": root,
			"scenePath":   "scenes/tree.tscn",
			"nodeType":    "Sprite2D",
			"nodeName":    "Player",
		})

		text := requireToolText(t, session, "read_scene", map[string]any{
			"projectPath": root,
			"scenePath":   "scenes/tree.tscn",
		})

		var tree sceneNode
		if err := json.Unmarshal([]byte(text), &tree); err != nil {
			t.Fatalf("unmarshal %q: %v", text, err)
		}
		if len(tree.Children) != 1 {
			t.Fatalf("children = %+v", tree.Children)
		}
		if got := tree.Children[0]; got.Name != "Player" || got.Type != "Sprite2D" {
			t.Errorf("child = %+v", got)
		}
	})
}

// TestReadSceneErrors covers the reference's validation and failure messages.
func TestReadSceneErrors(t *testing.T) {
	session := connect(t)

	t.Run("missing paths", func(t *testing.T) {
		requireToolError(t, session, "read_scene", map[string]any{}, "projectPath and scenePath are required.")
	})

	t.Run("invalid path", func(t *testing.T) {
		requireToolError(t, session, "read_scene", map[string]any{
			"projectPath": "../escape",
			"scenePath":   "main.tscn",
		}, "Invalid path.")
	})

	t.Run("not a project", func(t *testing.T) {
		root := t.TempDir()
		requireToolError(t, session, "read_scene", map[string]any{
			"projectPath": root,
			"scenePath":   "main.tscn",
		}, "Not a valid Godot project: "+root)
	})

	t.Run("missing scene", func(t *testing.T) {
		root := t.TempDir()
		seedProject(t, root)
		requireToolError(t, session, "read_scene", map[string]any{
			"projectPath": root,
			"scenePath":   "missing.tscn",
		}, "Scene file does not exist: missing.tscn")
	})
}
