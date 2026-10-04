package e2e

import (
	"encoding/json"
	"path/filepath"
	"reflect"
	"testing"
)

// readEnvelope is the self-identifying outer contract shared by read and
// read_node: kind + path (+ nodePath when a node was addressed) wrapping a
// kind-specific data body.
type readEnvelope[T any] struct {
	Kind     string `json:"kind"`
	Path     string `json:"path"`
	NodePath string `json:"nodePath"`
	Data     T      `json:"data"`
}

type sceneData struct {
	Root          sceneNode             `json:"root"`
	BaseScenePath string                `json:"baseScenePath"`
	Connections   []testSceneConnection `json:"connections"`
}

type sceneNode struct {
	Name       string         `json:"name"`
	Type       string         `json:"type"`
	Script     string         `json:"script"`
	Groups     []string       `json:"groups"`
	Properties map[string]any `json:"properties"`
	Children   []sceneNode    `json:"children"`
}

func unmarshalSceneEnvelope(t *testing.T, text string) readEnvelope[sceneData] {
	t.Helper()
	var env readEnvelope[sceneData]
	if err := json.Unmarshal([]byte(text), &env); err != nil {
		t.Fatalf("unmarshal %q: %v", text, err)
	}
	return env
}

func TestReadSceneTool(t *testing.T) {
	t.Parallel()
	root := newProject(t)
	session := connectProject(t, root)

	t.Run("reads the root node", func(t *testing.T) {
		createScene(t, session, "scenes/main.tscn", "Node2D")

		text := requireToolText(t, session, "read", map[string]any{
			"type": "scene",
			"path": "scenes/main.tscn",
		})

		env := unmarshalSceneEnvelope(t, text)
		if env.Kind != "scene" || env.Path != "scenes/main.tscn" {
			t.Errorf("envelope = %+v, want kind scene at scenes/main.tscn", env)
		}
		if tree := env.Data.Root; tree.Name != "root" || tree.Type != "Node2D" {
			t.Errorf("root = %+v", tree)
		}
	})

	t.Run("reads a child node", func(t *testing.T) {
		createScene(t, session, "scenes/tree.tscn", "Node2D")
		addNodeViaScript(t, session, "scenes/tree.tscn", "", "Player", "Sprite2D", nil)

		text := requireToolText(t, session, "read", map[string]any{
			"type": "scene",
			"path": "scenes/tree.tscn",
		})

		tree := unmarshalSceneEnvelope(t, text).Data.Root
		if len(tree.Children) != 1 {
			t.Fatalf("children = %+v", tree.Children)
		}
		if got := tree.Children[0]; got.Name != "Player" || got.Type != "Sprite2D" {
			t.Errorf("child = %+v", got)
		}
	})

	t.Run("scopes to a node and depth", func(t *testing.T) {
		createScene(t, session, "scenes/scope.tscn", "Node2D")
		addNodeViaScript(t, session, "scenes/scope.tscn", "", "Player", "Sprite2D", nil)
		addNodeViaScript(t, session, "scenes/scope.tscn", "Player", "Weapon", "Node2D", nil)

		text := requireToolText(t, session, "read_node", map[string]any{
			"type":     "scene",
			"path":     "scenes/scope.tscn",
			"nodePath": "root/Player",
		})
		env := unmarshalSceneEnvelope(t, text)
		if env.NodePath != "root/Player" {
			t.Errorf("envelope nodePath = %q, want root/Player", env.NodePath)
		}
		scoped := env.Data.Root
		if scoped.Name != "Player" {
			t.Errorf("scoped root = %+v, want Player", scoped)
		}
		if len(scoped.Children) != 1 || scoped.Children[0].Name != "Weapon" {
			t.Errorf("scoped children = %+v, want Weapon", scoped.Children)
		}

		text = requireToolText(t, session, "read", map[string]any{
			"type":  "scene",
			"path":  "scenes/scope.tscn",
			"depth": 1,
		})
		shallow := unmarshalSceneEnvelope(t, text).Data.Root
		if len(shallow.Children) != 1 || shallow.Children[0].Name != "Player" {
			t.Fatalf("depth-1 children = %+v, want Player", shallow.Children)
		}
		if len(shallow.Children[0].Children) != 0 {
			t.Errorf("depth-1 grandchildren = %+v, want none", shallow.Children[0].Children)
		}
	})

	t.Run("depth zero returns the node alone", func(t *testing.T) {
		createScene(t, session, "scenes/depth0.tscn", "Node2D")
		addNodeViaScript(t, session, "scenes/depth0.tscn", "", "Player", "Sprite2D", nil)

		text := requireToolText(t, session, "read", map[string]any{
			"type":  "scene",
			"path":  "scenes/depth0.tscn",
			"depth": 0,
		})
		tree := unmarshalSceneEnvelope(t, text).Data.Root
		if tree.Name != "root" || len(tree.Children) != 0 {
			t.Errorf("depth-0 tree = %+v, want the root with no children", tree)
		}
	})

	t.Run("explicit root nodePath", func(t *testing.T) {
		createScene(t, session, "scenes/rootpath.tscn", "Node2D")

		text := requireToolText(t, session, "read_node", map[string]any{
			"type":     "scene",
			"path":     "scenes/rootpath.tscn",
			"nodePath": "root",
		})
		tree := unmarshalSceneEnvelope(t, text).Data.Root
		if tree.Name != "root" || tree.Type != "Node2D" {
			t.Errorf("root = %+v", tree)
		}
	})

	t.Run("unknown nodePath", func(t *testing.T) {
		createScene(t, session, "scenes/unknown.tscn", "Node2D")

		res := callTool(t, session, "read_node", map[string]any{
			"type":     "scene",
			"path":     "scenes/unknown.tscn",
			"nodePath": "Nope",
		})
		if !res.IsError {
			t.Fatal("expected an error for an unknown nodePath")
		}
		requirePrefix(t, firstText(t, res), "Failed to read scene:")
	})

	t.Run("accepts res:// path", func(t *testing.T) {
		createScene(t, session, "scenes/respath.tscn", "Node2D")

		text := requireToolText(t, session, "read", map[string]any{
			"type": "scene",
			"path": "res://scenes/respath.tscn",
		})
		tree := unmarshalSceneEnvelope(t, text).Data.Root
		if tree.Name != "root" || tree.Type != "Node2D" {
			t.Errorf("root = %+v", tree)
		}
	})

	t.Run("non-scene resource is an error", func(t *testing.T) {
		writeE2EFile(t, filepath.Join(root, "thing.tres"), resourceTres)

		// A .tres loads as a Resource, not a PackedScene, so read(type=scene) errors.
		res := callTool(t, session, "read", map[string]any{
			"type": "scene",
			"path": "thing.tres",
		})
		if !res.IsError {
			t.Fatal("expected an error for a non-scene resource")
		}
		requirePrefix(t, firstText(t, res), "Failed to read scene:")
		requireContains(t, firstText(t, res), "Not a scene file")
	})
}

// JSON-native types stay nested; engine-only types stay strings.
func TestReadSceneNestedProperties(t *testing.T) {
	t.Parallel()
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

	text := requireToolText(t, session, "read", map[string]any{
		"type": "scene",
		"path": "scenes/dict.tscn",
	})

	tree := unmarshalSceneEnvelope(t, text).Data.Root
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

// Hand-written .tscn: no tool exposes group membership.
func TestReadSceneGroups(t *testing.T) {
	t.Parallel()
	root := newProject(t)
	session := connectProject(t, root)

	writeE2EFile(t, filepath.Join(root, "scenes", "groups.tscn"), `[gd_scene format=3]

[node name="root" type="Node2D" groups=["enemies"]]

[node name="Child" type="Node2D" parent="." groups=["pickups", "enemies"]]
`)

	text := requireToolText(t, session, "read", map[string]any{
		"type": "scene",
		"path": "scenes/groups.tscn",
	})
	tree := unmarshalSceneEnvelope(t, text).Data.Root
	if !reflect.DeepEqual(tree.Groups, []string{"enemies"}) {
		t.Errorf("root groups = %v, want [enemies]", tree.Groups)
	}
	if len(tree.Children) != 1 {
		t.Fatalf("children = %+v", tree.Children)
	}
	if got := tree.Children[0].Groups; !reflect.DeepEqual(got, []string{"pickups", "enemies"}) {
		t.Errorf("child groups = %v, want [pickups enemies]", got)
	}
}

func TestReadSceneErrors(t *testing.T) {
	t.Parallel()
	root := newProject(t)
	session := connectProject(t, root)

	t.Run("missing type and path", func(t *testing.T) {
		requireToolError(t, session, "read", map[string]any{}, "type is required.")
	})

	t.Run("missing path", func(t *testing.T) {
		requireToolError(t, session, "read", map[string]any{"type": "scene"}, "path is required.")
	})

	t.Run("not a project", func(t *testing.T) {
		notProject := t.TempDir()
		notProjectSession := connectProject(t, notProject)
		requireToolError(t, notProjectSession, "read", map[string]any{
			"type": "scene",
			"path": "main.tscn",
		}, "Not a valid Godot project: "+notProject)
	})

	t.Run("missing scene", func(t *testing.T) {
		requireToolError(t, session, "read", map[string]any{
			"type": "scene",
			"path": "missing.tscn",
		}, "Scene file does not exist: missing.tscn")
	})

	t.Run("negative depth", func(t *testing.T) {
		requireToolError(t, session, "read", map[string]any{
			"type":  "scene",
			"path":  "main.tscn",
			"depth": -1,
		}, "depth must be zero or greater.")
	})
}
