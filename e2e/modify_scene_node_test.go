package e2e

import (
	"path/filepath"
	"testing"
)

// TestModifySceneNodeSchema ports the reference's tool-definition expectations
// for modify_scene_node over the real stdio surface.
func TestModifySceneNodeSchema(t *testing.T) {
	t.Parallel()
	schema := requireSchema(t, "modify_scene_node", "Modify node properties in a scene file (headless)")
	schema.requireRequired(t, "scenePath", "nodePath", "properties")
	schema.requireStringProps(t, "scenePath", "nodePath")
	schema.requireType(t, "properties", "object")
	schema.requirePropCount(t, 3)
}

// TestModifySceneNodeTool drives the real modify_scene_node tool against Godot
// and reads the saved scene back from disk.
func TestModifySceneNodeTool(t *testing.T) {
	t.Parallel()
	root, session := seedScene(t, "scenes/main.tscn", "Node2D")

	requireToolText(t, session, "add_node", map[string]any{
		"scenePath": "scenes/main.tscn",
		"nodeType":  "Sprite2D",
		"nodeName":  "Player",
	})

	requireToolText(t, session, "modify_scene_node", map[string]any{
		"scenePath": "scenes/main.tscn",
		"nodePath":  "root/Player",
		"properties": map[string]any{
			"position": map[string]any{"$type": "Vector2", "x": 10, "y": 20},
			"visible":  false,
		},
	})

	scene := readFile(t, filepath.Join(root, "scenes", "main.tscn"))
	requireContains(t, scene, "position = Vector2(10, 20)", "visible = false")
}

// TestModifySceneNodeDecodeErrors pins the value-contract decoder's rejection
// messages for values it cannot turn into the type a property expects. Each
// case must fail with the specific reason and, because _apply_properties
// refuses before writing, leave the scene file untouched.
func TestModifySceneNodeDecodeErrors(t *testing.T) {
	t.Parallel()
	root, session := seedScene(t, "scenes/errors.tscn", "Node2D")
	requireToolText(t, session, "add_node", map[string]any{
		"scenePath": "scenes/errors.tscn",
		"nodeType":  "Sprite2D",
		"nodeName":  "Player",
	})
	requireToolText(t, session, "add_node", map[string]any{
		"scenePath": "scenes/errors.tscn",
		"nodeType":  "AnimationPlayer",
		"nodeName":  "Anim",
	})
	before := readFile(t, filepath.Join(root, "scenes", "errors.tscn"))

	cases := []struct {
		name  string
		node  string
		props map[string]any
		want  string
	}{
		{
			name:  "unknown tag",
			node:  "root/Player",
			props: map[string]any{"position": map[string]any{"$type": "BogusTag", "x": 1, "y": 1}},
			want:  "unknown $type: BogusTag",
		},
		{
			name:  "missing number field",
			node:  "root/Player",
			props: map[string]any{"position": map[string]any{"$type": "Vector2", "x": 1}},
			want:  `expected a number for "y" in $type Vector2`,
		},
		{
			name:  "non-number field",
			node:  "root/Player",
			props: map[string]any{"position": map[string]any{"$type": "Vector2", "x": "a", "y": 1}},
			want:  `expected a number for "x" in $type Vector2`,
		},
		{
			name:  "expected boolean",
			node:  "root/Player",
			props: map[string]any{"visible": "nope"},
			want:  "expected a boolean",
		},
		{
			name:  "expected integer",
			node:  "root/Player",
			props: map[string]any{"z_index": "nope"},
			want:  "expected an integer",
		},
		{
			name:  "expected string",
			node:  "root/Player",
			props: map[string]any{"editor_description": 5},
			want:  "expected a string",
		},
		{
			name:  "expected node path",
			node:  "root/Anim",
			props: map[string]any{"root_node": 5},
			want:  "expected a string path",
		},
		{
			name:  "object needs a resource path",
			node:  "root/Player",
			props: map[string]any{"texture": "notaresource"},
			want:  "expected a res:// path or a tagged value for an object property",
		},
		{
			name:  "resource load failure",
			node:  "root/Player",
			props: map[string]any{"texture": map[string]any{"$type": "Resource", "path": "res://missing.tres"}},
			want:  "failed to load resource: res://missing.tres",
		},
		{
			name:  "nested unknown tag",
			node:  "root/Player",
			props: map[string]any{"config": map[string]any{"inner": map[string]any{"$type": "BogusTag"}}},
			want:  "unknown $type: BogusTag",
		},
		{
			name:  "packed string array needs values",
			node:  "root/Player",
			props: map[string]any{"config": map[string]any{"a": map[string]any{"$type": "PackedStringArray"}}},
			want:  `$type PackedStringArray needs a "values" array`,
		},
		{
			name:  "packed string array values must be strings",
			node:  "root/Player",
			props: map[string]any{"config": map[string]any{"a": map[string]any{"$type": "PackedStringArray", "values": []any{1}}}},
			want:  "PackedStringArray values must be strings",
		},
		{
			name:  "dictionary needs a value",
			node:  "root/Player",
			props: map[string]any{"config": map[string]any{"a": map[string]any{"$type": "Dictionary"}}},
			want:  `$type Dictionary needs a "value" object`,
		},
		{
			name:  "resource needs a path",
			node:  "root/Player",
			props: map[string]any{"config": map[string]any{"a": map[string]any{"$type": "Resource"}}},
			want:  `$type Resource needs a "path" string`,
		},
		{
			name: "vector field wrong type",
			node: "root/Player",
			props: map[string]any{"region_rect": map[string]any{
				"$type":    "Rect2",
				"position": map[string]any{"$type": "Vector3", "x": 0, "y": 0, "z": 0},
				"size":     map[string]any{"$type": "Vector2", "x": 1, "y": 1},
			}},
			want: `field "position" has the wrong vector type`,
		},
		{
			name: "vector field not tagged",
			node: "root/Player",
			props: map[string]any{"region_rect": map[string]any{
				"$type":    "Rect2",
				"position": 5,
				"size":     map[string]any{"$type": "Vector2", "x": 1, "y": 1},
			}},
			want: `field "position" must be a tagged vector value`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res := callTool(t, session, "modify_scene_node", map[string]any{
				"scenePath":  "scenes/errors.tscn",
				"nodePath":   tc.node,
				"properties": tc.props,
			})
			if !res.IsError {
				t.Fatalf("expected an error result, got: %s", firstText(t, res))
			}
			requireContains(t, firstText(t, res), "Invalid property value: "+tc.want)
		})
	}

	if after := readFile(t, filepath.Join(root, "scenes", "errors.tscn")); after != before {
		t.Errorf("a rejected property changed the scene:\nbefore:\n%s\nafter:\n%s", before, after)
	}
}

// TestModifySceneNodeErrors covers the reference's validation and failure
// messages.
func TestModifySceneNodeErrors(t *testing.T) {
	t.Parallel()
	t.Run("missing scene", func(t *testing.T) {
		session := connectProject(t, newProject(t))
		requireToolError(t, session, "modify_scene_node", map[string]any{
			"scenePath":  "missing.tscn",
			"nodePath":   "root",
			"properties": map[string]any{"visible": true},
		}, "Scene file does not exist: missing.tscn")
	})

	t.Run("not a project", func(t *testing.T) {
		root := t.TempDir()
		session := connectProject(t, root)
		requireToolError(t, session, "modify_scene_node", map[string]any{
			"scenePath":  "main.tscn",
			"nodePath":   "root",
			"properties": map[string]any{"visible": true},
		}, "Not a valid Godot project: "+root)
	})

	// The two engine-backed cases share one warm session, each with its own scene.
	root := newProject(t)
	session := connectProject(t, root)

	t.Run("missing node", func(t *testing.T) {
		const path = "scenes/missing_node.tscn"
		createScene(t, session, path, "Node2D")

		res := callTool(t, session, "modify_scene_node", map[string]any{
			"scenePath":  path,
			"nodePath":   "root/Missing",
			"properties": map[string]any{"visible": true},
		})
		if !res.IsError {
			t.Fatal("modify_scene_node: expected an error result")
		}
		requireContains(t, firstText(t, res), "modify_node failed", "Node not found: root/Missing")
	})

	t.Run("invalid property value", func(t *testing.T) {
		const path = "scenes/invalid_property.tscn"
		createScene(t, session, path, "Node2D")

		// position is a Vector2 slot, so a bare string cannot be decoded.
		res := callTool(t, session, "modify_scene_node", map[string]any{
			"scenePath":  path,
			"nodePath":   "root",
			"properties": map[string]any{"position": "not-a-vector"},
		})
		if !res.IsError {
			t.Fatal("modify_scene_node: expected an error result")
		}
		requireContains(t, firstText(t, res), "modify_node failed", "Invalid property value: expected a tagged")
	})
}
