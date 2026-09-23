package tools

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestAliasParamsRewritesSnakeCase(t *testing.T) {
	raw := json.RawMessage(`{
		"project_path": "/p",
		"scene_path": "main.tscn",
		"node_type": "Node2D",
		"properties": {"texture_path": "res://icon.png", "position": 1}
	}`)

	got, err := aliasParams(raw, parameterMappings)
	if err != nil {
		t.Fatalf("aliasParams: %v", err)
	}
	var obj map[string]any
	if err := json.Unmarshal(got, &obj); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if obj["projectPath"] != "/p" || obj["scenePath"] != "main.tscn" || obj["nodeType"] != "Node2D" {
		t.Errorf("top-level keys = %v", obj)
	}
	properties, ok := obj["properties"].(map[string]any)
	if !ok {
		t.Fatalf("properties = %T", obj["properties"])
	}
	if properties["texturePath"] != "res://icon.png" || properties["position"] != float64(1) {
		t.Errorf("nested properties = %v", properties)
	}
	if _, exists := obj["project_path"]; exists {
		t.Errorf("snake_case key survived: %v", obj)
	}
}

func TestAliasParamsCamelCaseWins(t *testing.T) {
	raw := json.RawMessage(`{"project_path": "/snake", "projectPath": "/camel"}`)

	got, err := aliasParams(raw, parameterMappings)
	if err != nil {
		t.Fatalf("aliasParams: %v", err)
	}
	var obj map[string]any
	if err := json.Unmarshal(got, &obj); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if obj["projectPath"] != "/camel" {
		t.Errorf("projectPath = %v, want /camel", obj["projectPath"])
	}
}

func TestAliasParamsLeavesArraysUntouched(t *testing.T) {
	raw := json.RawMessage(`{"mesh_item_names": ["a", "b"], "recursive": true}`)

	got, err := aliasParams(raw, parameterMappings)
	if err != nil {
		t.Fatalf("aliasParams: %v", err)
	}
	var obj map[string]any
	if err := json.Unmarshal(got, &obj); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	want := []any{"a", "b"}
	if !reflect.DeepEqual(obj["meshItemNames"], want) {
		t.Errorf("meshItemNames = %v, want %v", obj["meshItemNames"], want)
	}
}

func TestAliasParamsRewritesArraysOfObjects(t *testing.T) {
	raw := json.RawMessage(`{
		"items": [
			{"texture_path": "res://a.png", "position": 1},
			{"texture_path": "res://b.png", "children": [{"node_path": "root/X"}]}
		]
	}`)

	got, err := aliasParams(raw, parameterMappings)
	if err != nil {
		t.Fatalf("aliasParams: %v", err)
	}
	var obj map[string]any
	if err := json.Unmarshal(got, &obj); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	items, ok := obj["items"].([]any)
	if !ok || len(items) != 2 {
		t.Fatalf("items = %#v", obj["items"])
	}
	first, _ := items[0].(map[string]any)
	if first["texturePath"] != "res://a.png" {
		t.Errorf("first = %v", first)
	}
	second, _ := items[1].(map[string]any)
	children, ok := second["children"].([]any)
	if !ok || len(children) != 1 {
		t.Fatalf("children = %#v", second["children"])
	}
	child, _ := children[0].(map[string]any)
	if child["nodePath"] != "root/X" {
		t.Errorf("child = %v", child)
	}
}

func TestToSnakeCase(t *testing.T) {
	params := map[string]any{
		"scenePath":    "main.tscn",
		"rootNodeType": "Node2D",
		"properties": map[string]any{
			"texturePath": "res://icon.png",
			"position":    1,
		},
	}

	got := toSnakeCase(params)
	if got["scene_path"] != "main.tscn" || got["root_node_type"] != "Node2D" {
		t.Errorf("top-level = %v", got)
	}
	properties, ok := got["properties"].(map[string]any)
	if !ok {
		t.Fatalf("properties = %T", got["properties"])
	}
	if properties["texture_path"] != "res://icon.png" || properties["position"] != 1 {
		t.Errorf("nested = %v", properties)
	}
}

func TestToSnakeCaseGenericFallback(t *testing.T) {
	got := toSnakeCase(map[string]any{"someCustomProp": "x"})
	if got["some_custom_prop"] != "x" {
		t.Errorf("got = %v", got)
	}
}

func TestToSnakeCaseArraysOfObjects(t *testing.T) {
	params := map[string]any{
		"meshItemNames": []any{"a"},
		"properties": map[string]any{
			"items": []any{
				map[string]any{"texturePath": "res://a.png"},
				map[string]any{"nested": []any{map[string]any{"nodePath": "root/X"}}},
			},
		},
	}

	got := toSnakeCase(params)
	if names, ok := got["mesh_item_names"].([]any); !ok || len(names) != 1 || names[0] != "a" {
		t.Errorf("mesh_item_names = %#v", got["mesh_item_names"])
	}
	props, ok := got["properties"].(map[string]any)
	if !ok {
		t.Fatalf("properties = %T", got["properties"])
	}
	items, ok := props["items"].([]any)
	if !ok || len(items) != 2 {
		t.Fatalf("items = %#v", props["items"])
	}
	first, _ := items[0].(map[string]any)
	if first["texture_path"] != "res://a.png" {
		t.Errorf("first = %v", first)
	}
	second, _ := items[1].(map[string]any)
	nested, ok := second["nested"].([]any)
	if !ok || len(nested) != 1 {
		t.Fatalf("nested = %#v", second["nested"])
	}
	inner, _ := nested[0].(map[string]any)
	if inner["node_path"] != "root/X" {
		t.Errorf("inner = %v", inner)
	}
}
