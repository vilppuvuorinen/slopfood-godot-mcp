package e2e

import (
	"encoding/json"
	"path/filepath"
	"reflect"
	"testing"
)

// sceneNode mirrors the JSON node tree read_scene returns for assertions.
type sceneNode struct {
	Name       string         `json:"name"`
	Type       string         `json:"type"`
	Script     string         `json:"script"`
	Properties map[string]any `json:"properties"`
	Children   []sceneNode    `json:"children"`
}

// TestReadSceneSchema ports the reference's tool-definition expectations for
// read_scene over the real stdio surface.
func TestReadSceneSchema(t *testing.T) {
	schema := requireSchema(t, "read_scene", "Read scene file as JSON node tree (headless)")
	schema.requireRequired(t, "scenePath")
	schema.requireStringProps(t, "scenePath")
	schema.requirePropCount(t, 1)
}

// TestReadSceneTool drives the real read_scene tool against Godot and parses
// the returned JSON node tree.
func TestReadSceneTool(t *testing.T) {
	t.Run("reads the root node", func(t *testing.T) {
		_, session := seedScene(t, "scenes/main.tscn", "Node2D")

		text := requireToolText(t, session, "read_scene", map[string]any{
			"scenePath": "scenes/main.tscn",
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
		_, session := seedScene(t, "scenes/tree.tscn", "Node2D")

		requireToolText(t, session, "add_node", map[string]any{
			"scenePath": "scenes/tree.tscn",
			"nodeType":  "Sprite2D",
			"nodeName":  "Player",
		})

		text := requireToolText(t, session, "read_scene", map[string]any{
			"scenePath": "scenes/tree.tscn",
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

// TestReadSceneNestedProperties drives the real read_scene tool against a node
// whose exported Dictionary property exercises the hybrid value encoding:
// JSON-native types stay nested, engine-only types stay strings.
func TestReadSceneNestedProperties(t *testing.T) {
	root, session := seedScene(t, "scenes/dict.tscn", "Node2D")
	writeE2EFile(t, filepath.Join(root, "scripts", "config.gd"),
		`extends Node2D

@export var config: Dictionary = {"myKey": 1, "nested": {"a": [1, 2, 3]}}
`)

	requireToolText(t, session, "attach_script", map[string]any{
		"scenePath":  "scenes/dict.tscn",
		"nodePath":   "root",
		"scriptPath": "scripts/config.gd",
	})

	text := requireToolText(t, session, "read_scene", map[string]any{
		"scenePath": "scenes/dict.tscn",
	})

	var tree sceneNode
	if err := json.Unmarshal([]byte(text), &tree); err != nil {
		t.Fatalf("unmarshal %q: %v", text, err)
	}
	if tree.Script != "scripts/config.gd" {
		t.Errorf("script = %q, want the project-relative scripts/config.gd", tree.Script)
	}
	config, ok := tree.Properties["config"].(map[string]any)
	if !ok {
		t.Fatalf("config = %T (%v), want a nested object", tree.Properties["config"], tree.Properties["config"])
	}
	if got := config["myKey"]; got != float64(1) {
		t.Errorf("config.myKey = %v, want 1", got)
	}
	nested, ok := config["nested"].(map[string]any)
	if !ok {
		t.Fatalf("config.nested = %T, want object", config["nested"])
	}
	if got := nested["a"]; !reflect.DeepEqual(got, []any{float64(1), float64(2), float64(3)}) {
		t.Errorf("config.nested.a = %v, want [1 2 3]", got)
	}
	position, ok := tree.Properties["position"].(map[string]any)
	if !ok || position["$type"] != "Vector2" {
		t.Errorf("position = %#v, want a tagged Vector2 object", tree.Properties["position"])
	}
}

// TestReadSceneErrors covers the reference's validation and failure messages.
func TestReadSceneErrors(t *testing.T) {
	t.Run("missing paths", func(t *testing.T) {
		session := connectProject(t, newProject(t))
		requireToolError(t, session, "read_scene", map[string]any{}, "scenePath is required.")
	})

	t.Run("not a project", func(t *testing.T) {
		root := t.TempDir()
		session := connectProject(t, root)
		requireToolError(t, session, "read_scene", map[string]any{
			"scenePath": "main.tscn",
		}, "Not a valid Godot project: "+root)
	})

	t.Run("missing scene", func(t *testing.T) {
		session := connectProject(t, newProject(t))
		requireToolError(t, session, "read_scene", map[string]any{
			"scenePath": "missing.tscn",
		}, "Scene file does not exist: missing.tscn")
	})
}
