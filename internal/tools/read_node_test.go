package tools

import (
	"context"
	"encoding/json"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/vilppuvuorinen/slopfood-godot-mcp/internal/godot"
	"github.com/vilppuvuorinen/slopfood-godot-mcp/internal/mcp"
)

func TestReadNodeSchema(t *testing.T) {
	tool := readNode()
	if tool.Name != "read_node" {
		t.Errorf("name = %q", tool.Name)
	}
	if tool.Title == "" {
		t.Error("title is empty")
	}
	if !hasRequired(tool.InputSchema, "type", "path", "nodePath") {
		t.Errorf("required = %v", tool.InputSchema["required"])
	}
	props, ok := tool.InputSchema["properties"].(map[string]any)
	if !ok {
		t.Fatalf("properties = %T", tool.InputSchema["properties"])
	}
	if got := len(props); got != 4 {
		t.Errorf("properties = %d, want 4", got)
	}
	for _, key := range []string{"type", "path", "nodePath"} {
		prop, isMap := props[key].(map[string]any)
		if !isMap || prop["type"] != "string" {
			t.Errorf("property %q = %v, want string", key, props[key])
		}
	}
	enum, ok := props["type"].(map[string]any)["enum"].([]string)
	if !ok || !reflect.DeepEqual(enum, readNodeTypes) {
		t.Errorf("type enum = %v, want %v", props["type"], readNodeTypes)
	}
	depth, ok := props["depth"].(map[string]any)
	if !ok || depth["type"] != "integer" {
		t.Errorf("depth = %v, want integer", props["depth"])
	}
}

// callReadNodeSession drives read_node() with the recording fake session so
// tests can assert the forwarded bridge operation and params.
func callReadNodeSession(t *testing.T, project, args string) (*fakeSession, mcp.ToolResult) {
	t.Helper()
	sess := &fakeSession{}
	ctx := WithSession(context.Background(), sess)
	if project != "" {
		ctx = withProject(ctx, project)
	}
	out, err := readNode().Handler(ctx, json.RawMessage(args))
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	result, ok := out.(mcp.ToolResult)
	if !ok {
		t.Fatalf("result type %T, want mcp.ToolResult", out)
	}
	return sess, result
}

func TestReadNodeDispatchMapping(t *testing.T) {
	root := t.TempDir()
	makeProject(t, root)
	seedFile(t, filepath.Join(root, "scenes", "main.tscn"), "[gd_scene]\n")

	tests := []struct {
		name   string
		args   string
		op     string
		params map[string]any
	}{
		{name: "scene node subtree", args: `{"type":"scene","path":"scenes/main.tscn","nodePath":"root/Player","depth":1}`, op: "read_scene", params: map[string]any{"scenePath": "scenes/main.tscn", "nodePath": "root/Player", "depth": 1}},
		{name: "scene without depth", args: `{"type":"scene","path":"scenes/main.tscn","nodePath":"root/Player"}`, op: "read_scene", params: map[string]any{"scenePath": "scenes/main.tscn", "nodePath": "root/Player"}},
		{name: "tilemap", args: `{"type":"tilemap","path":"scenes/main.tscn","nodePath":"root/Ground"}`, op: "read_tilemap", params: map[string]any{"scenePath": "scenes/main.tscn", "nodePath": "root/Ground"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sess, _ := callReadNodeSession(t, root, tt.args)
			if len(sess.runs) != 1 {
				t.Fatalf("session runs = %d, want 1", len(sess.runs))
			}
			run := sess.runs[0]
			if run.operation != tt.op {
				t.Errorf("operation = %q, want %q", run.operation, tt.op)
			}
			if !reflect.DeepEqual(run.params, tt.params) {
				t.Errorf("params = %#v, want %#v", run.params, tt.params)
			}
		})
	}
}

func TestReadNodeTilemapStructured(t *testing.T) {
	t.Setenv(godot.PathEnv, fakeGodotOps(t, `printf '<<<GODOT_MCP_JSON_START>>>\n{"kind":"tilemap","path":"scenes/level.tscn","nodePath":"root/Ground","data":{"tileSet":"tiles/set.tres","cells":[{"sourceId":0}]}}\n<<<GODOT_MCP_JSON_END>>>\n'`))
	root := t.TempDir()
	makeProject(t, root)
	seedFile(t, filepath.Join(root, "scenes", "level.tscn"), "[gd_scene format=3]\n")

	result := callToolArgs(t, readNode(), root, `{"type":"tilemap","path":"scenes/level.tscn","nodePath":"root/Ground"}`)
	if result.IsError {
		t.Fatalf("unexpected error: %+v", result.Content)
	}
	text := result.Content[0].Text
	for _, want := range []string{`"kind": "tilemap"`, `"nodePath": "root/Ground"`, `"tileSet": "tiles/set.tres"`, `"cells": [`, `"sourceId": 0`} {
		if !strings.Contains(text, want) {
			t.Errorf("text = %q, want it to contain %q", text, want)
		}
	}
	if strings.Contains(text, `"usedCells"`) {
		t.Errorf("text = %q, want the cells key", text)
	}
}

func TestReadNodeOutputSchemaAcceptsEnvelopeAndStatus(t *testing.T) {
	tool := readNode()
	value := jsonValue(t, map[string]any{
		"kind":     "tilemap",
		"path":     "scenes/level.tscn",
		"nodePath": "root/Ground",
		"data":     map[string]any{"tileSet": "tiles/set.tres", "cells": []any{}},
	})
	if msg := schemaViolation(tool.OutputSchema, tool.OutputSchema, value, tool.Name); msg != "" {
		t.Errorf("envelope rejected: %s", msg)
	}
	status := jsonValue(t, mcp.ErrorResult("boom", "try again").StructuredContent)
	if msg := schemaViolation(tool.OutputSchema, tool.OutputSchema, status, tool.Name); msg != "" {
		t.Errorf("status rejected: %s", msg)
	}
	if msg := schemaViolation(tool.OutputSchema, tool.OutputSchema, jsonValue(t, map[string]any{
		"kind": "tilemap", "path": "scenes/level.tscn", "data": map[string]any{},
	}), tool.Name); msg == "" {
		t.Error("envelope missing nodePath accepted")
	}
}

func TestReadNodeTilemapScriptFailure(t *testing.T) {
	t.Setenv(godot.PathEnv, fakeGodotOps(t, `echo 'Node is not a TileMapLayer: root/Ground' >&2; exit 1`))
	root := t.TempDir()
	makeProject(t, root)
	seedFile(t, filepath.Join(root, "scenes", "level.tscn"), "[gd_scene format=3]\n")

	result := callToolArgs(t, readNode(), root, `{"type":"tilemap","path":"scenes/level.tscn","nodePath":"root/Ground"}`)
	if !result.IsError {
		t.Fatalf("expected an error result: %+v", result.Content)
	}
	if !strings.HasPrefix(result.Content[0].Text, "read_tilemap failed: ") {
		t.Errorf("message = %q", result.Content[0].Text)
	}
}

func TestReadNodeValidation(t *testing.T) {
	root := t.TempDir()
	makeProject(t, root)

	tests := []struct {
		name    string
		project string
		args    string
		want    string
	}{
		{name: "missing type", args: `{}`, want: "type is required."},
		{name: "unknown type", project: root, args: `{"type":"nope","path":"main.tscn","nodePath":"root"}`, want: "Invalid type: nope."},
		{name: "missing path", project: root, args: `{"type":"scene","nodePath":"root"}`, want: "path is required."},
		{name: "missing nodePath", project: root, args: `{"type":"scene","path":"main.tscn"}`, want: "nodePath is required."},
		{name: "depth on tilemap", project: root, args: `{"type":"tilemap","path":"main.tscn","nodePath":"root/Ground","depth":1}`, want: "depth applies only to type=scene."},
		{name: "negative scene depth", project: root, args: `{"type":"scene","path":"main.tscn","nodePath":"root","depth":-1}`, want: "depth must be zero or greater."},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := callToolArgs(t, readNode(), tt.project, tt.args)
			if !result.IsError {
				t.Fatalf("expected an error result: %+v", result.Content)
			}
			if !strings.HasPrefix(result.Content[0].Text, tt.want) {
				t.Errorf("message = %q, want prefix %q", result.Content[0].Text, tt.want)
			}
		})
	}
}
