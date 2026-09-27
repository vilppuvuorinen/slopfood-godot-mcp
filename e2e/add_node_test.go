package e2e

import (
	"path/filepath"
	"testing"
)

// TestAddNodeSchema ports the reference's tool-definition expectations for
// add_node over the real stdio surface.
func TestAddNodeSchema(t *testing.T) {
	t.Parallel()
	schema := requireSchema(t, "add_node", "Add a built-in node or a scene instance to an existing scene (rewrites the .tscn file)")
	schema.requireRequired(t, "scenePath", "nodeName")
	schema.requireProps(t, "scenePath", "parentNodePath", "nodeType", "instancePath", "nodeName", "properties")
	schema.requireType(t, "properties", "object")
}

// TestAddNodeTool drives the real add_node tool against Godot.
func TestAddNodeTool(t *testing.T) {
	t.Parallel()
	t.Run("adds a node", func(t *testing.T) {
		root, session := seedScene(t, "scenes/main.tscn")

		text := requireToolText(t, session, "add_node", map[string]any{
			"scenePath": "scenes/main.tscn",
			"nodeType":  "Sprite2D",
			"nodeName":  "Player",
		})
		requirePrefix(t, text, "Node 'Player' of type 'Sprite2D' added successfully to 'scenes/main.tscn'.")

		scene := readFile(t, filepath.Join(root, "scenes", "main.tscn"))
		requireContains(t, scene, `[node name="Player" type="Sprite2D"`)
	})

	t.Run("adds a scene instance", func(t *testing.T) {
		root, session := seedScene(t, "scenes/main.tscn")
		res := callTool(t, session, "create_scene", map[string]any{"scenePath": "scenes/enemy.tscn"})
		if res.IsError {
			t.Fatalf("create_scene failed: %s", firstText(t, res))
		}

		text := requireToolText(t, session, "add_node", map[string]any{
			"scenePath":    "scenes/main.tscn",
			"instancePath": "scenes/enemy.tscn",
			"nodeName":     "Foe",
		})
		requirePrefix(t, text, "Scene 'scenes/enemy.tscn' instanced as 'Foe' added successfully to 'scenes/main.tscn'.")

		scene := readFile(t, filepath.Join(root, "scenes", "main.tscn"))
		requireContains(t, scene,
			`[ext_resource type="PackedScene" path="res://scenes/enemy.tscn"`,
			`instance=ExtResource(`,
		)
	})
}

// TestAddNodeErrors covers the reference's validation and failure messages.
func TestAddNodeErrors(t *testing.T) {
	t.Parallel()
	t.Run("missing params", func(t *testing.T) {
		root := newProject(t)
		session := connectProject(t, root)
		requireToolError(t, session, "add_node", map[string]any{}, "scenePath and nodeName are required.")
	})

	t.Run("both sources", func(t *testing.T) {
		_, session := seedScene(t, "scenes/main.tscn")
		res := callTool(t, session, "create_scene", map[string]any{"scenePath": "scenes/enemy.tscn"})
		if res.IsError {
			t.Fatalf("create_scene failed: %s", firstText(t, res))
		}
		requireToolError(t, session, "add_node", map[string]any{
			"scenePath":    "scenes/main.tscn",
			"nodeType":     "Sprite2D",
			"instancePath": "scenes/enemy.tscn",
			"nodeName":     "Player",
		}, "Provide exactly one of nodeType or instancePath.")
	})

	t.Run("neither source", func(t *testing.T) {
		_, session := seedScene(t, "scenes/main.tscn")
		requireToolError(t, session, "add_node", map[string]any{
			"scenePath": "scenes/main.tscn",
			"nodeName":  "Player",
		}, "Provide exactly one of nodeType or instancePath.")
	})

	t.Run("missing instance scene", func(t *testing.T) {
		_, session := seedScene(t, "scenes/main.tscn")
		requireToolError(t, session, "add_node", map[string]any{
			"scenePath":    "scenes/main.tscn",
			"instancePath": "scenes/missing.tscn",
			"nodeName":     "Foe",
		}, "Instance scene does not exist: scenes/missing.tscn")
	})

	t.Run("invalid path", func(t *testing.T) {
		root := newProject(t)
		session := connectProject(t, root)
		requireToolError(t, session, "add_node", map[string]any{
			"scenePath": "../escape",
			"nodeType":  "Sprite2D",
			"nodeName":  "Player",
		}, "Invalid path.")
	})

	t.Run("invalid nodeType", func(t *testing.T) {
		root := newProject(t)
		session := connectProject(t, root)
		requireToolError(t, session, "add_node", map[string]any{
			"scenePath": "main.tscn",
			"nodeType":  "res://evil.gd",
			"nodeName":  "Player",
		}, "Invalid nodeType")
	})

	t.Run("not a project", func(t *testing.T) {
		root := t.TempDir()
		session := connectProject(t, root)
		requireToolError(t, session, "add_node", map[string]any{
			"scenePath": "main.tscn",
			"nodeType":  "Sprite2D",
			"nodeName":  "Player",
		}, "Not a valid Godot project: "+root)
	})

	t.Run("missing scene", func(t *testing.T) {
		root := newProject(t)
		session := connectProject(t, root)
		requireToolError(t, session, "add_node", map[string]any{
			"scenePath": "missing.tscn",
			"nodeType":  "Sprite2D",
			"nodeName":  "Player",
		}, "Scene file does not exist: missing.tscn")
	})

	t.Run("missing parent node", func(t *testing.T) {
		_, session := seedScene(t, "scenes/main.tscn", "Node2D")

		res := callTool(t, session, "add_node", map[string]any{
			"scenePath":      "scenes/main.tscn",
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
		_, session := seedScene(t, "scenes/main.tscn", "Node2D")

		// validClassName accepts any identifier, so an unregistered class
		// reaches the engine and fails there rather than being pre-rejected.
		res := callTool(t, session, "add_node", map[string]any{
			"scenePath": "scenes/main.tscn",
			"nodeType":  "NotARealNode",
			"nodeName":  "Player",
		})
		if !res.IsError {
			t.Fatal("add_node: expected an error result")
		}
		requireContains(t, firstText(t, res), "Failed to add node", "Failed to instantiate node of type: NotARealNode")
	})

	t.Run("abstract node type", func(t *testing.T) {
		_, session := seedScene(t, "scenes/main.tscn", "Node2D")

		// CanvasItem is a registered ClassDB class Godot refuses to
		// instantiate, taking the can_instantiate branch rather than the
		// script-lookup one.
		res := callTool(t, session, "add_node", map[string]any{
			"scenePath": "scenes/main.tscn",
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
