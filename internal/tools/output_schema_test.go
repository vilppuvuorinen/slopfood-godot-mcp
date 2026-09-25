package tools

import "testing"

// hasRequired reports whether schema requires every key.
func hasRequired(schema map[string]any, keys ...string) bool {
	required, _ := schema["required"].([]string)
	for _, key := range keys {
		found := false
		for _, r := range required {
			if r == key {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

// TestGodotValueSchemaAdvertisesContract verifies a value slot names the
// JSON-native types and repeats them as anyOf branches, so the value contract
// is visible in the schema instead of a bare description.
func TestGodotValueSchemaAdvertisesContract(t *testing.T) {
	schema := godotValueSchema("A value in the tool value contract")
	if schema["description"] != "A value in the tool value contract" {
		t.Errorf("description = %v", schema["description"])
	}

	want := []string{"null", "boolean", "string", "number", "array", "object"}
	types, ok := schema["type"].([]string)
	if !ok || len(types) != len(want) {
		t.Fatalf("type = %#v, want %v", schema["type"], want)
	}
	for i, name := range want {
		if types[i] != name {
			t.Errorf("type[%d] = %q, want %q", i, types[i], name)
		}
	}

	branches, ok := schema["anyOf"].([]any)
	if !ok || len(branches) != len(want) {
		t.Fatalf("anyOf = %#v, want %d branches", schema["anyOf"], len(want))
	}
	for i, name := range want {
		branch, ok := branches[i].(map[string]any)
		if !ok || branch["type"] != name {
			t.Errorf("anyOf[%d] = %#v, want type %q", i, branches[i], name)
		}
	}
}

// TestStatusOutputSchemaRequiresEnvelope verifies the status envelope is
// advertised and required, so a mutation result is never described loosely.
func TestStatusOutputSchemaRequiresEnvelope(t *testing.T) {
	schema := statusOutputSchema(map[string]any{"name": map[string]any{"type": "string"}})
	if !hasRequired(schema, "success", "message") {
		t.Fatalf("required = %v, want success and message", schema["required"])
	}
	props, ok := schema["properties"].(map[string]any)
	if !ok {
		t.Fatalf("properties = %T", schema["properties"])
	}
	for _, key := range []string{"success", "message", "name"} {
		if _, ok := props[key]; !ok {
			t.Errorf("missing property %q", key)
		}
	}
}

// TestActionOutputSchemaSeparatesShapes verifies a mixed action tool advertises
// the status envelope for mutations and the bare payload for reads via anyOf,
// so the read shape never promises success or message.
func TestActionOutputSchemaSeparatesShapes(t *testing.T) {
	schema := actionOutputSchema(
		map[string]any{"action": map[string]any{"type": "string"}},
		payloadOutputSchema(map[string]any{"layers": map[string]any{"type": "array"}}),
	)

	props, ok := schema["properties"].(map[string]any)
	if !ok {
		t.Fatalf("properties = %T", schema["properties"])
	}
	for _, key := range []string{"success", "message", "action", "layers"} {
		if _, present := props[key]; !present {
			t.Errorf("missing merged property %q", key)
		}
	}
	if _, hasRequired := schema["required"]; hasRequired {
		t.Errorf("a mixed schema must not require one shape: %v", schema["required"])
	}

	branches, ok := schema["anyOf"].([]any)
	if !ok || len(branches) != 2 {
		t.Fatalf("anyOf = %#v, want two branches", schema["anyOf"])
	}
	status, ok := branches[0].(map[string]any)
	if !ok || !hasRequired(status, "success", "message") {
		t.Errorf("status branch must require the envelope: %#v", branches[0])
	}
	payload, ok := branches[1].(map[string]any)
	if !ok || hasRequired(payload, "success") {
		t.Errorf("payload branch must not require the envelope: %#v", branches[1])
	}
	payloadProps, _ := payload["properties"].(map[string]any)
	if _, ok := payloadProps["layers"]; !ok {
		t.Errorf("payload branch missing layers: %#v", payload)
	}
}

// TestPayloadOutputSchemaOmitsEnvelope verifies a bare payload schema declares
// only the payload fields.
func TestPayloadOutputSchemaOmitsEnvelope(t *testing.T) {
	schema := payloadOutputSchema(map[string]any{"value": map[string]any{"type": "string"}})
	if _, ok := schema["required"]; ok {
		t.Errorf("payload schema must require nothing: %v", schema["required"])
	}
	props, ok := schema["properties"].(map[string]any)
	if !ok {
		t.Fatalf("properties = %T", schema["properties"])
	}
	if _, ok := props["success"]; ok {
		t.Error("payload schema must not advertise success")
	}
	if _, ok := props["message"]; ok {
		t.Error("payload schema must not advertise message")
	}
	if _, ok := props["value"]; !ok {
		t.Error("payload schema missing value")
	}
}
