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
	Groups     []string       `json:"groups"`
	Properties map[string]any `json:"properties"`
	Children   []sceneNode    `json:"children"`
}

// TestReadSceneTool drives the real read_scene tool against Godot and parses
// the returned JSON node tree.
func TestReadSceneTool(t *testing.T) {
	t.Parallel()
	// One warm project and session serve every subtest; each owns its scene.
	root := newProject(t)
	session := connectProject(t, root)

	t.Run("reads the root node", func(t *testing.T) {
		createScene(t, session, "scenes/main.tscn", "Node2D")

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
		createScene(t, session, "scenes/tree.tscn", "Node2D")

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

	t.Run("scopes to a node and depth", func(t *testing.T) {
		createScene(t, session, "scenes/scope.tscn", "Node2D")
		requireToolText(t, session, "add_node", map[string]any{
			"scenePath": "scenes/scope.tscn",
			"nodeType":  "Sprite2D",
			"nodeName":  "Player",
		})
		requireToolText(t, session, "add_node", map[string]any{
			"scenePath":      "scenes/scope.tscn",
			"nodeType":       "Node2D",
			"nodeName":       "Weapon",
			"parentNodePath": "Player",
		})

		text := requireToolText(t, session, "read_scene", map[string]any{
			"scenePath": "scenes/scope.tscn",
			"nodePath":  "root/Player",
		})
		var scoped sceneNode
		if err := json.Unmarshal([]byte(text), &scoped); err != nil {
			t.Fatalf("unmarshal %q: %v", text, err)
		}
		if scoped.Name != "Player" {
			t.Errorf("scoped root = %+v, want Player", scoped)
		}
		if len(scoped.Children) != 1 || scoped.Children[0].Name != "Weapon" {
			t.Errorf("scoped children = %+v, want Weapon", scoped.Children)
		}

		text = requireToolText(t, session, "read_scene", map[string]any{
			"scenePath": "scenes/scope.tscn",
			"depth":     1,
		})
		var shallow sceneNode
		if err := json.Unmarshal([]byte(text), &shallow); err != nil {
			t.Fatalf("unmarshal %q: %v", text, err)
		}
		if len(shallow.Children) != 1 || shallow.Children[0].Name != "Player" {
			t.Fatalf("depth-1 children = %+v, want Player", shallow.Children)
		}
		if len(shallow.Children[0].Children) != 0 {
			t.Errorf("depth-1 grandchildren = %+v, want none", shallow.Children[0].Children)
		}
	})

	t.Run("depth zero returns the node alone", func(t *testing.T) {
		createScene(t, session, "scenes/depth0.tscn", "Node2D")
		requireToolText(t, session, "add_node", map[string]any{
			"scenePath": "scenes/depth0.tscn",
			"nodeType":  "Sprite2D",
			"nodeName":  "Player",
		})

		text := requireToolText(t, session, "read_scene", map[string]any{
			"scenePath": "scenes/depth0.tscn",
			"depth":     0,
		})
		var tree sceneNode
		if err := json.Unmarshal([]byte(text), &tree); err != nil {
			t.Fatalf("unmarshal %q: %v", text, err)
		}
		if tree.Name != "root" || len(tree.Children) != 0 {
			t.Errorf("depth-0 tree = %+v, want the root with no children", tree)
		}
	})

	t.Run("explicit root nodePath", func(t *testing.T) {
		createScene(t, session, "scenes/rootpath.tscn", "Node2D")

		text := requireToolText(t, session, "read_scene", map[string]any{
			"scenePath": "scenes/rootpath.tscn",
			"nodePath":  "root",
		})
		var tree sceneNode
		if err := json.Unmarshal([]byte(text), &tree); err != nil {
			t.Fatalf("unmarshal %q: %v", text, err)
		}
		if tree.Name != "root" || tree.Type != "Node2D" {
			t.Errorf("root = %+v", tree)
		}
	})

	t.Run("unknown nodePath", func(t *testing.T) {
		createScene(t, session, "scenes/unknown.tscn", "Node2D")

		res := callTool(t, session, "read_scene", map[string]any{
			"scenePath": "scenes/unknown.tscn",
			"nodePath":  "Nope",
		})
		if !res.IsError {
			t.Fatal("expected an error for an unknown nodePath")
		}
		requirePrefix(t, firstText(t, res), "Failed to read scene:")
	})

	t.Run("non-scene resource is an error", func(t *testing.T) {
		writeE2EFile(t, filepath.Join(root, "thing.tres"), resourceTres)

		// A .tres loads as a Resource, not a PackedScene, so read_scene reports
		// a controlled failure instead of a degraded raw-text success; a caller
		// reads a text .tres directly and binary .res with read_binary_resource.
		res := callTool(t, session, "read_scene", map[string]any{
			"scenePath": "thing.tres",
		})
		if !res.IsError {
			t.Fatal("expected an error for a non-scene resource")
		}
		requirePrefix(t, firstText(t, res), "Failed to read scene:")
		requireContains(t, firstText(t, res), "Not a scene file")
	})
}

// TestReadSceneNestedProperties drives the real read_scene tool against a node
// whose exported Dictionary property exercises the hybrid value encoding:
// JSON-native types stay nested, engine-only types stay strings.
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

// TestReadSceneGroups exercises the node-group branch of the scene walker. A
// hand-written .tscn is used because no tool exposes group membership, and the
// engine persists groups even when the scene is instantiated outside a tree.
func TestReadSceneGroups(t *testing.T) {
	t.Parallel()
	root := newProject(t)
	session := connectProject(t, root)

	writeE2EFile(t, filepath.Join(root, "scenes", "groups.tscn"), `[gd_scene format=3]

[node name="root" type="Node2D" groups=["enemies"]]

[node name="Child" type="Node2D" parent="." groups=["pickups", "enemies"]]
`)

	text := requireToolText(t, session, "read_scene", map[string]any{
		"scenePath": "scenes/groups.tscn",
	})
	var tree sceneNode
	if err := json.Unmarshal([]byte(text), &tree); err != nil {
		t.Fatalf("unmarshal %q: %v", text, err)
	}
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

// TestReadSceneErrors covers the reference's validation and failure messages.
func TestReadSceneErrors(t *testing.T) {
	t.Parallel()
	// Every project-backed case shares one warm project and session; only "not a
	// project" needs its own non-project directory.
	root := newProject(t)
	session := connectProject(t, root)

	t.Run("missing paths", func(t *testing.T) {
		requireToolError(t, session, "read_scene", map[string]any{}, "scenePath is required.")
	})

	t.Run("not a project", func(t *testing.T) {
		notProject := t.TempDir()
		notProjectSession := connectProject(t, notProject)
		requireToolError(t, notProjectSession, "read_scene", map[string]any{
			"scenePath": "main.tscn",
		}, "Not a valid Godot project: "+notProject)
	})

	t.Run("missing scene", func(t *testing.T) {
		requireToolError(t, session, "read_scene", map[string]any{
			"scenePath": "missing.tscn",
		}, "Scene file does not exist: missing.tscn")
	})

	t.Run("negative depth", func(t *testing.T) {
		requireToolError(t, session, "read_scene", map[string]any{
			"scenePath": "main.tscn",
			"depth":     -1,
		}, "depth must be zero or greater.")
	})
}
