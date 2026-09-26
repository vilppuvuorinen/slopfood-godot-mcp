package e2e

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// inheritedScene is the subset of a read_scene result the inheritance tests
// assert on.
type inheritedScene struct {
	Name          string `json:"name"`
	Type          string `json:"type"`
	BaseScenePath string `json:"baseScenePath"`
	Children      []struct {
		Name string `json:"name"`
		Type string `json:"type"`
	} `json:"children"`
}

// seedInheritedScene creates a base scene with one child, then a scene that
// inherits from it. It returns the project root, the connected session, and the
// derived scene's project-relative path.
func seedInheritedScene(t *testing.T, derivedPath string) (string, *mcp.ClientSession) {
	t.Helper()
	root := newProject(t)
	session := connectProject(t, root)
	requireGodot(t, session)

	requireToolText(t, session, "create_scene", map[string]any{
		"scenePath":    "scenes/base.tscn",
		"rootNodeType": "Node2D",
	})
	requireToolText(t, session, "add_node", map[string]any{
		"scenePath": "scenes/base.tscn",
		"nodeType":  "Sprite2D",
		"nodeName":  "Icon",
	})
	requireToolText(t, session, "create_scene", map[string]any{
		"scenePath":     derivedPath,
		"baseScenePath": "scenes/base.tscn",
	})
	return root, session
}

// TestSceneInheritanceTool drives create_scene with baseScenePath and read_scene
// over the resulting inherited scene.
func TestSceneInheritanceTool(t *testing.T) {
	root, session := seedInheritedScene(t, "scenes/derived.tscn")

	derived := readFile(t, filepath.Join(root, "scenes", "derived.tscn"))
	requireContains(t, derived,
		`[ext_resource type="PackedScene" path="res://scenes/base.tscn"`,
		`instance=ExtResource(`,
	)

	text := requireToolText(t, session, "read_scene", map[string]any{
		"scenePath": "scenes/derived.tscn",
	})
	var tree inheritedScene
	if err := json.Unmarshal([]byte(text), &tree); err != nil {
		t.Fatalf("unmarshal %q: %v", text, err)
	}
	if tree.BaseScenePath != "scenes/base.tscn" {
		t.Errorf("baseScenePath = %q, want scenes/base.tscn", tree.BaseScenePath)
	}
	if tree.Name != "root" || tree.Type != "Node2D" {
		t.Errorf("root = %+v", tree)
	}
	if len(tree.Children) != 1 || tree.Children[0].Name != "Icon" {
		t.Errorf("children = %+v, want the inherited Icon", tree.Children)
	}
}

// TestInheritedSceneEditsRejected proves every repacking scene mutator refuses
// an inherited scene instead of silently flattening the base into it.
func TestInheritedSceneEditsRejected(t *testing.T) {
	root, session := seedInheritedScene(t, "scenes/derived.tscn")
	derivedFull := filepath.Join(root, "scenes", "derived.tscn")
	before := readFile(t, derivedFull)

	writeE2EFile(t, filepath.Join(root, "scripts", "icon.gd"), "extends Sprite2D\n")

	calls := []struct {
		name string
		args map[string]any
	}{
		{"add_node", map[string]any{
			"scenePath": "scenes/derived.tscn",
			"nodeType":  "Camera2D",
			"nodeName":  "Cam",
		}},
		{"modify_scene_node", map[string]any{
			"scenePath":  "scenes/derived.tscn",
			"nodePath":   "root",
			"properties": map[string]any{"position": map[string]any{"$type": "Vector2", "x": 1, "y": 2}},
		}},
		{"remove_scene_node", map[string]any{
			"scenePath": "scenes/derived.tscn",
			"nodePath":  "root/Icon",
		}},
		{"rename_scene_node", map[string]any{
			"scenePath": "scenes/derived.tscn",
			"nodePath":  "root/Icon",
			"newName":   "Renamed",
		}},
		{"attach_script", map[string]any{
			"scenePath":  "scenes/derived.tscn",
			"nodePath":   "root/Icon",
			"scriptPath": "scripts/icon.gd",
		}},
	}
	for _, call := range calls {
		res := callTool(t, session, call.name, call.args)
		if !res.IsError {
			t.Errorf("%s: expected an error for an inherited scene", call.name)
			continue
		}
		requireContains(t, firstText(t, res), "inherited scene")
	}

	if got := readFile(t, derivedFull); got != before {
		t.Errorf("derived scene changed:\nbefore:\n%s\nafter:\n%s", before, got)
	}

	// Reading an inherited scene stays available: it instantiates the base
	// subtree and reports its own connections, so it is not caught by the guard.
	requireContains(t, requireToolText(t, session, "read_scene", map[string]any{
		"scenePath": "scenes/derived.tscn",
	}), `"connections"`)
}

// TestCreateSceneInheritanceErrors covers baseScenePath validation.
func TestCreateSceneInheritanceErrors(t *testing.T) {
	t.Run("both root sources", func(t *testing.T) {
		session := connectProject(t, newProject(t))
		requireToolError(t, session, "create_scene", map[string]any{
			"scenePath":     "derived.tscn",
			"rootNodeType":  "Node2D",
			"baseScenePath": "base.tscn",
		}, "Provide either rootNodeType or baseScenePath, not both.")
	})

	t.Run("missing base scene", func(t *testing.T) {
		session := connectProject(t, newProject(t))
		requireToolError(t, session, "create_scene", map[string]any{
			"scenePath":     "derived.tscn",
			"baseScenePath": "scenes/missing.tscn",
		}, "Base scene does not exist: scenes/missing.tscn")
	})

	t.Run("invalid base path", func(t *testing.T) {
		session := connectProject(t, newProject(t))
		requireToolError(t, session, "create_scene", map[string]any{
			"scenePath":     "derived.tscn",
			"baseScenePath": "../escape.tscn",
		}, "Invalid path.")
	})
}
