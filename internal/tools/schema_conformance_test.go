package tools

import (
	"encoding/json"
	"fmt"
	"math"
	"reflect"
	"strings"
	"testing"

	"github.com/vilppuvuorinen/slopfood-godot-mcp/internal/mcp"
)

func schemaViolation(schema, root map[string]any, value any, path string) string {
	if ref, ok := schema["$ref"].(string); ok {
		resolved := resolveRef(root, ref)
		if resolved == nil {
			return path + ": unresolved $ref " + ref
		}
		return schemaViolation(resolved, root, value, path)
	}
	if want, ok := schema["const"]; ok && !reflect.DeepEqual(value, want) {
		return fmt.Sprintf("%s: value %#v is not the const %#v", path, value, want)
	}
	if enum, ok := schema["enum"].([]string); ok && !containsStringValue(enum, value) {
		return fmt.Sprintf("%s: value %#v is not in the enum %#v", path, value, enum)
	}
	if schemaType, ok := schema["type"].(string); ok && !matchesSchemaType(schemaType, value) {
		return fmt.Sprintf("%s: value %#v is not a %s", path, value, schemaType)
	}

	if required, ok := schema["required"].([]string); ok {
		obj, isObject := value.(map[string]any)
		if !isObject {
			return fmt.Sprintf("%s: required fields declared for a non-object", path)
		}
		for _, key := range required {
			if _, present := obj[key]; !present {
				return fmt.Sprintf("%s: missing required field %q", path, key)
			}
		}
	}

	if props, ok := schema["properties"].(map[string]any); ok {
		if obj, isObject := value.(map[string]any); isObject {
			for name, sub := range props {
				field, present := obj[name]
				if !present {
					continue
				}
				subSchema, isMap := sub.(map[string]any)
				if !isMap {
					continue
				}
				if msg := schemaViolation(subSchema, root, field, path+"."+name); msg != "" {
					return msg
				}
			}
		}
	}

	if extra, ok := schema["additionalProperties"]; ok {
		if obj, isObject := value.(map[string]any); isObject {
			known := map[string]bool{}
			if props, ok := schema["properties"].(map[string]any); ok {
				for name := range props {
					known[name] = true
				}
			}
			switch extraSchema := extra.(type) {
			case bool:
				if !extraSchema {
					for name := range obj {
						if !known[name] {
							return fmt.Sprintf("%s: unexpected property %q", path, name)
						}
					}
				}
			case map[string]any:
				for name, field := range obj {
					if known[name] {
						continue
					}
					if msg := schemaViolation(extraSchema, root, field, path+"."+name); msg != "" {
						return msg
					}
				}
			}
		}
	}

	if items, ok := schema["items"].(map[string]any); ok {
		if arr, isArray := value.([]any); isArray {
			for i, item := range arr {
				if msg := schemaViolation(items, root, item, fmt.Sprintf("%s[%d]", path, i)); msg != "" {
					return msg
				}
			}
		}
	}

	return branchesViolation(schema, root, value, path)
}

func branchesViolation(schema, root map[string]any, value any, path string) string {
	if branches, ok := schema["anyOf"].([]any); ok {
		matched := 0
		for _, branch := range branches {
			if msg := schemaViolation(asSchema(branch), root, value, path); msg == "" {
				matched++
			}
		}
		if matched == 0 {
			return path + ": no anyOf branch matched"
		}
	}
	if branches, ok := schema["oneOf"].([]any); ok {
		matched := 0
		for _, branch := range branches {
			if msg := schemaViolation(asSchema(branch), root, value, path); msg == "" {
				matched++
			}
		}
		if matched != 1 {
			return fmt.Sprintf("%s: %d oneOf branches matched, want exactly 1", path, matched)
		}
	}
	if branches, ok := schema["allOf"].([]any); ok {
		for _, branch := range branches {
			if msg := schemaViolation(asSchema(branch), root, value, path); msg != "" {
				return msg
			}
		}
	}
	return ""
}

func asSchema(branch any) map[string]any {
	schema, _ := branch.(map[string]any)
	return schema
}

func resolveRef(root map[string]any, ref string) map[string]any {
	if !strings.HasPrefix(ref, "#/") {
		return nil
	}
	var node any = root
	for _, part := range strings.Split(strings.TrimPrefix(ref, "#/"), "/") {
		obj, ok := node.(map[string]any)
		if !ok {
			return nil
		}
		node, ok = obj[part]
		if !ok {
			return nil
		}
	}
	resolved, _ := node.(map[string]any)
	return resolved
}

func matchesSchemaType(want string, value any) bool {
	switch want {
	case "null":
		return value == nil
	case "boolean":
		_, ok := value.(bool)
		return ok
	case "string":
		_, ok := value.(string)
		return ok
	case "number":
		_, ok := value.(float64)
		return ok
	case "integer":
		number, ok := value.(float64)
		return ok && number == math.Trunc(number)
	case "array":
		_, ok := value.([]any)
		return ok
	case "object":
		_, ok := value.(map[string]any)
		return ok
	default:
		return true
	}
}

func containsStringValue(enum []string, value any) bool {
	for _, entry := range enum {
		if entry == value {
			return true
		}
	}
	return false
}

func jsonValue(t *testing.T, value any) any {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal structured content: %v", err)
	}
	var normalized any
	if err := json.Unmarshal(data, &normalized); err != nil {
		t.Fatalf("unmarshal structured content: %v", err)
	}
	return normalized
}

func TestSchemaValidatorRejectsViolations(t *testing.T) {
	schema := map[string]any{
		"type":       "object",
		"properties": map[string]any{"count": map[string]any{"type": "integer"}},
		"required":   []string{"count"},
	}
	if msg := schemaViolation(schema, schema, map[string]any{"count": 1.0}, "x"); msg != "" {
		t.Errorf("valid value rejected: %s", msg)
	}
	for name, value := range map[string]any{
		"missing required": map[string]any{},
		"wrong type":       map[string]any{"count": "one"},
		"non-integer":      map[string]any{"count": 1.5},
	} {
		if msg := schemaViolation(schema, schema, value, "x"); msg == "" {
			t.Errorf("%s: expected a violation", name)
		}
	}
}

func TestErrorResultMatchesStatusOutputSchema(t *testing.T) {
	envelope := jsonValue(t, mcp.ErrorResult("boom", "try again").StructuredContent)
	for _, newTool := range allTools {
		tool := newTool()
		if !hasRequired(tool.OutputSchema, "success", "message") {
			continue
		}
		if msg := schemaViolation(tool.OutputSchema, tool.OutputSchema, envelope, tool.Name); msg != "" {
			t.Errorf("tool %q rejects its own error envelope: %s", tool.Name, msg)
		}
	}
}

func TestSuccessResultsMatchOutputSchema(t *testing.T) {
	tests := []struct {
		name  string
		tool  func() mcp.Tool
		args  string
		setup func(t *testing.T) string
	}{
		{name: "create_project", tool: createProject, args: `{"projectName":"Demo"}`, setup: func(t *testing.T) string { t.Helper(); return t.TempDir() }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := callToolArgs(t, tt.tool(), tt.setup(t), tt.args)
			if result.IsError {
				t.Fatalf("unexpected error: %+v", result.Content)
			}
			tool := tt.tool()
			value := jsonValue(t, result.StructuredContent)
			if msg := schemaViolation(tool.OutputSchema, tool.OutputSchema, value, tool.Name); msg != "" {
				t.Errorf("tool %q result violates its outputSchema: %s", tool.Name, msg)
			}
		})
	}
}
