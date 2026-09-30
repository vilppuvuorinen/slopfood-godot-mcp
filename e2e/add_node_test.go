package e2e

import (
	"path/filepath"
	"regexp"
	"testing"
)

// TestAddNodeTool drives the real add_node tool against Godot.
func TestAddNodeTool(t *testing.T) {
	t.Parallel()
	// Both subtests share one warm project and session, each with its own scene.
	root := newProject(t)
	session := connectProject(t, root)

	t.Run("adds a node", func(t *testing.T) {
		const path = "scenes/add_main.tscn"
		createScene(t, session, path)

		text := requireToolText(t, session, "add_node", map[string]any{
			"scenePath": path,
			"nodeType":  "Sprite2D",
			"nodeName":  "Player",
		})
		requirePrefix(t, text, "Node 'Player' of type 'Sprite2D' added successfully to '"+path+"'.")

		scene := readFile(t, filepath.Join(root, "scenes", "add_main.tscn"))
		requireContains(t, scene, `[node name="Player" type="Sprite2D"`)
	})

	t.Run("adds a scene instance", func(t *testing.T) {
		const path = "scenes/instance_main.tscn"
		createScene(t, session, path)
		res := callTool(t, session, "create_scene", map[string]any{"scenePath": "scenes/enemy.tscn"})
		if res.IsError {
			t.Fatalf("create_scene failed: %s", firstText(t, res))
		}

		text := requireToolText(t, session, "add_node", map[string]any{
			"scenePath":    path,
			"instancePath": "scenes/enemy.tscn",
			"nodeName":     "Foe",
		})
		requirePrefix(t, text, "Scene 'scenes/enemy.tscn' instanced as 'Foe' added successfully to '"+path+"'.")

		scene := readFile(t, filepath.Join(root, "scenes", "instance_main.tscn"))
		requireContains(t, scene, `instance=ExtResource(`)
		// The editor records the instance's UID on the ext_resource line, so the
		// path is not adjacent to the type; match the reference with the uid
		// optional rather than pinning attribute order.
		extResource := regexp.MustCompile(`\[ext_resource type="PackedScene"( uid="uid://[^"]+")? path="res://scenes/enemy\.tscn"`)
		if !extResource.MatchString(scene) {
			t.Errorf("missing a PackedScene ext_resource for enemy.tscn in:\n%s", scene)
		}
	})
}

// TestAddNodeErrors covers the reference's validation and failure messages.
func TestAddNodeErrors(t *testing.T) {
	t.Parallel()
	// Every case that touches a project shares one warm session; only the
	// "not a project" case, which needs a non-project directory, dials its own.
	root := newProject(t)
	session := connectProject(t, root)

	t.Run("missing params", func(t *testing.T) {
		requireToolError(t, session, "add_node", map[string]any{}, "scenePath and nodeName are required.")
	})

	t.Run("both sources", func(t *testing.T) {
		createScene(t, session, "scenes/both.tscn")
		res := callTool(t, session, "create_scene", map[string]any{"scenePath": "scenes/enemy.tscn"})
		if res.IsError {
			t.Fatalf("create_scene failed: %s", firstText(t, res))
		}
		requireToolError(t, session, "add_node", map[string]any{
			"scenePath":    "scenes/both.tscn",
			"nodeType":     "Sprite2D",
			"instancePath": "scenes/enemy.tscn",
			"nodeName":     "Player",
		}, "Provide exactly one of nodeType or instancePath.")
	})

	t.Run("neither source", func(t *testing.T) {
		createScene(t, session, "scenes/neither.tscn")
		requireToolError(t, session, "add_node", map[string]any{
			"scenePath": "scenes/neither.tscn",
			"nodeName":  "Player",
		}, "Provide exactly one of nodeType or instancePath.")
	})

	t.Run("missing instance scene", func(t *testing.T) {
		createScene(t, session, "scenes/missing_instance.tscn")
		requireToolError(t, session, "add_node", map[string]any{
			"scenePath":    "scenes/missing_instance.tscn",
			"instancePath": "scenes/missing.tscn",
			"nodeName":     "Foe",
		}, "Instance scene does not exist: scenes/missing.tscn")
	})

	t.Run("invalid path", func(t *testing.T) {
		requireToolError(t, session, "add_node", map[string]any{
			"scenePath": "../escape",
			"nodeType":  "Sprite2D",
			"nodeName":  "Player",
		}, "Invalid path.")
	})

	t.Run("invalid nodeType", func(t *testing.T) {
		requireToolError(t, session, "add_node", map[string]any{
			"scenePath": "main.tscn",
			"nodeType":  "res://evil.gd",
			"nodeName":  "Player",
		}, "Invalid nodeType")
	})

	t.Run("not a project", func(t *testing.T) {
		notProject := t.TempDir()
		notProjectSession := connectProject(t, notProject)
		requireToolError(t, notProjectSession, "add_node", map[string]any{
			"scenePath": "main.tscn",
			"nodeType":  "Sprite2D",
			"nodeName":  "Player",
		}, "Not a valid Godot project: "+notProject)
	})

	t.Run("missing scene", func(t *testing.T) {
		requireToolError(t, session, "add_node", map[string]any{
			"scenePath": "missing.tscn",
			"nodeType":  "Sprite2D",
			"nodeName":  "Player",
		}, "Scene file does not exist: missing.tscn")
	})

	t.Run("missing parent node", func(t *testing.T) {
		createScene(t, session, "scenes/missing_parent.tscn", "Node2D")

		res := callTool(t, session, "add_node", map[string]any{
			"scenePath":      "scenes/missing_parent.tscn",
			"parentNodePath": "root/Missing",
			"nodeType":       "Sprite2D",
			"nodeName":       "Player",
		})
		if !res.IsError {
			t.Fatal("add_node: expected an error result")
		}
		requireContains(t, firstText(t, res), "Failed to add node", "Parent node not found: root/Missing")
	})

	t.Run("bogus node type", func(t *testing.T) {
		createScene(t, session, "scenes/bogus.tscn", "Node2D")

		// validClassName accepts any identifier, so an unregistered class
		// reaches the engine and fails there rather than being pre-rejected.
		res := callTool(t, session, "add_node", map[string]any{
			"scenePath": "scenes/bogus.tscn",
			"nodeType":  "NotARealNode",
			"nodeName":  "Player",
		})
		if !res.IsError {
			t.Fatal("add_node: expected an error result")
		}
		requireContains(t, firstText(t, res), "Failed to add node", "Failed to instantiate node of type: NotARealNode")
	})

	t.Run("abstract node type", func(t *testing.T) {
		createScene(t, session, "scenes/abstract.tscn", "Node2D")

		// CanvasItem is a registered ClassDB class Godot refuses to
		// instantiate, taking the can_instantiate branch rather than the
		// script-lookup one.
		res := callTool(t, session, "add_node", map[string]any{
			"scenePath": "scenes/abstract.tscn",
			"nodeType":  "CanvasItem",
			"nodeName":  "Player",
		})
		if !res.IsError {
			t.Fatal("add_node: expected an error result")
		}
		requireContains(t, firstText(t, res),
			"Failed to add node",
			"Class exists but cannot be instantiated: CanvasItem",
			"Failed to instantiate node of type: CanvasItem",
		)
	})
}
