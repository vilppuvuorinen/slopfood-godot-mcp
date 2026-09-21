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
