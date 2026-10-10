package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/vilppuvuorinen/slopfood-godot-mcp/internal/godot"
	"github.com/vilppuvuorinen/slopfood-godot-mcp/internal/mcp"
)

func TestReadSchema(t *testing.T) {
	tool := newToolset("", nil, nil).read()
	if tool.Name != "read" {
		t.Errorf("name = %q", tool.Name)
	}
	if tool.Title == "" {
		t.Error("title is empty")
	}
	if tool.Description != "Read a scene, animation, tileset, theme, or binary resource file; use read_node to read a node subtree. depth applies to scenes, propertyFilter to resources" {
		t.Errorf("description = %q", tool.Description)
	}
	if !hasRequired(tool.InputSchema, "type", "path") {
		t.Errorf("required = %v", tool.InputSchema["required"])
	}
	props, ok := tool.InputSchema["properties"].(map[string]any)
	if !ok {
		t.Fatalf("properties = %T", tool.InputSchema["properties"])
	}
	if got := len(props); got != 4 {
		t.Errorf("properties = %d, want 4", got)
	}
	for _, key := range []string{"type", "path"} {
		prop, isMap := props[key].(map[string]any)
		if !isMap || prop["type"] != "string" {
			t.Errorf("property %q = %v, want string", key, props[key])
		}
	}
	enum, ok := props["type"].(map[string]any)["enum"].([]string)
	if !ok || !reflect.DeepEqual(enum, readTypes) {
		t.Errorf("type enum = %v, want %v", props["type"], readTypes)
	}
	depth, ok := props["depth"].(map[string]any)
	if !ok || depth["type"] != "integer" {
		t.Errorf("depth = %v, want integer", props["depth"])
	}
	filter, ok := props["propertyFilter"].(map[string]any)
	if !ok || filter["type"] != "array" {
		t.Errorf("propertyFilter = %v, want array", props["propertyFilter"])
	}
}

// callReadSession drives read() with the recording fake session so tests can
// assert the forwarded bridge operation and params.
func callReadSession(t *testing.T, project, args string) (*fakeSession, mcp.ToolResult) {
	t.Helper()
	sess := &fakeSession{}
	out, err := newToolset(project, sess, nil).read().Handler(context.Background(), json.RawMessage(args))
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	result, ok := out.(mcp.ToolResult)
	if !ok {
		t.Fatalf("result type %T, want mcp.ToolResult", out)
	}
	return sess, result
}

func TestReadDispatchMapping(t *testing.T) {
	root := t.TempDir()
	makeProject(t, root)
	seedFile(t, filepath.Join(root, "main.tscn"), "[gd_scene]\n")

	tests := []struct {
		name   string
		args   string
		op     string
		params map[string]any
	}{
		{name: "scene", args: `{"type":"scene","path":"main.tscn","depth":1}`, op: "read_scene", params: map[string]any{"scenePath": "main.tscn", "depth": 1}},
		{name: "scene without depth", args: `{"type":"scene","path":"main.tscn"}`, op: "read_scene", params: map[string]any{"scenePath": "main.tscn"}},
		{name: "animation", args: `{"type":"animation","path":"anims/walk.tres"}`, op: "read_animation", params: map[string]any{"resourcePath": "anims/walk.tres"}},
		{name: "tileset", args: `{"type":"tileset","path":"tiles/set.tres"}`, op: "read_tileset", params: map[string]any{"resourcePath": "tiles/set.tres"}},
		{name: "theme", args: `{"type":"theme","path":"themes/main.tres"}`, op: "read_theme", params: map[string]any{"resourcePath": "themes/main.tres"}},
		{name: "resource", args: `{"type":"resource","path":"resources/data.res"}`, op: "read_resource", params: map[string]any{"resourcePath": "resources/data.res"}},
		{name: "resource with filter", args: `{"type":"resource","path":"resources/data.res","propertyFilter":["name","value"]}`, op: "read_resource", params: map[string]any{"resourcePath": "resources/data.res", "propertyFilter": []string{"name", "value"}}},
		{name: "resource empty filter is omitted", args: `{"type":"resource","path":"resources/data.res","propertyFilter":[]}`, op: "read_resource", params: map[string]any{"resourcePath": "resources/data.res"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sess, _ := callReadSession(t, root, tt.args)
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

func TestReadToolForwardsScopeOptions(t *testing.T) {
	t.Setenv(godot.PathEnv, fakeGodotOps(t, `echo "$@"`))
	root := t.TempDir()
	makeProject(t, root)
	seedFile(t, filepath.Join(root, "scenes", "main.tscn"), "[gd_scene]\n")

	result := callToolArgs(t, (*toolset).read, root, `{"type":"scene","path":"scenes/main.tscn","depth":1}`)
	if !result.IsError {
		t.Fatalf("expected the markerless echo to fail: %+v", result.Content)
	}
	text := result.Content[0].Text
	for _, want := range []string{`"scenePath":"scenes/main.tscn"`, `"depth":1`} {
		if !strings.Contains(text, want) {
			t.Errorf("text = %q, want it to contain %q", text, want)
		}
	}
}

func TestReadValidation(t *testing.T) {
	root := t.TempDir()
	makeProject(t, root)
	notAProject := t.TempDir()

	tests := []struct {
		name    string
		project string
		args    string
		want    string
	}{
		{name: "missing type", args: `{}`, want: "type is required."},
		{name: "unknown type", project: root, args: `{"type":"nope","path":"x.tscn"}`, want: "Invalid type: nope."},
		{name: "missing path", project: root, args: `{"type":"scene"}`, want: "path is required."},
		{name: "nodePath rejected", project: root, args: `{"type":"scene","path":"main.tscn","nodePath":"root"}`, want: "read does not accept nodePath."},
		{name: "depth on animation", project: root, args: `{"type":"animation","path":"a.tres","depth":1}`, want: "depth applies only to type=scene."},
		{name: "propertyFilter on scene", project: root, args: `{"type":"scene","path":"main.tscn","propertyFilter":["x"]}`, want: "propertyFilter applies only to type=resource."},
		{name: "negative depth", project: root, args: `{"type":"scene","path":"main.tscn","depth":-1}`, want: "depth must be zero or greater."},
		{name: "invalid project path", project: "../escape", args: `{"type":"animation","path":"a.tres"}`, want: "Invalid path."},
		{name: "invalid resource path", project: root, args: `{"type":"animation","path":"../escape.tres"}`, want: "Invalid path."},
		{name: "absolute resource path", project: root, args: `{"type":"animation","path":"/etc/passwd.tres"}`, want: "Absolute paths are not allowed."},
		{name: "normalized parent resource path", project: root, args: `{"type":"animation","path":"foo/../bar.tres"}`, want: "Invalid path."},
		{name: "scene parent path", project: root, args: `{"type":"scene","path":"foo/../bar.tscn"}`, want: "Invalid path."},
		{name: "scene escape path", project: root, args: `{"type":"scene","path":"../escape.tscn"}`, want: "Invalid path."},
		{name: "missing scene file", project: root, args: `{"type":"scene","path":"missing.tscn"}`, want: "Scene file does not exist: missing.tscn"},
		{name: "not a project", project: notAProject, args: `{"type":"animation","path":"a.tres"}`, want: "Not a valid Godot project"},
		{name: "resource rejects tres", project: root, args: `{"type":"resource","path":"foo.tres"}`, want: "read(type=resource) reads binary .res resources; foo.tres is a text .tres file."},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := callToolArgs(t, (*toolset).read, tt.project, tt.args)
			if !result.IsError {
				t.Fatalf("expected an error result: %+v", result.Content)
			}
			if !strings.HasPrefix(result.Content[0].Text, tt.want) {
				t.Errorf("message = %q, want prefix %q", result.Content[0].Text, tt.want)
			}
		})
	}
}

// TestReadResourcePathRejections carries the step-1 tightened contract across
// every resource type, since the old handler tests are deleted in step 7.
func TestReadResourcePathRejections(t *testing.T) {
	root := t.TempDir()
	makeProject(t, root)

	for _, readType := range []string{"animation", "tileset", "theme", "resource"} {
		for _, tt := range []struct{ path, want string }{
			{path: "/etc/passwd.tres", want: "Absolute paths are not allowed."},
			{path: "foo/../bar.tres", want: "Invalid path."},
			{path: "user://save.tres", want: "user:// paths are not supported."},
		} {
			t.Run(readType+" "+tt.path, func(t *testing.T) {
				result := callToolArgs(t, (*toolset).read, root, fmt.Sprintf(`{"type":%q,"path":%q}`, readType, tt.path))
				if !result.IsError {
					t.Fatalf("expected an error result: %+v", result.Content)
				}
				if !strings.HasPrefix(result.Content[0].Text, tt.want) {
					t.Errorf("message = %q, want prefix %q", result.Content[0].Text, tt.want)
				}
			})
		}
	}
}

func TestReadToolSuccess(t *testing.T) {
	t.Setenv(godot.PathEnv, fakeGodotOps(t, `printf '<<<GODOT_MCP_JSON_START>>>\n{"kind":"scene","path":"scenes/main.tscn","data":{"root":{"name":"root","type":"Node2D","children":[{"name":"Child","type":"Sprite2D","children":[]}]},"connections":[]}}\n<<<GODOT_MCP_JSON_END>>>\n'`))
	root := t.TempDir()
	makeProject(t, root)
	seedFile(t, filepath.Join(root, "scenes", "main.tscn"), "[gd_scene]\n")

	result := callToolArgs(t, (*toolset).read, root, `{"type":"scene","path":"scenes/main.tscn"}`)
	if result.IsError {
		t.Fatalf("unexpected error: %+v", result.Content)
	}
	if len(result.Content) != 1 {
		t.Fatalf("content = %+v", result.Content)
	}
	text := result.Content[0].Text
	for _, want := range []string{`"kind": "scene"`, `"path": "scenes/main.tscn"`, `"name": "root"`, `"type": "Node2D"`, `"name": "Child"`, `"type": "Sprite2D"`, `"connections": []`} {
		if !strings.Contains(text, want) {
			t.Errorf("text = %q, want it to contain %q", text, want)
		}
	}
	if strings.Contains(text, jsonStartMarker) || strings.Contains(text, jsonEndMarker) {
		t.Errorf("text still contains markers: %q", text)
	}
}

func TestReadToolPreservesNestedPropertyValues(t *testing.T) {
	t.Setenv(godot.PathEnv, fakeGodotOps(t, `printf '<<<GODOT_MCP_JSON_START>>>\n{"kind":"scene","path":"scenes/main.tscn","data":{"root":{"name":"root","type":"Node2D","properties":{"config":{"myKey":1,"nested":{"a":[1,2,3]}},"position":{"$type":"Vector2","x":0,"y":0}},"children":[]},"connections":[]}}\n<<<GODOT_MCP_JSON_END>>>\n'`))
	root := t.TempDir()
	makeProject(t, root)
	seedFile(t, filepath.Join(root, "scenes", "main.tscn"), "[gd_scene]\n")

	result := callToolArgs(t, (*toolset).read, root, `{"type":"scene","path":"scenes/main.tscn"}`)
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
	data, ok := structured["data"].(map[string]any)
	if !ok {
		t.Fatalf("data = %T, want object", structured["data"])
	}
	rootNode, ok := data["root"].(map[string]any)
	if !ok {
		t.Fatalf("data.root = %T, want object", data["root"])
	}
	props, ok := rootNode["properties"].(map[string]any)
	if !ok {
		t.Fatalf("properties = %T, want object", rootNode["properties"])
	}
	if got := props["config"].(map[string]any)["myKey"]; got != float64(1) {
		t.Errorf("config.myKey = %v, want 1", got)
	}
	position, ok := props["position"].(map[string]any)
	if !ok || position["$type"] != "Vector2" {
		t.Errorf("position = %#v, want a tagged Vector2 object", props["position"])
	}
}

func TestReadToolNoMarkersIsError(t *testing.T) {
	t.Setenv(godot.PathEnv, fakeGodotOps(t, `echo "plain output"`))
	root := t.TempDir()
	makeProject(t, root)
	seedFile(t, filepath.Join(root, "main.tscn"), "[gd_scene]\n")

	result := callToolArgs(t, (*toolset).read, root, `{"type":"scene","path":"main.tscn"}`)
	if !result.IsError {
		t.Fatalf("expected an error result: %+v", result.Content)
	}
	if !strings.HasPrefix(result.Content[0].Text, "Failed to read scene: ") {
		t.Errorf("message = %q", result.Content[0].Text)
	}
	if !strings.Contains(result.Content[0].Text, "plain output") {
		t.Errorf("message = %q, want the raw engine output for inspection", result.Content[0].Text)
	}
}

func TestReadToolInvalidJSONIsError(t *testing.T) {
	t.Setenv(godot.PathEnv, fakeGodotOps(t, `printf '<<<GODOT_MCP_JSON_START>>>\nnot-json\n<<<GODOT_MCP_JSON_END>>>\n'`))
	root := t.TempDir()
	makeProject(t, root)
	seedFile(t, filepath.Join(root, "main.tscn"), "[gd_scene]\n")

	result := callToolArgs(t, (*toolset).read, root, `{"type":"scene","path":"main.tscn"}`)
	if !result.IsError {
		t.Fatalf("expected an error result: %+v", result.Content)
	}
	text := result.Content[0].Text
	if !strings.HasPrefix(text, "Failed to read scene: ") || !strings.Contains(text, "not-json") {
		t.Errorf("message = %q, want the failure prefix and the invalid payload", text)
	}
	if strings.Contains(text, jsonStartMarker) || strings.Contains(text, jsonEndMarker) {
		t.Errorf("message still contains markers: %q", text)
	}
}

func TestReadToolScriptFailure(t *testing.T) {
	t.Setenv(godot.PathEnv, fakeGodotOps(t, `echo 'Failed to load scene: boom' >&2; exit 1`))
	root := t.TempDir()
	makeProject(t, root)
	seedFile(t, filepath.Join(root, "main.tscn"), "[gd_scene]\n")

	result := callToolArgs(t, (*toolset).read, root, `{"type":"scene","path":"main.tscn"}`)
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

func TestReadToolSpawnFailure(t *testing.T) {
	t.Setenv(godot.PathEnv, filepath.Join(t.TempDir(), "missing-godot"))
	root := t.TempDir()
	makeProject(t, root)
	seedFile(t, filepath.Join(root, "main.tscn"), "[gd_scene]\n")

	result := callToolArgs(t, (*toolset).read, root, `{"type":"scene","path":"main.tscn"}`)
	if !result.IsError {
		t.Fatalf("expected an error result: %+v", result.Content)
	}
	if !strings.HasPrefix(result.Content[0].Text, "Failed to read scene: ") {
		t.Errorf("message = %q", result.Content[0].Text)
	}
}

func TestReadResourceStructured(t *testing.T) {
	tests := []struct {
		name string
		body string
		args string
		want string
	}{
		{
			name: "animation",
			body: `printf '<<<GODOT_MCP_JSON_START>>>\n{"kind":"animation","path":"anims/walk.tres","data":{"length":2,"loopMode":"linear","tracks":[]}}\n<<<GODOT_MCP_JSON_END>>>\n'`,
			args: `{"type":"animation","path":"anims/walk.tres"}`,
			want: `"kind": "animation"`,
		},
		{
			name: "tileset",
			body: `printf '<<<GODOT_MCP_JSON_START>>>\n{"kind":"tileset","path":"tiles/set.tres","data":{"tileShape":"square","sources":[]}}\n<<<GODOT_MCP_JSON_END>>>\n'`,
			args: `{"type":"tileset","path":"tiles/set.tres"}`,
			want: `"kind": "tileset"`,
		},
		{
			name: "theme",
			body: `printf '<<<GODOT_MCP_JSON_START>>>\n{"kind":"theme","path":"themes/main.tres","data":{"defaultFontSize":16,"types":[]}}\n<<<GODOT_MCP_JSON_END>>>\n'`,
			args: `{"type":"theme","path":"themes/main.tres"}`,
			want: `"kind": "theme"`,
		},
		{
			name: "resource",
			body: `printf '<<<GODOT_MCP_JSON_START>>>\n{"kind":"resource","path":"resources/data.res","data":{"class":"Resource","properties":{"value":1}}}\n<<<GODOT_MCP_JSON_END>>>\n'`,
			args: `{"type":"resource","path":"resources/data.res"}`,
			want: `"kind": "resource"`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv(godot.PathEnv, fakeGodotOps(t, tt.body))
			root := t.TempDir()
			makeProject(t, root)

			result := callToolArgs(t, (*toolset).read, root, tt.args)
			if result.IsError {
				t.Fatalf("unexpected error: %+v", result.Content)
			}
			text := result.Content[0].Text
			if !strings.Contains(text, tt.want) {
				t.Errorf("text = %q, want it to contain %q", text, tt.want)
			}
			if strings.Contains(text, "succeeded.\n\nOutput:") {
				t.Errorf("text = %q, want the structured payload without the legacy wrapper", text)
			}
		})
	}
}

func TestReadResourceFallbackUsesEnvelopeEcho(t *testing.T) {
	t.Setenv(godot.PathEnv, fakeGodotOps(t, `echo "$@"`))
	root := t.TempDir()
	makeProject(t, root)

	result := callToolArgs(t, (*toolset).read, root, `{"type":"animation","path":"anims/walk.tres"}`)
	if result.IsError {
		t.Fatalf("unexpected error: %+v", result.Content)
	}
	if !strings.Contains(result.Content[0].Text, `read_animation`) {
		t.Errorf("text = %q, want the operation echoed", result.Content[0].Text)
	}
	structured, ok := result.StructuredContent.(map[string]any)
	if !ok {
		t.Fatalf("structuredContent = %T, want object", result.StructuredContent)
	}
	if got := structured["path"]; got != "anims/walk.tres" {
		t.Errorf("path = %v, want the original path", got)
	}
	if got := structured["kind"]; got != "animation" {
		t.Errorf("kind = %v, want animation", got)
	}
	if _, ok := structured["resourcePath"]; ok {
		t.Errorf("structuredContent still uses the bridge key: %v", structured)
	}
}

func TestReadOutputSchemaAcceptsEnvelopeAndStatus(t *testing.T) {
	tool := newToolset("", nil, nil).read()
	value := jsonValue(t, map[string]any{
		"kind": "resource",
		"path": "resources/data.res",
		"data": map[string]any{"class": "Resource", "properties": map[string]any{}},
	})
	if msg := schemaViolation(tool.OutputSchema, tool.OutputSchema, value, tool.Name); msg != "" {
		t.Errorf("envelope rejected: %s", msg)
	}
	status := jsonValue(t, mcp.ErrorResult("boom", "try again").StructuredContent)
	if msg := schemaViolation(tool.OutputSchema, tool.OutputSchema, status, tool.Name); msg != "" {
		t.Errorf("status rejected: %s", msg)
	}
	if msg := schemaViolation(tool.OutputSchema, tool.OutputSchema, jsonValue(t, map[string]any{"kind": "resource"}), tool.Name); msg == "" {
		t.Error("incomplete envelope accepted")
	}
}

func TestReadResourceScriptFailure(t *testing.T) {
	tests := []struct {
		name   string
		body   string
		args   string
		prefix string
	}{
		{"animation", `echo 'Animation resource not found: boom' >&2; exit 1`, `{"type":"animation","path":"anim.tres"}`, "read_animation failed: "},
		{"tileset", `echo 'TileSet resource not found: boom' >&2; exit 1`, `{"type":"tileset","path":"set.tres"}`, "read_tileset failed: "},
		{"theme", `echo 'Theme resource not found: boom' >&2; exit 1`, `{"type":"theme","path":"main.tres"}`, "read_theme failed: "},
		{"resource", `echo 'Resource not found: boom' >&2; exit 1`, `{"type":"resource","path":"data.res"}`, "read_resource failed: "},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv(godot.PathEnv, fakeGodotOps(t, tt.body))
			root := t.TempDir()
			makeProject(t, root)

			result := callToolArgs(t, (*toolset).read, root, tt.args)
			if !result.IsError {
				t.Fatalf("expected an error result: %+v", result.Content)
			}
			if !strings.HasPrefix(result.Content[0].Text, tt.prefix) {
				t.Errorf("message = %q, want prefix %q", result.Content[0].Text, tt.prefix)
			}
			if len(result.Content) != 2 || !strings.HasPrefix(result.Content[1].Text, "Possible solutions:") {
				t.Errorf("solutions = %+v", result.Content)
			}
		})
	}
}
