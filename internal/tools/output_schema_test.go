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
// JSON-native types as anyOf branches with single string types, so the value
// contract is visible without an array-valued top-level type that stricter
// schema clients reject.
func TestGodotValueSchemaAdvertisesContract(t *testing.T) {
	schema := godotValueSchema("A value in the tool value contract")
	if schema["description"] != "A value in the tool value contract" {
		t.Errorf("description = %v", schema["description"])
	}
	if _, present := schema["type"]; present {
		t.Errorf("type = %#v, want no top-level type", schema["type"])
	}

	want := []string{"null", "boolean", "string", "number", "array", "object"}
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

// TestMutationErrorCarriesStatusEnvelope verifies a failing mutation returns the
// success/message envelope its outputSchema requires, so a client validating
// structuredContent finds the required fields instead of an absent body. Errors
// are spec-permissible without structuredContent, but this keeps the declared
// schema true.
func TestMutationErrorCarriesStatusEnvelope(t *testing.T) {
	tool := removeSceneNode()
	result := callToolArgs(t, tool, "", "{}")
	if !result.IsError {
		t.Fatalf("expected an error result: %+v", result.Content)
	}
	if !hasRequired(tool.OutputSchema, "success", "message") {
		t.Fatalf("mutation schema does not require the envelope: %v", tool.OutputSchema)
	}
	structured, ok := result.StructuredContent.(map[string]any)
	if !ok {
		t.Fatalf("structuredContent = %T, want object", result.StructuredContent)
	}
	if structured["success"] != false {
		t.Errorf("success = %v, want false", structured["success"])
	}
	if message, _ := structured["message"].(string); message == "" {
		t.Errorf("message = %v, want non-empty", structured["message"])
	}
}

// TestReadOutputSchemaCarriesFreeFormPayload verifies a read tool's free-form
// payload carries its additionalProperties shape to the top level, so a client
// that reads only the top level still learns the list result is a name-to-value
// map rather than an unconstrained object, and the error envelope stays an
// accepted alternative through anyOf.
func TestReadOutputSchemaCarriesFreeFormPayload(t *testing.T) {
	schema := readOutputSchema(map[string]any{
		"type":                 "object",
		"additionalProperties": godotValueSchema("A value in the tool value contract"),
	})
	if _, ok := schema["additionalProperties"]; !ok {
		t.Fatalf("read output schema = %#v, want the payload's additionalProperties", schema)
	}
	branches, ok := schema["anyOf"].([]any)
	if !ok || len(branches) != 2 {
		t.Fatalf("anyOf = %#v, want the error and payload branches", schema["anyOf"])
	}
	if !hasRequired(asSchema(branches[0]), "success", "message") {
		t.Errorf("error branch = %#v, want the envelope", branches[0])
	}
}

// TestReadInputMapAdvertisesActionMap pins the free-form payload case to the
// tool that has it: the list result is keyed by input action name, so the top
// level of its output schema must describe that map.
func TestReadInputMapAdvertisesActionMap(t *testing.T) {
	tool := readInputMap()
	if _, ok := tool.OutputSchema["additionalProperties"]; !ok {
		t.Errorf("read_input_map output schema must describe the action-name map: %#v", tool.OutputSchema)
	}
}

// TestReadOutputSchemaOmitsEnvelopeFromTopLevel verifies a read tool's
// top-level properties describe only its payload. The error envelope stays an
// accepted shape through the anyOf branch, so a successful read never
// advertises success/message fields it does not return while a failure still
// validates.
func TestReadOutputSchemaOmitsEnvelopeFromTopLevel(t *testing.T) {
	schema := readOutputSchema(map[string]any{
		"type":       "object",
		"properties": map[string]any{"version": map[string]any{"type": "string"}},
		"required":   []string{"version"},
	})
	props, ok := schema["properties"].(map[string]any)
	if !ok {
		t.Fatalf("properties = %T", schema["properties"])
	}
	for _, key := range []string{"success", "message", "solutions"} {
		if _, present := props[key]; present {
			t.Errorf("top-level properties advertise %q, which the payload never returns", key)
		}
	}
	if _, present := props["version"]; !present {
		t.Error("top-level properties dropped the payload field version")
	}
	branches, ok := schema["anyOf"].([]any)
	if !ok || len(branches) != 2 {
		t.Fatalf("anyOf = %#v, want the error and payload branches", schema["anyOf"])
	}
	if !hasRequired(asSchema(branches[0]), "success", "message") {
		t.Errorf("error branch = %#v, want the envelope", branches[0])
	}
}

// TestPayloadOutputSchemaPinsShape verifies a bare payload schema declares its
// fields and requires every one, while still omitting the status envelope, so
// a read or list result is described as precisely as a mutation's envelope.
func TestPayloadOutputSchemaPinsShape(t *testing.T) {
	schema := payloadOutputSchema(map[string]any{"value": map[string]any{"type": "string"}})
	required, _ := schema["required"].([]string)
	if len(required) != 1 || !hasRequired(schema, "value") {
		t.Errorf("required = %v, want [value]", schema["required"])
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
