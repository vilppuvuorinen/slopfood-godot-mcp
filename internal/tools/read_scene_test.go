package tools

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/vilppuvuorinen/go-dot-mcp/internal/godot"
)

func TestReadSceneSchema(t *testing.T) {
	tool := readScene()
	if tool.Name != "read_scene" {
		t.Errorf("name = %q", tool.Name)
	}
	if tool.Description != "Read scene file as JSON node tree (headless)" {
		t.Errorf("description = %q", tool.Description)
	}
	required, ok := tool.InputSchema["required"].([]string)
	if !ok || strings.Join(required, ",") != "scenePath" {
		t.Errorf("required = %v", tool.InputSchema["required"])
	}
	props, ok := tool.InputSchema["properties"].(map[string]any)
	if !ok {
		t.Fatalf("properties = %T", tool.InputSchema["properties"])
	}
	for _, key := range []string{"scenePath"} {
		prop, ok := props[key].(map[string]any)
		if !ok || prop["type"] != "string" {
			t.Errorf("property %q = %v, want string", key, props[key])
		}
	}
}

func TestReadSceneSuccess(t *testing.T) {
	t.Setenv(godot.PathEnv, fakeGodotOps(t, `printf '<<<GO_DOT_MCP_JSON_START>>>\n{"name":"root","type":"Node2D","children":[{"name":"Child","type":"Sprite2D"}]}\n<<<GO_DOT_MCP_JSON_END>>>\n'`))
	root := t.TempDir()
	makeProject(t, root)
	seedFile(t, filepath.Join(root, "scenes", "main.tscn"), "[gd_scene]\n")

	result := callToolArgs(t, readScene(), root, `{"scenePath":"scenes/main.tscn"}`)
	if result.IsError {
		t.Fatalf("unexpected error: %+v", result.Content)
	}
	if len(result.Content) != 1 {
		t.Fatalf("content = %+v", result.Content)
	}
	text := result.Content[0].Text
	for _, want := range []string{`"name": "root"`, `"type": "Node2D"`, `"name": "Child"`, `"type": "Sprite2D"`} {
		if !strings.Contains(text, want) {
			t.Errorf("text = %q, want it to contain %q", text, want)
		}
	}
	if strings.Contains(text, jsonStartMarker) || strings.Contains(text, jsonEndMarker) {
		t.Errorf("text still contains markers: %q", text)
	}
}

func TestReadScenePreservesNestedPropertyValues(t *testing.T) {
	t.Setenv(godot.PathEnv, fakeGodotOps(t, `printf '<<<GO_DOT_MCP_JSON_START>>>\n{"name":"root","type":"Node2D","properties":{"config":{"myKey":1,"nested":{"a":[1,2,3]}},"position":{"$type":"Vector2","x":0,"y":0}}}\n<<<GO_DOT_MCP_JSON_END>>>\n'`))
	root := t.TempDir()
	makeProject(t, root)
	seedFile(t, filepath.Join(root, "scenes", "main.tscn"), "[gd_scene]\n")

	result := callToolArgs(t, readScene(), root, `{"scenePath":"scenes/main.tscn"}`)
	if result.IsError {
		t.Fatalf("unexpected error: %+v", result.Content)
	}
	text := result.Content[0].Text
	for _, want := range []string{`"config": {`, `"myKey": 1`, `"a": [`, `"position": {`, `"$type": "Vector2"`} {
		if !strings.Contains(text, want) {
			t.Errorf("text = %q, want it to contain %q", text, want)
		}
	}

	structured, ok := result.StructuredContent.(map[string]any)
	if !ok {
		t.Fatalf("structuredContent = %T, want object", result.StructuredContent)
	}
	props, ok := structured["properties"].(map[string]any)
	if !ok {
		t.Fatalf("properties = %T, want object", structured["properties"])
	}
	config, ok := props["config"].(map[string]any)
	if !ok {
		t.Fatalf("config = %T, want nested object", props["config"])
	}
	if got := config["myKey"]; got != float64(1) {
		t.Errorf("config.myKey = %v, want 1", got)
	}
	position, ok := props["position"].(map[string]any)
	if !ok || position["$type"] != "Vector2" {
		t.Errorf("position = %#v, want a tagged Vector2 object", props["position"])
	}
}

func TestReadSceneRawWhenNoMarkers(t *testing.T) {
	t.Setenv(godot.PathEnv, fakeGodotOps(t, `echo "plain output"`))
	root := t.TempDir()
	makeProject(t, root)
	seedFile(t, filepath.Join(root, "main.tscn"), "[gd_scene]\n")

	result := callToolArgs(t, readScene(), root, `{"scenePath":"main.tscn"}`)
	if result.IsError {
		t.Fatalf("unexpected error: %+v", result.Content)
	}
	if !strings.Contains(result.Content[0].Text, "Scene read output:\nplain output") {
		t.Errorf("text = %q", result.Content[0].Text)
	}
}

func TestReadSceneRawWhenMarkersInvalid(t *testing.T) {
	t.Setenv(godot.PathEnv, fakeGodotOps(t, `printf '<<<GO_DOT_MCP_JSON_START>>>\nnot-json\n<<<GO_DOT_MCP_JSON_END>>>\n'`))
	root := t.TempDir()
	makeProject(t, root)
	seedFile(t, filepath.Join(root, "main.tscn"), "[gd_scene]\n")

	result := callToolArgs(t, readScene(), root, `{"scenePath":"main.tscn"}`)
	if result.IsError {
		t.Fatalf("unexpected error: %+v", result.Content)
	}
	text := result.Content[0].Text
	if !strings.Contains(text, "not-json") {
		t.Errorf("text = %q, want the raw invalid JSON payload", text)
	}
	if strings.Contains(text, jsonStartMarker) || strings.Contains(text, jsonEndMarker) {
		t.Errorf("text still contains markers: %q", text)
	}
}

func TestReadSceneValidation(t *testing.T) {
	root := t.TempDir()
	makeProject(t, root)
	notAProject := t.TempDir()

	tests := []struct {
		name    string
		project string
		args    string
		want    string
	}{
		{name: "missing paths", args: `{}`, want: "scenePath is required."},
		{name: "missing scene", project: root, args: `{}`, want: "scenePath is required."},
		{name: "invalid project path", project: "../escape", args: `{"scenePath":"main.tscn"}`, want: "Invalid path."},
		{name: "invalid scene path", project: root, args: `{"scenePath":"../escape.tscn"}`, want: "Invalid path."},
		{name: "not a project", project: notAProject, args: `{"scenePath":"main.tscn"}`, want: "Not a valid Godot project"},
		{name: "missing scene file", project: root, args: `{"scenePath":"missing.tscn"}`, want: "Scene file does not exist: missing.tscn"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := callToolArgs(t, readScene(), tt.project, tt.args)
			if !result.IsError {
				t.Fatalf("expected an error result: %+v", result.Content)
			}
			if !strings.HasPrefix(result.Content[0].Text, tt.want) {
				t.Errorf("message = %q, want prefix %q", result.Content[0].Text, tt.want)
			}
		})
	}
}

func TestReadSceneScriptFailure(t *testing.T) {
	t.Setenv(godot.PathEnv, fakeGodotOps(t, `echo 'Failed to load scene: boom' >&2; exit 1`))
	root := t.TempDir()
	makeProject(t, root)
	seedFile(t, filepath.Join(root, "main.tscn"), "[gd_scene]\n")

	result := callToolArgs(t, readScene(), root, `{"scenePath":"main.tscn"}`)
	if !result.IsError {
		t.Fatalf("expected an error result: %+v", result.Content)
	}
	if !strings.HasPrefix(result.Content[0].Text, "Failed to read scene: ") {
		t.Errorf("message = %q", result.Content[0].Text)
	}
	if len(result.Content) != 2 || !strings.HasPrefix(result.Content[1].Text, "Possible solutions:") {
		t.Errorf("solutions = %+v", result.Content)
	}
}

func TestReadSceneSpawnFailure(t *testing.T) {
	t.Setenv(godot.PathEnv, filepath.Join(t.TempDir(), "missing-godot"))
	root := t.TempDir()
	makeProject(t, root)
	seedFile(t, filepath.Join(root, "main.tscn"), "[gd_scene]\n")

	result := callToolArgs(t, readScene(), root, `{"scenePath":"main.tscn"}`)
	if !result.IsError {
		t.Fatalf("expected an error result: %+v", result.Content)
	}
	if !strings.HasPrefix(result.Content[0].Text, "Failed to read scene: ") {
		t.Errorf("message = %q", result.Content[0].Text)
	}
}
