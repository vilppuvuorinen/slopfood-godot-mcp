package godotconfig

import "testing"

func TestDecodeJSONValueScalars(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want string
	}{
		{name: "null", raw: `null`, want: `null`},
		{name: "true", raw: `true`, want: `true`},
		{name: "false", raw: `false`, want: `false`},
		{name: "int", raw: `1280`, want: `1280`},
		{name: "negative int", raw: `-3`, want: `-3`},
		{name: "float", raw: `0.5`, want: `0.5`},
		{name: "integral float keeps decimal", raw: `1280.0`, want: `1280.0`},
		{name: "string", raw: `"hi"`, want: `"hi"`},
		{name: "escaped string", raw: `"say \"hi\""`, want: `"say \"hi\""`},
		{name: "array", raw: `[1, "a"]`, want: `[1, "a"]`},
		{name: "dictionary sorts keys", raw: `{"b": 2, "a": 1}`, want: "{\n\"a\": 1,\n\"b\": 2\n}"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			value, err := DecodeJSONValue([]byte(tt.raw))
			if err != nil {
				t.Fatalf("DecodeJSONValue(%s): %v", tt.raw, err)
			}
			if got := FormatValue(value); got != tt.want {
				t.Errorf("FormatValue = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestDecodeJSONValueTagged(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want string
	}{
		{name: "vector2", raw: `{"$type":"Vector2","x":1,"y":2}`, want: `Vector2(1, 2)`},
		{name: "vector2i", raw: `{"$type":"Vector2i","x":1,"y":2}`, want: `Vector2i(1, 2)`},
		{name: "vector3", raw: `{"$type":"Vector3","x":1,"y":2,"z":3}`, want: `Vector3(1, 2, 3)`},
		{name: "color default alpha", raw: `{"$type":"Color","r":1,"g":0,"b":0}`, want: `Color(1, 0, 0, 1)`},
		{name: "quaternion default w", raw: `{"$type":"Quaternion","x":0,"y":0,"z":0}`, want: `Quaternion(0, 0, 0, 1)`},
		{name: "node path", raw: `{"$type":"NodePath","path":"root/A"}`, want: `NodePath("root/A")`},
		{name: "string name", raw: `{"$type":"StringName","value":"foo"}`, want: `&"foo"`},
		{name: "godot text", raw: `{"$type":"GodotText","text":"Object(InputEventKey)"}`, want: `Object(InputEventKey)`},
		{name: "packed string array", raw: `{"$type":"PackedStringArray","values":["a","b"]}`, want: `PackedStringArray("a", "b")`},
		{name: "rect2", raw: `{"$type":"Rect2","position":{"$type":"Vector2","x":1,"y":2},"size":{"$type":"Vector2","x":3,"y":4}}`, want: `Rect2(1, 2, 3, 4)`},
		{name: "transform3d", raw: `{"$type":"Transform3D","basis":{"$type":"Basis","x":{"$type":"Vector3","x":1,"y":0,"z":0},"y":{"$type":"Vector3","x":0,"y":1,"z":0},"z":{"$type":"Vector3","x":0,"y":0,"z":1}},"origin":{"$type":"Vector3","x":5,"y":6,"z":7}}`, want: `Transform3D(1, 0, 0, 0, 1, 0, 0, 0, 1, 5, 6, 7)`},
		{name: "dictionary wrapper", raw: `{"$type":"Dictionary","value":{"a":1}}`, want: "{\n\"a\": 1\n}"},
		{name: "nested vector in array", raw: `[{"$type":"Vector2","x":1,"y":2}]`, want: `[Vector2(1, 2)]`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			value, err := DecodeJSONValue([]byte(tt.raw))
			if err != nil {
				t.Fatalf("DecodeJSONValue(%s): %v", tt.raw, err)
			}
			if got := FormatValue(value); got != tt.want {
				t.Errorf("FormatValue = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestDecodeJSONValueErrors(t *testing.T) {
	tests := []struct {
		name string
		raw  string
	}{
		{name: "unknown $type", raw: `{"$type":"Vectr2","x":1,"y":2}`},
		{name: "missing vector field", raw: `{"$type":"Vector2","x":1}`},
		{name: "non-numeric field", raw: `{"$type":"Vector2","x":"a","y":2}`},
		{name: "bad nested vector", raw: `{"$type":"Rect2","position":1,"size":{"$type":"Vector2","x":3,"y":4}}`},
		{name: "resource unsupported", raw: `{"$type":"Resource","path":"res://x.tres"}`},
		{name: "dictionary without value", raw: `{"$type":"Dictionary","other":{}}`},
		{name: "trailing content", raw: `1 2`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := DecodeJSONValue([]byte(tt.raw)); err == nil {
				t.Errorf("DecodeJSONValue(%s) = nil error, want an error", tt.raw)
			}
		})
	}
}
