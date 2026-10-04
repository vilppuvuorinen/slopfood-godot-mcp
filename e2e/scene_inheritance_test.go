package e2e

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type inheritedScene struct {
	Name          string `json:"name"`
	Type          string `json:"type"`
	BaseScenePath string `json:"baseScenePath"`
	Children      []struct {
		Name string `json:"name"`
		Type string `json:"type"`
	} `json:"children"`
}

const inheritedBaseScene = `[gd_scene format=3]

[node name="root" type="Node2D"]

[node name="Icon" type="Sprite2D" parent="."]
`

const inheritedDerivedScene = `[gd_scene load_steps=2 format=3]

[ext_resource type="PackedScene" path="res://scenes/base.tscn" id="1"]

[node name="root" instance=ExtResource("1")]
`

func seedInheritedScene(t *testing.T, derivedPath string) (string, *mcp.ClientSession) {
	t.Helper()
	root := newProject(t)
	session := connectProject(t, root)

	writeE2EFile(t, filepath.Join(root, "scenes", "base.tscn"), inheritedBaseScene)
	writeE2EFile(t, filepath.Join(root, derivedPath), inheritedDerivedScene)
	return root, session
}

func TestSceneInheritanceTool(t *testing.T) {
	t.Parallel()
	root, session := seedInheritedScene(t, "scenes/derived.tscn")

	derived := readFile(t, filepath.Join(root, "scenes", "derived.tscn"))
	requireContains(t, derived,
		`[ext_resource type="PackedScene" path="res://scenes/base.tscn"`,
		`instance=ExtResource(`,
	)

	text := requireToolText(t, session, "read", map[string]any{
		"type": "scene",
		"path": "scenes/derived.tscn",
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

func TestInheritedSceneEditsRejected(t *testing.T) {
	t.Parallel()
	root, session := seedInheritedScene(t, "scenes/derived.tscn")
	derivedFull := filepath.Join(root, "scenes", "derived.tscn")
	before := readFile(t, derivedFull)

	writeE2EFile(t, filepath.Join(root, "scripts", "icon.gd"), "extends Sprite2D\n")

	calls := []struct {
		name string
		args map[string]any
	}{
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

	// read(type=scene) is not caught by the guard; it instantiates the base subtree.
	requireContains(t, requireToolText(t, session, "read", map[string]any{
		"type": "scene",
		"path": "scenes/derived.tscn",
	}), `"connections"`)
}
