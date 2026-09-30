package tools

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/vilppuvuorinen/slopfood-godot-mcp/internal/godot"
	"github.com/vilppuvuorinen/slopfood-godot-mcp/internal/mcp"
)

func TestCreateResourceSchema(t *testing.T) {
	tool := createResource()
	if tool.Name != "create_resource" {
		t.Errorf("name = %q", tool.Name)
	}
	if tool.Description != "Create a .tres/.res resource file" {
		t.Errorf("description = %q", tool.Description)
	}
	if !hasRequired(tool.InputSchema, "resourcePath", "resourceType") {
		t.Errorf("required = %v", tool.InputSchema["required"])
	}
	props, ok := tool.InputSchema["properties"].(map[string]any)
	if !ok {
		t.Fatalf("properties = %T", tool.InputSchema["properties"])
	}
	for _, key := range []string{"resourcePath", "resourceType"} {
		prop, propOK := props[key].(map[string]any)
		if !propOK || prop["type"] != "string" {
			t.Errorf("property %q = %v, want string", key, props[key])
		}
	}
	properties, ok := props["properties"].(map[string]any)
	if !ok || properties["type"] != "object" {
		t.Errorf("properties property = %v, want object", props["properties"])
	}
}

func TestModifyResourceSchema(t *testing.T) {
	tool := modifyResource()
	if tool.Name != "modify_resource" {
		t.Errorf("name = %q", tool.Name)
	}
	if tool.Description != "Set properties on a .tres/.res resource file" {
		t.Errorf("description = %q", tool.Description)
	}
	if !hasRequired(tool.InputSchema, "resourcePath", "properties") {
		t.Errorf("required = %v", tool.InputSchema["required"])
	}
	props, ok := tool.InputSchema["properties"].(map[string]any)
	if !ok {
		t.Fatalf("properties = %T", tool.InputSchema["properties"])
	}
	prop, propOK := props["resourcePath"].(map[string]any)
	if !propOK || prop["type"] != "string" {
		t.Errorf("resourcePath = %v, want string", props["resourcePath"])
	}
	properties, ok := props["properties"].(map[string]any)
	if !ok || properties["type"] != "object" {
		t.Errorf("properties property = %v, want object", props["properties"])
	}
}

func TestReadResourceSchema(t *testing.T) {
	tool := readBinaryResource()
	if tool.Name != "read_binary_resource" {
		t.Errorf("name = %q", tool.Name)
	}
	if tool.Description != "Read engine-decoded binary .res resource files (text .tres files are read directly)" {
		t.Errorf("description = %q", tool.Description)
	}
	if !hasRequired(tool.InputSchema, "resourcePath") {
		t.Errorf("required = %v", tool.InputSchema["required"])
	}
	props, ok := tool.InputSchema["properties"].(map[string]any)
	if !ok {
		t.Fatalf("properties = %T", tool.InputSchema["properties"])
	}
	if _, isMap := props["propertyFilter"].(map[string]any); !isMap {
		t.Errorf("read_binary_resource must advertise propertyFilter: %v", props["propertyFilter"])
	}
}

func TestReadResourceForwardsPropertyFilter(t *testing.T) {
	t.Setenv(godot.PathEnv, fakeGodotOps(t, `echo "$@"`))
	root := t.TempDir()
	makeProject(t, root)

	result := callToolArgs(t, readBinaryResource(), root, `{"resourcePath":"resources/mat.res","propertyFilter":["metallic","albedo_*"]}`)
	if result.IsError {
		t.Fatalf("unexpected error: %+v", result.Content)
	}
	text := result.Content[0].Text
	if !strings.Contains(text, `"propertyFilter":["metallic","albedo_*"]`) {
		t.Errorf("text = %q, want the property filter forwarded", text)
	}
}

func TestModifyResourceFilterIgnoredForMutation(t *testing.T) {
	t.Setenv(godot.PathEnv, fakeGodotOps(t, `echo "$@"`))
	root := t.TempDir()
	makeProject(t, root)

	result := callToolArgs(t, modifyResource(), root, `{"resourcePath":"resources/mat.tres","propertyFilter":["metallic"],"properties":{"metallic":0.5}}`)
	if result.IsError {
		t.Fatalf("unexpected error: %+v", result.Content)
	}
	if strings.Contains(result.Content[0].Text, `"propertyFilter"`) {
		t.Errorf("text = %q, want propertyFilter dropped for a modify", result.Content[0].Text)
	}
}

func TestReadResourceSuccess(t *testing.T) {
	t.Setenv(godot.PathEnv, fakeGodotOps(t, `echo "$@"`))
	root := t.TempDir()
	makeProject(t, root)

	result := callToolArgs(t, readBinaryResource(), root, `{"resourcePath":"resources/mat.res"}`)
	if result.IsError {
		t.Fatalf("unexpected error: %+v", result.Content)
	}
	if len(result.Content) != 1 {
		t.Fatalf("content = %+v", result.Content)
	}
	text := result.Content[0].Text
	if !strings.HasPrefix(text, "manage_resource succeeded.") {
		t.Errorf("text = %q", text)
	}
	for _, want := range []string{`"resourcePath":"resources/mat.res"`, `"action":"read"`} {
		if !strings.Contains(text, want) {
			t.Errorf("text = %q, want it to contain %q", text, want)
		}
	}
	if strings.Contains(text, `"properties"`) {
		t.Errorf("text = %q, want no properties for a read action", text)
	}
}

func TestReadResourceStructured(t *testing.T) {
	t.Setenv(godot.PathEnv, fakeGodotOps(t, `printf '<<<GODOT_MCP_JSON_START>>>\n{"type":"StandardMaterial3D","path":"resources/mat.res"}\n<<<GODOT_MCP_JSON_END>>>\n'`))
	root := t.TempDir()
	makeProject(t, root)

	result := callToolArgs(t, readBinaryResource(), root, `{"resourcePath":"resources/mat.res"}`)
	if result.IsError {
		t.Fatalf("unexpected error: %+v", result.Content)
	}
	text := result.Content[0].Text
	for _, want := range []string{`"type": "StandardMaterial3D"`, `"path": "resources/mat.res"`} {
		if !strings.Contains(text, want) {
			t.Errorf("text = %q, want it to contain %q", text, want)
		}
	}
	if strings.Contains(text, jsonStartMarker) || strings.Contains(text, jsonEndMarker) {
		t.Errorf("text still contains markers: %q", text)
	}
	if strings.Contains(text, "succeeded.\n\nOutput:") {
		t.Errorf("text = %q, want the structured payload without the legacy wrapper", text)
	}
}

func TestCreateResourceForwardsFields(t *testing.T) {
	t.Setenv(godot.PathEnv, fakeGodotOps(t, `echo "$@"`))
	root := t.TempDir()
	makeProject(t, root)

	result := callToolArgs(t, createResource(), root, `{"resourcePath":"materials/red.tres","resourceType":"StandardMaterial3D"}`)
	if result.IsError {
		t.Fatalf("unexpected error: %+v", result.Content)
	}
	if len(result.Content) != 1 {
		t.Fatalf("content = %+v", result.Content)
	}
	text := result.Content[0].Text
	if !strings.HasPrefix(text, "manage_resource succeeded.") {
		t.Errorf("text = %q", text)
	}
	for _, want := range []string{`"action":"create"`, `"resourceType":"StandardMaterial3D"`, `"resourcePath":"materials/red.tres"`} {
		if !strings.Contains(text, want) {
			t.Errorf("text = %q, want it to contain %q", text, want)
		}
	}
	if strings.Contains(text, `"properties"`) {
		t.Errorf("text = %q, want no properties", text)
	}
}

func TestCreateResourceForwardsProperties(t *testing.T) {
	t.Setenv(godot.PathEnv, fakeGodotOps(t, `echo "$@"`))
	root := t.TempDir()
	makeProject(t, root)

	result := callToolArgs(t, createResource(), root, `{"resourcePath":"materials/red.tres","resourceType":"StandardMaterial3D","properties":{"albedo_color":{"r":1,"g":0,"b":0,"a":1}}}`)
	if result.IsError {
		t.Fatalf("unexpected error: %+v", result.Content)
	}
	text := result.Content[0].Text
	if !strings.Contains(text, `"properties":{"albedo_color":{"a":1,"b":0,"g":0,"r":1}}`) {
		t.Errorf("text = %q, want the properties to round-trip", text)
	}
}

func TestModifyResourceForwardsProperties(t *testing.T) {
	t.Setenv(godot.PathEnv, fakeGodotOps(t, `echo "$@"`))
	root := t.TempDir()
	makeProject(t, root)

	result := callToolArgs(t, modifyResource(), root, `{"resourcePath":"resources/mat.tres","properties":{"metallic":0.9}}`)
	if result.IsError {
		t.Fatalf("unexpected error: %+v", result.Content)
	}
	text := result.Content[0].Text
	for _, want := range []string{`"action":"modify"`, `"properties":{"metallic":0.9}`} {
		if !strings.Contains(text, want) {
			t.Errorf("text = %q, want it to contain %q", text, want)
		}
	}
}

func TestResourceValidation(t *testing.T) {
	root := t.TempDir()
	makeProject(t, root)
	notAProject := t.TempDir()

	tests := []struct {
		name    string
		tool    func() mcp.Tool
		project string
		args    string
		want    string
	}{
		{name: "create missing params", tool: createResource, args: `{}`, want: "resourcePath and resourceType are required."},
		{name: "create missing resource type", tool: createResource, project: root, args: `{"resourcePath":"mat.tres"}`, want: "resourcePath and resourceType are required."},
		{name: "modify missing params", tool: modifyResource, args: `{}`, want: "resourcePath and properties are required."},
		{name: "modify missing properties", tool: modifyResource, project: root, args: `{"resourcePath":"mat.tres"}`, want: "resourcePath and properties are required."},
		{name: "modify empty properties", tool: modifyResource, project: root, args: `{"resourcePath":"mat.tres","properties":{}}`, want: "resourcePath and properties are required."},
		{name: "read missing resource path", tool: readBinaryResource, project: root, args: `{}`, want: "resourcePath is required."},
		{name: "read text .tres rejected", tool: readBinaryResource, project: root, args: `{"resourcePath":"mat.tres"}`, want: "read_binary_resource reads binary .res resources"},
		{name: "invalid project path", tool: readBinaryResource, project: "../escape", args: `{"resourcePath":"mat.res"}`, want: "Invalid path."},
		{name: "invalid resource path", tool: readBinaryResource, project: root, args: `{"resourcePath":"../escape.res"}`, want: "Invalid path."},
		{name: "not a project", tool: readBinaryResource, project: notAProject, args: `{"resourcePath":"mat.res"}`, want: "Not a valid Godot project"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := callToolArgs(t, tt.tool(), tt.project, tt.args)
			if !result.IsError {
				t.Fatalf("expected an error result: %+v", result.Content)
			}
			if !strings.HasPrefix(result.Content[0].Text, tt.want) {
				t.Errorf("message = %q, want prefix %q", result.Content[0].Text, tt.want)
			}
		})
	}
}

func TestResourceScriptFailure(t *testing.T) {
	t.Setenv(godot.PathEnv, fakeGodotOps(t, `echo 'Failed to load resource: boom' >&2; exit 1`))
	root := t.TempDir()
	makeProject(t, root)

	result := callToolArgs(t, readBinaryResource(), root, `{"resourcePath":"mat.res"}`)
	if !result.IsError {
		t.Fatalf("expected an error result: %+v", result.Content)
	}
	if !strings.HasPrefix(result.Content[0].Text, "manage_resource failed: ") {
		t.Errorf("message = %q", result.Content[0].Text)
	}
	if len(result.Content) != 2 || !strings.HasPrefix(result.Content[1].Text, "Possible solutions:") {
		t.Errorf("solutions = %+v", result.Content)
	}
}

func TestResourceSpawnFailure(t *testing.T) {
	t.Setenv(godot.PathEnv, filepath.Join(t.TempDir(), "missing-godot"))
	root := t.TempDir()
	makeProject(t, root)

	result := callToolArgs(t, readBinaryResource(), root, `{"resourcePath":"mat.res"}`)
	if !result.IsError {
		t.Fatalf("expected an error result: %+v", result.Content)
	}
	if !strings.HasPrefix(result.Content[0].Text, "manage_resource failed: ") {
		t.Errorf("message = %q", result.Content[0].Text)
	}
}
