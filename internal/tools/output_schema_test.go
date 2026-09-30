package tools

import (
	"strings"
	"testing"

	"github.com/vilppuvuorinen/slopfood-godot-mcp/internal/godotconfig"
)

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

func TestGodotValueSchemaAdvertisesContract(t *testing.T) {
	const local = "A value in the tool value contract"
	schema := godotValueSchema(local)
	description, _ := schema["description"].(string)
	if !strings.HasPrefix(description, local) {
		t.Errorf("description = %v, want it to start with %q", schema["description"], local)
	}
	for _, tag := range godotconfig.ValueContractTags {
		if !strings.Contains(description, tag) {
			t.Errorf("description omits $type tag %q", tag)
		}
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
	if _, ok := props["solutions"]; ok {
		t.Error("success schema advertises solutions, which success never returns")
	}
	message, _ := props["message"].(map[string]any)
	if got, _ := message["description"].(string); got != envelopeMessageContract {
		t.Errorf("message description = %q, want %q", got, envelopeMessageContract)
	}
}

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
