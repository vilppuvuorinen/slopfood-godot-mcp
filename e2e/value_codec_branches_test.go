package e2e

import (
	"encoding/json"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Bare scalars coerced to a declared type, plus composite tags exercising the typed-zero fallback.
const valueCodecScript = `extends Node2D

@export var string_value: String = ""
@export var string_name_value: StringName = &""
@export var node_path_value: NodePath = NodePath("")
@export var packed_string_value: PackedStringArray = PackedStringArray()
@export var dictionary_value: Dictionary = {}
@export var rect_i_value: Rect2i = Rect2i()
@export var aabb_value: AABB = AABB()
@export var transform_value: Transform3D = Transform3D()
`

func seedValueCodecScene(t *testing.T) (string, *mcp.ClientSession) {
	t.Helper()
	root, session := seedScene(t, "scenes/codec.tscn", "Node2D")
	writeE2EFile(t, filepath.Join(root, "scripts", "codec.gd"), valueCodecScript)
	requireToolText(t, session, "attach_script", map[string]any{
		"scenePath":  "scenes/codec.tscn",
		"nodePath":   "root",
		"scriptPath": "scripts/codec.gd",
	})
	return root, session
}

func readRootProperties(t *testing.T, session *mcp.ClientSession) map[string]any {
	t.Helper()
	text := requireToolText(t, session, "read", map[string]any{
		"type": "scene",
		"path": "scenes/codec.tscn",
	})
	var tree sceneNode
	if err := json.Unmarshal([]byte(text), &tree); err != nil {
		t.Fatalf("unmarshal scene %q: %v", text, err)
	}
	return tree.Properties
}

func TestValueCodecScalarCoercion(t *testing.T) {
	t.Parallel()
	_, session := seedValueCodecScene(t)

	requireToolText(t, session, "modify_scene_node", map[string]any{
		"scenePath": "scenes/codec.tscn",
		"nodePath":  "root",
		"properties": map[string]any{
			"string_value":        "hello",
			"string_name_value":   "world",
			"node_path_value":     "root/Child",
			"packed_string_value": []any{"a", "b"},
		},
	})

	props := readRootProperties(t, session)
	want := map[string]any{
		"string_value":        "hello",
		"string_name_value":   map[string]any{"$type": "StringName", "value": "world"},
		"node_path_value":     map[string]any{"$type": "NodePath", "path": "root/Child"},
		"packed_string_value": map[string]any{"$type": "PackedStringArray", "values": []any{"a", "b"}},
	}
	for key, expected := range want {
		if got := props[key]; !reflect.DeepEqual(got, expected) {
			t.Errorf("%s = %#v, want %#v", key, got, expected)
		}
	}
}

func TestValueCodecDictionaryTag(t *testing.T) {
	t.Parallel()
	_, session := seedValueCodecScene(t)

	requireToolText(t, session, "modify_scene_node", map[string]any{
		"scenePath": "scenes/codec.tscn",
		"nodePath":  "root",
		"properties": map[string]any{
			"dictionary_value": map[string]any{
				"$type": "Dictionary",
				"value": map[string]any{"a": 1},
			},
		},
	})

	props := readRootProperties(t, session)
	want := map[string]any{"a": float64(1)}
	if got := props["dictionary_value"]; !reflect.DeepEqual(got, want) {
		t.Errorf("dictionary_value = %#v, want %#v", got, want)
	}
}

func TestValueCodecGodotTextFallback(t *testing.T) {
	t.Parallel()
	_, session := seedValueCodecScene(t)

	requireToolText(t, session, "modify_scene_node", map[string]any{
		"scenePath": "scenes/codec.tscn",
		"nodePath":  "root",
		"properties": map[string]any{
			"dictionary_value": map[string]any{
				"k": map[string]any{"$type": "GodotText", "text": "<Object#12345>"},
			},
		},
	})

	props := readRootProperties(t, session)
	want := map[string]any{"k": "<Object#12345>"}
	if got := props["dictionary_value"]; !reflect.DeepEqual(got, want) {
		t.Errorf("dictionary_value = %#v, want %#v", got, want)
	}
}

func TestValueCodecOptionalFieldsDefault(t *testing.T) {
	t.Parallel()
	_, session := seedValueCodecScene(t)

	requireToolText(t, session, "modify_scene_node", map[string]any{
		"scenePath": "scenes/codec.tscn",
		"nodePath":  "root",
		"properties": map[string]any{
			"dictionary_value": map[string]any{
				"c": map[string]any{"$type": "Color", "r": 1, "g": 0, "b": 0},
				"q": map[string]any{"$type": "Quaternion", "x": 0, "y": 0, "z": 0},
			},
		},
	})

	props := readRootProperties(t, session)
	want := map[string]any{
		"c": map[string]any{"$type": "Color", "r": float64(1), "g": float64(0), "b": float64(0), "a": float64(1)},
		"q": map[string]any{"$type": "Quaternion", "x": float64(0), "y": float64(0), "z": float64(0), "w": float64(1)},
	}
	if got := props["dictionary_value"]; !reflect.DeepEqual(got, want) {
		t.Errorf("dictionary_value = %#v, want %#v", got, want)
	}
}

func TestValueCodecTypeFailures(t *testing.T) {
	t.Parallel()
	root, session := seedValueCodecScene(t)
	before := readFile(t, filepath.Join(root, "scenes", "codec.tscn"))

	cases := []struct {
		name  string
		props map[string]any
		want  string
	}{
		{
			name:  "string name expects a string",
			props: map[string]any{"string_name_value": 5},
			want:  "expected a string",
		},
		{
			name:  "node path expects a string",
			props: map[string]any{"node_path_value": 5},
			want:  "expected a string path",
		},
		{
			name:  "packed string array expects an array",
			props: map[string]any{"packed_string_value": 5},
			want:  "expected an array of strings",
		},
		{
			name:  "packed string array rejects non-strings",
			props: map[string]any{"packed_string_value": []any{1, 2}},
			want:  "expected an array of strings",
		},
		{
			name: "rect2i component wrong vector type",
			props: map[string]any{"rect_i_value": map[string]any{
				"$type":    "Rect2i",
				"position": map[string]any{"$type": "Vector2", "x": 0, "y": 0},
				"size":     map[string]any{"$type": "Vector2i", "x": 1, "y": 1},
			}},
			want: `field "position" has the wrong vector type`,
		},
		{
			name: "aabb component wrong vector type",
			props: map[string]any{"aabb_value": map[string]any{
				"$type":    "AABB",
				"position": map[string]any{"$type": "Vector2", "x": 0, "y": 0},
				"size":     map[string]any{"$type": "Vector3", "x": 1, "y": 1, "z": 1},
			}},
			want: `field "position" has the wrong vector type`,
		},
		{
			name: "transform3d needs a basis",
			props: map[string]any{"transform_value": map[string]any{
				"$type":  "Transform3D",
				"basis":  map[string]any{"$type": "Vector2", "x": 1, "y": 0},
				"origin": map[string]any{"$type": "Vector3", "x": 0, "y": 0, "z": 0},
			}},
			want: `field "basis" must be a {"$type": "Basis"} value`,
		},
		{
			name: "godot text needs string text",
			props: map[string]any{"dictionary_value": map[string]any{
				"k": map[string]any{"$type": "GodotText", "text": 5},
			}},
			want: `expected a string for "text" in $type GodotText`,
		},
		{
			name: "unknown tag nested in an array",
			props: map[string]any{"dictionary_value": map[string]any{
				"k": []any{map[string]any{"$type": "BogusTag"}},
			}},
			want: "unknown $type: BogusTag",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res := callTool(t, session, "modify_scene_node", map[string]any{
				"scenePath":  "scenes/codec.tscn",
				"nodePath":   "root",
				"properties": tc.props,
			})
			if !res.IsError {
				t.Fatalf("expected an error result, got: %s", firstText(t, res))
			}
			requireContains(t, firstText(t, res), "Invalid property value: "+tc.want)
		})
	}

	if after := readFile(t, filepath.Join(root, "scenes", "codec.tscn")); after != before {
		t.Errorf("a rejected property changed the scene:\nbefore:\n%s\nafter:\n%s", before, after)
	}
}
