package tools

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/vilppuvuorinen/go-dot-mcp/internal/godot"
	"github.com/vilppuvuorinen/go-dot-mcp/internal/mcp"
)

func TestManageResourceSchema(t *testing.T) {
	tool := manageResource()
	if tool.Name != "manage_resource" {
		t.Errorf("name = %q", tool.Name)
	}
	if tool.Description != "Create or modify .tres/.res resource files" {
		t.Errorf("description = %q", tool.Description)
	}
	required, ok := tool.InputSchema["required"].([]string)
	if !ok || strings.Join(required, ",") != "resourcePath,action" {
		t.Errorf("required = %v", tool.InputSchema["required"])
	}
	props, ok := tool.InputSchema["properties"].(map[string]any)
	if !ok {
		t.Fatalf("properties = %T", tool.InputSchema["properties"])
	}
	for _, key := range []string{"resourcePath", "action", "resourceType"} {
		prop, propOK := props[key].(map[string]any)
		if !propOK || prop["type"] != "string" {
			t.Errorf("property %q = %v, want string", key, props[key])
		}
	}
	properties, ok := props["properties"].(map[string]any)
	if !ok || properties["type"] != "object" {
		t.Errorf("properties property = %v", props["properties"])
	}
	if _, isMap := props["propertyFilter"].(map[string]any); isMap {
		t.Errorf("manage_resource must not advertise the read-only propertyFilter: %v", props["propertyFilter"])
	}
}

// TestManageResourceReadForwardsPropertyFilter proves a read's propertyFilter
// reaches the operations script.
func TestManageResourceReadForwardsPropertyFilter(t *testing.T) {
	t.Setenv(godot.PathEnv, fakeGodotOps(t, `echo "$@"`))
	root := t.TempDir()
	makeProject(t, root)

	result := callToolArgs(t, readResource(), root, `{"resourcePath":"resources/mat.tres","propertyFilter":["metallic","albedo_*"]}`)
	if result.IsError {
		t.Fatalf("unexpected error: %+v", result.Content)
	}
	text := result.Content[0].Text
	if !strings.Contains(text, `"propertyFilter":["metallic","albedo_*"]`) {
		t.Errorf("text = %q, want the property filter forwarded", text)
	}
}

// TestManageResourceFilterIgnoredForMutation proves propertyFilter is a read
// option: create and modify must not forward it.
func TestManageResourceFilterIgnoredForMutation(t *testing.T) {
	t.Setenv(godot.PathEnv, fakeGodotOps(t, `echo "$@"`))
	root := t.TempDir()
	makeProject(t, root)

	result := callToolArgs(t, manageResource(), root, `{"resourcePath":"resources/mat.tres","action":"modify","propertyFilter":["metallic"],"properties":{"metallic":0.5}}`)
	if result.IsError {
		t.Fatalf("unexpected error: %+v", result.Content)
	}
	if strings.Contains(result.Content[0].Text, `"propertyFilter"`) {
		t.Errorf("text = %q, want propertyFilter dropped for a modify", result.Content[0].Text)
	}
}

func TestManageResourceReadSuccess(t *testing.T) {
	t.Setenv(godot.PathEnv, fakeGodotOps(t, `echo "$@"`))
	root := t.TempDir()
	makeProject(t, root)

	result := callToolArgs(t, readResource(), root, `{"resourcePath":"resources/mat.tres"}`)
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
	for _, want := range []string{`"resourcePath":"resources/mat.tres"`, `"action":"read"`} {
		if !strings.Contains(text, want) {
			t.Errorf("text = %q, want it to contain %q", text, want)
		}
	}
	if strings.Contains(text, `"properties"`) {
		t.Errorf("text = %q, want no properties for a read action", text)
	}
}

func TestManageResourceReadStructured(t *testing.T) {
	t.Setenv(godot.PathEnv, fakeGodotOps(t, `printf '<<<GO_DOT_MCP_JSON_START>>>\n{"type":"StandardMaterial3D","path":"resources/mat.tres"}\n<<<GO_DOT_MCP_JSON_END>>>\n'`))
	root := t.TempDir()
	makeProject(t, root)

	result := callToolArgs(t, readResource(), root, `{"resourcePath":"resources/mat.tres"}`)
	if result.IsError {
		t.Fatalf("unexpected error: %+v", result.Content)
	}
	text := result.Content[0].Text
	for _, want := range []string{`"type": "StandardMaterial3D"`, `"path": "resources/mat.tres"`} {
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

func TestManageResourceModifySuccess(t *testing.T) {
	t.Setenv(godot.PathEnv, fakeGodotOps(t, `echo "$@"`))
	root := t.TempDir()
	makeProject(t, root)

	result := callToolArgs(t, manageResource(), root, `{"resourcePath":"resources/mat.tres","action":"modify","properties":{"metallic":0.9}}`)
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

func TestManageResourceCreateSuccess(t *testing.T) {
	t.Setenv(godot.PathEnv, fakeGodotOps(t, `echo "$@"`))
	root := t.TempDir()
	makeProject(t, root)

	result := callToolArgs(t, manageResource(), root, `{"resourcePath":"materials/red.tres","action":"create","resourceType":"StandardMaterial3D"}`)
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

func TestManageResourceCreateProperties(t *testing.T) {
	t.Setenv(godot.PathEnv, fakeGodotOps(t, `echo "$@"`))
	root := t.TempDir()
	makeProject(t, root)

	result := callToolArgs(t, manageResource(), root, `{"resourcePath":"materials/red.tres","action":"create","resourceType":"StandardMaterial3D","properties":{"albedo_color":{"r":1,"g":0,"b":0,"a":1}}}`)
	if result.IsError {
		t.Fatalf("unexpected error: %+v", result.Content)
	}
	text := result.Content[0].Text
	if !strings.Contains(text, `"properties":{"albedo_color":{"a":1,"b":0,"g":0,"r":1}}`) {
		t.Errorf("text = %q, want the properties to round-trip", text)
	}
}

func TestManageResourceValidation(t *testing.T) {
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
		{name: "missing params", tool: manageResource, args: `{}`, want: "resourcePath and action are required."},
		{name: "missing action", tool: manageResource, project: root, args: `{"resourcePath":"mat.tres"}`, want: "resourcePath and action are required."},
		{name: "missing resource path", tool: readResource, project: root, args: `{}`, want: "resourcePath is required."},
		{name: "create without resource type", tool: manageResource, project: root, args: `{"resourcePath":"mat.tres","action":"create"}`, want: "resourceType is required for the create action."},
		{name: "invalid project path", tool: readResource, project: "../escape", args: `{"resourcePath":"mat.tres"}`, want: "Invalid path."},
		{name: "invalid resource path", tool: readResource, project: root, args: `{"resourcePath":"../escape.tres"}`, want: "Invalid path."},
		{name: "not a project", tool: readResource, project: notAProject, args: `{"resourcePath":"mat.tres"}`, want: "Not a valid Godot project"},
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

func TestManageResourceScriptFailure(t *testing.T) {
	t.Setenv(godot.PathEnv, fakeGodotOps(t, `echo 'Failed to load resource: boom' >&2; exit 1`))
	root := t.TempDir()
	makeProject(t, root)

	result := callToolArgs(t, readResource(), root, `{"resourcePath":"mat.tres"}`)
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

func TestManageResourceSpawnFailure(t *testing.T) {
	t.Setenv(godot.PathEnv, filepath.Join(t.TempDir(), "missing-godot"))
	root := t.TempDir()
	makeProject(t, root)

	result := callToolArgs(t, readResource(), root, `{"resourcePath":"mat.tres"}`)
	if !result.IsError {
		t.Fatalf("expected an error result: %+v", result.Content)
	}
	if !strings.HasPrefix(result.Content[0].Text, "manage_resource failed: ") {
		t.Errorf("message = %q", result.Content[0].Text)
	}
}
