package godotconfig

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"sort"
	"strconv"
	"strings"
)

// The tool value contract. Values cross the MCP boundary as JSON: JSON-native
// data maps directly, and engine-only types are tagged objects such as
// {"$type":"Vector2","x":1,"y":2}. DecodeJSONValue is the Go half of the codec;
// the GDScript half (_variant_to_json) lives in godot_operations.gd. Both are
// exact inverses, so a value read from Godot can be written back unchanged.

// DecodeJSONValue parses one value in the tool value contract. Numbers keep
// their JSON spelling, so 1280 is an Int and 1280.0 is a Float. An unknown
// $type is an error rather than a silent pass-through.
func DecodeJSONValue(raw []byte) (Value, error) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		return nil, fmt.Errorf("decode value: %w", err)
	}
	var extra any
	if err := dec.Decode(&extra); err != io.EOF {
		return nil, fmt.Errorf("unexpected trailing content after value")
	}
	return FromJSON(v)
}

// FromJSON converts a decoded JSON value (from encoding/json with UseNumber,
// or an equivalent tree) into a Value.
func FromJSON(v any) (Value, error) {
	switch t := v.(type) {
	case nil:
		return Nil{}, nil
	case bool:
		return Bool(t), nil
	case string:
		return String(t), nil
	case json.Number:
		return numberValue(t)
	case float64:
		return Float(t), nil
	case int:
		return Int(t), nil
	case int64:
		return Int(t), nil
	case []any:
		arr := make(Array, 0, len(t))
		for _, e := range t {
			ev, err := FromJSON(e)
			if err != nil {
				return nil, err
			}
			arr = append(arr, ev)
		}
		return arr, nil
	case map[string]any:
		return objectValue(t)
	default:
		return nil, fmt.Errorf("unsupported JSON value %T", v)
	}
}

// objectValue maps a JSON object to a tagged engine value or, absent a $type
// tag, a Dictionary with keys sorted so the encoding is deterministic.
func objectValue(obj map[string]any) (Value, error) {
	if tag, ok := obj["$type"].(string); ok {
		return taggedValue(tag, obj)
	}
	keys := make([]string, 0, len(obj))
	for key := range obj {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	dict := Dictionary{}
	for _, key := range keys {
		ev, err := FromJSON(obj[key])
		if err != nil {
			return nil, err
		}
		dict.Entries = append(dict.Entries, DictEntry{Key: String(key), Value: ev})
	}
	return dict, nil
}

// numberValue picks Int or Float from the number's JSON spelling.
func numberValue(n json.Number) (Value, error) {
	s := n.String()
	if strings.ContainsAny(s, ".eE") {
		f, err := n.Float64()
		if err != nil {
			return nil, fmt.Errorf("parse number %q: %w", s, err)
		}
		return Float(f), nil
	}
	i, err := n.Int64()
	if err == nil {
		return Int(i), nil
	}
	f, ferr := n.Float64()
	if ferr != nil {
		return nil, fmt.Errorf("parse number %q: %w", s, err)
	}
	return Float(f), nil
}

// taggedValue builds the Value for a recognized $type. Every constructor is
// emitted as a numeric Godot literal, the form the ConfigFile parser accepts.
func taggedValue(tag string, obj map[string]any) (Value, error) {
	switch tag {
	case "GodotText":
		text, err := stringField(obj, "text")
		if err != nil {
			return nil, err
		}
		return Opaque(text), nil
	case "PackedStringArray":
		raw, ok := obj["values"].([]any)
		if !ok {
			return nil, fmt.Errorf(`$type PackedStringArray needs a "values" array`)
		}
		out := make(PackedStringArray, 0, len(raw))
		for _, item := range raw {
			s, ok := item.(string)
			if !ok {
				return nil, fmt.Errorf("PackedStringArray values must be strings")
			}
			out = append(out, s)
		}
		return out, nil
	case "StringName":
		value, err := stringField(obj, "value")
		if err != nil {
			return nil, err
		}
		return StringName(value), nil
	case "Vector2":
		return numeric("Vector2", obj, false, "x", "y")
	case "Vector2i":
		return numeric("Vector2i", obj, true, "x", "y")
	case "Vector3":
		return numeric("Vector3", obj, false, "x", "y", "z")
	case "Vector3i":
		return numeric("Vector3i", obj, true, "x", "y", "z")
	case "Vector4":
		return numeric("Vector4", obj, false, "x", "y", "z", "w")
	case "Vector4i":
		return numeric("Vector4i", obj, true, "x", "y", "z", "w")
	case "Color":
		return numericDefaulted("Color", obj, false, 1.0, "r", "g", "b", "a")
	case "Quaternion":
		return numericDefaulted("Quaternion", obj, false, 1.0, "x", "y", "z", "w")
	case "Plane":
		normal, err := vectorFields(obj, "normal", false, 3)
		if err != nil {
			return nil, err
		}
		d, err := floatField(obj, "d")
		if err != nil {
			return nil, err
		}
		return Opaque("Plane(" + strings.Join(append(normal, d), ", ") + ")"), nil
	case "Rect2":
		return composite("Rect2", obj, false, 2, "position", "size")
	case "Rect2i":
		return composite("Rect2i", obj, true, 2, "position", "size")
	case "Transform2D":
		return composite("Transform2D", obj, false, 2, "x", "y", "origin")
	case "Basis":
		return composite("Basis", obj, false, 3, "x", "y", "z")
	case "AABB":
		return composite("AABB", obj, false, 3, "position", "size")
	case "Transform3D":
		basis, err := basisFields(obj, "basis")
		if err != nil {
			return nil, err
		}
		origin, err := vectorFields(obj, "origin", false, 3)
		if err != nil {
			return nil, err
		}
		return Opaque("Transform3D(" + strings.Join(append(basis, origin...), ", ") + ")"), nil
	case "NodePath":
		path, err := stringField(obj, "path")
		if err != nil {
			return nil, err
		}
		return Opaque(`NodePath("` + EscapeString(path) + `")`), nil
	case "Dictionary":
		inner, ok := obj["value"].(map[string]any)
		if !ok {
			return nil, fmt.Errorf(`$type Dictionary needs a "value" object`)
		}
		return FromJSON(inner)
	case "Resource":
		return nil, fmt.Errorf("$type Resource cannot be written to project.godot; use a plain res:// string setting")
	default:
		return nil, fmt.Errorf("unknown $type %q", tag)
	}
}

// numeric builds a "Name(v1, v2, ...)" constructor from the named fields.
func numeric(name string, obj map[string]any, ints bool, fields ...string) (Value, error) {
	parts := make([]string, 0, len(fields))
	for _, field := range fields {
		part, err := numberField(obj, field, ints)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", name, err)
		}
		parts = append(parts, part)
	}
	return Opaque(name + "(" + strings.Join(parts, ", ") + ")"), nil
}

// numericDefaulted is numeric where the last field is optional and defaults to
// def when absent.
func numericDefaulted(name string, obj map[string]any, ints bool, def float64, fields ...string) (Value, error) {
	parts := make([]string, 0, len(fields))
	for i, field := range fields {
		if _, ok := obj[field]; !ok && i == len(fields)-1 {
			parts = append(parts, formatNumber(def, ints))
			continue
		}
		part, err := numberField(obj, field, ints)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", name, err)
		}
		parts = append(parts, part)
	}
	return Opaque(name + "(" + strings.Join(parts, ", ") + ")"), nil
}

// composite builds a "Name(all vector components flattened)" constructor.
func composite(name string, obj map[string]any, ints bool, dim int, fields ...string) (Value, error) {
	parts, err := compositeText(name, obj, ints, dim, fields...)
	if err != nil {
		return nil, err
	}
	return Opaque(name + "(" + strings.Join(parts, ", ") + ")"), nil
}

// compositeText returns the flattened numeric components of the named vector
// fields, failing when any is not a tagged vector object with the needed axes.
func compositeText(name string, obj map[string]any, ints bool, dim int, fields ...string) ([]string, error) {
	parts := make([]string, 0, dim*len(fields))
	for _, field := range fields {
		vec, err := vectorFields(obj, field, ints, dim)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", name, err)
		}
		parts = append(parts, vec...)
	}
	return parts, nil
}

// basisFields flattens a tagged Basis field (three tagged vector axes) into
// its nine numeric components.
func basisFields(obj map[string]any, field string) ([]string, error) {
	inner, ok := obj[field].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("field %q must be a tagged Basis object", field)
	}
	parts := make([]string, 0, 9)
	for _, axis := range []string{"x", "y", "z"} {
		vec, err := vectorFields(inner, axis, false, 3)
		if err != nil {
			return nil, fmt.Errorf("field %q: %w", field, err)
		}
		parts = append(parts, vec...)
	}
	return parts, nil
}

// vectorFields flattens one tagged vector field into its numeric components.
func vectorFields(obj map[string]any, field string, ints bool, dim int) ([]string, error) {
	inner, ok := obj[field].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("field %q must be a tagged vector object", field)
	}
	axes := []string{"x", "y", "z", "w"}[:dim]
	parts := make([]string, 0, dim)
	for _, axis := range axes {
		part, err := numberField(inner, axis, ints)
		if err != nil {
			return nil, fmt.Errorf("field %q: %w", field, err)
		}
		parts = append(parts, part)
	}
	return parts, nil
}

func numberField(obj map[string]any, field string, asInt bool) (string, error) {
	v, ok := obj[field]
	if !ok {
		return "", fmt.Errorf("missing field %q", field)
	}
	f, err := toFloat(v)
	if err != nil {
		return "", fmt.Errorf("field %q: %w", field, err)
	}
	return formatNumber(f, asInt), nil
}

func floatField(obj map[string]any, field string) (string, error) {
	return numberField(obj, field, false)
}

func stringField(obj map[string]any, field string) (string, error) {
	v, ok := obj[field]
	if !ok {
		return "", fmt.Errorf("missing field %q", field)
	}
	s, ok := v.(string)
	if !ok {
		return "", fmt.Errorf("field %q must be a string", field)
	}
	return s, nil
}

func toFloat(v any) (float64, error) {
	switch n := v.(type) {
	case json.Number:
		f, err := n.Float64()
		if err != nil {
			return 0, fmt.Errorf("parse number %q: %w", n, err)
		}
		return f, nil
	case float64:
		return n, nil
	case float32:
		return float64(n), nil
	case int:
		return float64(n), nil
	case int64:
		return float64(n), nil
	default:
		return 0, fmt.Errorf("expected a number, got %T", v)
	}
}

func formatNumber(f float64, asInt bool) string {
	if asInt {
		return strconv.FormatInt(int64(f), 10)
	}
	return formatComponent(f)
}

// formatComponent writes a vector/color component the way Godot's VariantWriter
// does: an integral value has no decimal point, so a Vector2 reads as
// Vector2(1, 2) rather than Vector2(1.0, 2.0).
func formatComponent(f float64) string {
	if math.IsInf(f, 0) || math.IsNaN(f) {
		return formatFloat(f)
	}
	if f == math.Trunc(f) && math.Abs(f) < 1e15 {
		return strconv.FormatInt(int64(f), 10)
	}
	return formatFloat(f)
}
