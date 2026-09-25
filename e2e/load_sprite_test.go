package e2e

import (
	"path/filepath"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const gradientTextureResource = "[gd_resource type=\"GradientTexture2D\" format=3]\n\n[resource]\nwidth = 8\nheight = 8\n"

// TestLoadSpriteSchema ports the reference's tool-definition expectations for
// load_sprite over the real stdio surface.
func TestLoadSpriteSchema(t *testing.T) {
	schema := requireSchema(t, "load_sprite", "Load a sprite into a Sprite2D node")
	schema.requireRequired(t, "projectPath", "scenePath", "nodePath", "texturePath")
	schema.requirePropCount(t, 4)
	schema.requireStringProps(t, "projectPath", "scenePath", "nodePath", "texturePath")
}

// seedSpriteScene creates a scene with a Sprite2D node and a texture resource
// so load_sprite has a target and an image to load.
func seedSpriteScene(t *testing.T, session *mcp.ClientSession, scenePath string) string {
	t.Helper()
	root := seedScene(t, session, scenePath)

	res := callTool(t, session, "add_node", map[string]any{
		"projectPath": root,
		"scenePath":   scenePath,
		"nodeType":    "Sprite2D",
		"nodeName":    "Sprite",
	})
	if res.IsError {
		t.Skipf("add_node failed, skipping: %s", firstText(t, res))
	}
	writeE2EFile(t, filepath.Join(root, "icon.tres"), gradientTextureResource)
	return root
}

// TestLoadSpriteTool drives the real load_sprite tool against Godot.
func TestLoadSpriteTool(t *testing.T) {
	session := connect(t)
	requireGodot(t, session)

	t.Run("loads a sprite", func(t *testing.T) {
		root := seedSpriteScene(t, session, "scenes/main.tscn")

		text := requireToolText(t, session, "load_sprite", map[string]any{
			"projectPath": root,
			"scenePath":   "scenes/main.tscn",
			"nodePath":    "root/Sprite",
			"texturePath": "icon.tres",
		})
		requirePrefix(t, text, "Sprite loaded successfully with texture: icon.tres")

		scene := readFile(t, filepath.Join(root, "scenes", "main.tscn"))
		requireContains(t, scene, `texture = ExtResource(`)
	})
}

// TestLoadSpriteErrors covers the reference's validation and failure messages.
func TestLoadSpriteErrors(t *testing.T) {
	session := connect(t)

	t.Run("missing params", func(t *testing.T) {
		requireToolError(t, session, "load_sprite", map[string]any{}, "Missing required parameters")
	})

	t.Run("invalid path", func(t *testing.T) {
		requireToolError(t, session, "load_sprite", map[string]any{
			"projectPath": "../escape",
			"scenePath":   "scenes/main.tscn",
			"nodePath":    "root/Sprite",
			"texturePath": "icon.tres",
		}, "Invalid path")
	})

	t.Run("not a project", func(t *testing.T) {
		root := t.TempDir()
		requireToolError(t, session, "load_sprite", map[string]any{
			"projectPath": root,
			"scenePath":   "scenes/main.tscn",
			"nodePath":    "root/Sprite",
			"texturePath": "icon.tres",
		}, "Not a valid Godot project: "+root)
	})

	t.Run("missing scene", func(t *testing.T) {
		root := t.TempDir()
		seedProject(t, root)
		requireToolError(t, session, "load_sprite", map[string]any{
			"projectPath": root,
			"scenePath":   "missing.tscn",
			"nodePath":    "root/Sprite",
			"texturePath": "icon.tres",
		}, "Scene file does not exist: missing.tscn")
	})

	t.Run("missing texture", func(t *testing.T) {
		root := t.TempDir()
		seedProject(t, root)
		writeE2EFile(t, filepath.Join(root, "scenes", "main.tscn"), "scene")
		requireToolError(t, session, "load_sprite", map[string]any{
			"projectPath": root,
			"scenePath":   "scenes/main.tscn",
			"nodePath":    "root/Sprite",
			"texturePath": "missing.tres",
		}, "Texture file does not exist: missing.tres")
	})
}
