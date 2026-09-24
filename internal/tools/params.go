package tools

import (
	"bytes"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

// parameterMappings mirrors the reference server's snake_case to camelCase
// aliases. Every documented camelCase argument also accepts its snake_case
// spelling.
var parameterMappings = map[string]string{
	"project_path":     "projectPath",
	"scene_path":       "scenePath",
	"root_node_type":   "rootNodeType",
	"parent_node_path": "parentNodePath",
	"node_type":        "nodeType",
	"node_name":        "nodeName",
	"texture_path":     "texturePath",
	"node_path":        "nodePath",
	"output_path":      "outputPath",
	"mesh_item_names":  "meshItemNames",
	"new_path":         "newPath",
	"new_name":         "newName",
	"new_parent_path":  "newParentPath",
	"signal_name":      "signalName",
	"source_path":      "sourcePath",
	"target_path":      "targetPath",
	"preset_name":      "presetName",
	"file_path":        "filePath",
	"script_path":      "scriptPath",
	"class_name":       "className",
	"base_class":       "baseClass",
	"namespace_name":   "namespaceName",
	"script_paths":     "scriptPaths",
	"resource_type":    "resourceType",
	"resource_path":    "resourcePath",
	"shader_path":      "shaderPath",
	"shader_type":      "shaderType",
	"directory":        "directory",
	"recursive":        "recursive",
	"scene":            "scene",
}

// reverseParameterMappings maps camelCase keys back to their snake_case names,
// generated from parameterMappings like the reference server.
var reverseParameterMappings = func() map[string]string {
	reverse := make(map[string]string, len(parameterMappings))
	for snake, camel := range parameterMappings {
		reverse[camel] = snake
	}
	return reverse
}()

// uppercaseLetter matches a single ASCII uppercase letter for the generic
// camelCase to snake_case fallback.
var uppercaseLetter = regexp.MustCompile(`[A-Z]`)

// aliasParams rewrites snake_case argument keys to their camelCase equivalents
// so handlers accept both spellings, mirroring the reference server's
// normalizeParameters. Only the argument names at the top level are rewritten;
// nested values (most importantly the opaque "properties" payload) are user
// data and are left byte-for-byte untouched. When both spellings are present
// the camelCase value wins.
//
// An array at the top level is treated as a list of argument objects, each of
// whose top-level keys is normalized.
func aliasParams(raw json.RawMessage, aliases map[string]string) (json.RawMessage, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return raw, nil
	}
	switch trimmed[0] {
	case '[':
		var arr []json.RawMessage
		if err := json.Unmarshal(raw, &arr); err != nil {
			return nil, fmt.Errorf("invalid arguments: %w", err)
		}
		for i, elem := range arr {
			nested, err := aliasParams(elem, aliases)
			if err != nil {
				return nil, err
			}
			arr[i] = nested
		}
		encoded, err := json.Marshal(arr)
		if err != nil {
			return nil, fmt.Errorf("encode normalized arguments: %w", err)
		}
		return encoded, nil
	case '{':
	default:
		return raw, nil
	}
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(raw, &obj); err != nil {
		return nil, fmt.Errorf("invalid arguments: %w", err)
	}
	// Apply aliases only to the top-level argument names. The camelCase alias
	// always wins over a snake_case twin regardless of map iteration order.
	for from, to := range aliases {
		if from == to {
			continue
		}
		value, ok := obj[from]
		if !ok {
			continue
		}
		if _, exists := obj[to]; !exists {
			obj[to] = value
		}
		delete(obj, from)
	}
	encoded, err := json.Marshal(obj)
	if err != nil {
		return nil, fmt.Errorf("encode normalized arguments: %w", err)
	}
	return encoded, nil
}

// toSnakeCase converts the top-level camelCase argument names to snake_case
// before they are handed to the Godot script, mirroring the reference server's
// convertCamelToSnakeCase. Only the top-level keys are argument names; nested
// values (most importantly the opaque "properties" payload) are user data and
// are passed through untouched, so dictionary keys are never rewritten.
// Explicit aliases win; otherwise every uppercase letter becomes an
// underscore-prefixed lowercase letter.
func toSnakeCase(params map[string]any) map[string]any {
	out := make(map[string]any, len(params))
	for key, value := range params {
		snake, ok := reverseParameterMappings[key]
		if !ok {
			snake = uppercaseLetter.ReplaceAllStringFunc(key, func(letter string) string {
				return "_" + strings.ToLower(letter)
			})
		}
		out[snake] = value
	}
	return out
}
