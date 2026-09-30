package godotconfig

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// valueContract is the shared conformance fixture, read by the e2e suite too.
type valueContract struct {
	Values []struct {
		Name  string          `json:"name"`
		Value json.RawMessage `json:"value"`
		Godot string          `json:"godot"`
	} `json:"values"`
}

// loadValueContract reads the shared fixture, failing if it is missing or empty.
func loadValueContract(t *testing.T) valueContract {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locate test file")
	}
	path := filepath.Join(filepath.Dir(file), "..", "..", "testdata", "value_contract.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read value contract: %v", err)
	}
	var contract valueContract
	if err := json.Unmarshal(data, &contract); err != nil {
		t.Fatalf("unmarshal value contract: %v", err)
	}
	if len(contract.Values) == 0 {
		t.Fatal("value contract has no entries")
	}
	return contract
}

// TestValueContractGoHalf pins the Go half against the shared fixture; the e2e
// suite runs the same entries through the GDScript half.
func TestValueContractGoHalf(t *testing.T) {
	for _, entry := range loadValueContract(t).Values {
		t.Run(entry.Name, func(t *testing.T) {
			value, err := DecodeJSONValue(entry.Value)
			if err != nil {
				t.Fatalf("DecodeJSONValue(%s): %v", entry.Value, err)
			}
			if got := FormatValue(value); got != entry.Godot {
				t.Fatalf("FormatValue = %q, want %q", got, entry.Godot)
			}
			parsed, err := ParseValue(entry.Godot)
			if err != nil {
				t.Fatalf("ParseValue(%q): %v", entry.Godot, err)
			}
			if got := FormatValue(parsed); got != entry.Godot {
				t.Errorf("reparse = %q, want %q", got, entry.Godot)
			}
		})
	}
}

func TestValueToJSONEncoding(t *testing.T) {
	tests := []struct {
		name string
		val  Value
		want string
	}{
		{name: "nil", val: nil, want: `null`},
		{name: "Nil", val: Nil{}, want: `null`},
		{name: "bool", val: Bool(true), want: `true`},
		{name: "int", val: Int(1280), want: `1280`},
		{name: "float", val: Float(0.5), want: `0.5`},
		{name: "integral float keeps decimal", val: Float(1280), want: `1280.0`},
		{name: "string", val: String("hi"), want: `"hi"`},
		{name: "string name", val: StringName("foo"), want: `{"$type":"StringName","value":"foo"}`},
		{name: "array", val: Array{Int(1), String("a")}, want: `[1,"a"]`},
		{name: "dictionary", val: Dictionary{Entries: []DictEntry{{Key: String("b"), Value: Int(2)}, {Key: String("a"), Value: Int(1)}}}, want: `{"a":1,"b":2}`},
		{name: "packed string array", val: PackedStringArray{"a", "b"}, want: `{"$type":"PackedStringArray","values":["a","b"]}`},
		{name: "opaque", val: Opaque(`Object(InputEventKey)`), want: `{"$type":"GodotText","text":"Object(InputEventKey)"}`},
		{name: "nan", val: Float(math.NaN()), want: `{"$type":"GodotText","text":"nan"}`},
		{name: "inf", val: Float(math.Inf(1)), want: `{"$type":"GodotText","text":"inf"}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tree, err := ValueToJSON(tt.val)
			if err != nil {
				t.Fatalf("ValueToJSON: %v", err)
			}
			data, err := json.Marshal(tree)
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}
			if got := string(data); got != tt.want {
				t.Errorf("json = %s, want %s", got, tt.want)
			}
		})
	}
}

func TestValueToJSONRoundTrip(t *testing.T) {
	values := []Value{
		Nil{},
		Bool(false),
		Int(-3),
		Float(0.25),
		Float(1280),
		String(`say "hi"`),
		StringName("foo"),
		Array{Int(1), String("a"), Array{Bool(true)}},
		Dictionary{Entries: []DictEntry{{Key: String("deadzone"), Value: Float(0.5)}, {Key: String("events"), Value: Array{Opaque(`Object(InputEventKey)`)}}}},
		PackedStringArray{"res://en.translation", "res://fi.translation"},
		Opaque(`Object(InputEventKey,"physical_keycode":87)`),
	}
	for _, value := range values {
		tree, err := ValueToJSON(value)
		if err != nil {
			t.Fatalf("ValueToJSON(%s): %v", FormatValue(value), err)
		}
		data, err := json.Marshal(tree)
		if err != nil {
			t.Fatalf("marshal %s: %v", FormatValue(value), err)
		}
		decoded, err := DecodeJSONValue(data)
		if err != nil {
			t.Fatalf("DecodeJSONValue(%s): %v", data, err)
		}
		if got, want := FormatValue(decoded), FormatValue(value); got != want {
			t.Errorf("round trip = %s, want %s", got, want)
		}
	}
}

func TestValueContractTagsMatchDecoder(t *testing.T) {
	advertised := map[string]bool{}
	for _, tag := range ValueContractTags {
		advertised[tag] = true
		if _, err := taggedValue(tag, map[string]any{"$type": tag}); err != nil && err.Error() == fmt.Sprintf("unknown $type %q", tag) {
			t.Errorf("advertised tag %q is not recognized by taggedValue", tag)
		}
	}
	for _, entry := range loadValueContract(t).Values {
		for _, tag := range collectTypeTags(t, entry.Value) {
			if !advertised[tag] {
				t.Errorf("conformance fixture uses $type %q, which ValueContractTags does not advertise", tag)
			}
		}
	}
}

func collectTypeTags(t *testing.T, raw json.RawMessage) []string {
	t.Helper()
	var tree any
	if err := json.Unmarshal(raw, &tree); err != nil {
		t.Fatalf("unmarshal fixture value: %v", err)
	}
	var tags []string
	var walk func(any)
	walk = func(node any) {
		switch n := node.(type) {
		case map[string]any:
			if tag, ok := n["$type"].(string); ok {
				tags = append(tags, tag)
			}
			for _, child := range n {
				walk(child)
			}
		case []any:
			for _, child := range n {
				walk(child)
			}
		}
	}
	walk(tree)
	return tags
}

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
