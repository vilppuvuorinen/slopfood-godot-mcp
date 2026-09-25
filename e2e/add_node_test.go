package e2e

import (
	"path/filepath"
	"testing"
)

// TestAddNodeSchema ports the reference's tool-definition expectations for
// add_node over the real stdio surface.
func TestAddNodeSchema(t *testing.T) {
	schema := requireSchema(t, "add_node", "Add a node to an existing scene")
	schema.requireRequired(t, "projectPath", "scenePath", "nodeType", "nodeName")
	schema.requireProps(t, "projectPath", "scenePath", "parentNodePath", "nodeType", "nodeName", "properties")
	schema.requireType(t, "properties", "object")
}

// TestAddNodeTool drives the real add_node tool against Godot.
func TestAddNodeTool(t *testing.T) {
	session := connect(t)
	requireGodot(t, session)

	t.Run("adds a node", func(t *testing.T) {
		root := seedScene(t, session, "scenes/main.tscn")

		text := requireToolText(t, session, "add_node", map[string]any{
			"projectPath": root,
			"scenePath":   "scenes/main.tscn",
			"nodeType":    "Sprite2D",
			"nodeName":    "Player",
		})
		requirePrefix(t, text, "Node 'Player' of type 'Sprite2D' added successfully to 'scenes/main.tscn'.")

		scene := readFile(t, filepath.Join(root, "scenes", "main.tscn"))
		requireContains(t, scene, `[node name="Player" type="Sprite2D"`)
	})
}

// TestAddNodeErrors covers the reference's validation and failure messages.
func TestAddNodeErrors(t *testing.T) {
	session := connect(t)

	t.Run("missing params", func(t *testing.T) {
		requireToolError(t, session, "add_node", map[string]any{}, "Missing required parameters")
	})

	t.Run("invalid path", func(t *testing.T) {
		requireToolError(t, session, "add_node", map[string]any{
			"projectPath": "../escape",
			"scenePath":   "main.tscn",
			"nodeType":    "Sprite2D",
			"nodeName":    "Player",
		}, "Invalid path")
	})

	t.Run("invalid nodeType", func(t *testing.T) {
		requireToolError(t, session, "add_node", map[string]any{
			"projectPath": t.TempDir(),
			"scenePath":   "main.tscn",
			"nodeType":    "res://evil.gd",
			"nodeName":    "Player",
		}, "Invalid nodeType")
	})

	t.Run("not a project", func(t *testing.T) {
		root := t.TempDir()
		requireToolError(t, session, "add_node", map[string]any{
			"projectPath": root,
			"scenePath":   "main.tscn",
			"nodeType":    "Sprite2D",
			"nodeName":    "Player",
		}, "Not a valid Godot project: "+root)
	})

	t.Run("missing scene", func(t *testing.T) {
		root := t.TempDir()
		seedProject(t, root)
		requireToolError(t, session, "add_node", map[string]any{
			"projectPath": root,
			"scenePath":   "missing.tscn",
			"nodeType":    "Sprite2D",
			"nodeName":    "Player",
		}, "Scene file does not exist: missing.tscn")
	})
}
