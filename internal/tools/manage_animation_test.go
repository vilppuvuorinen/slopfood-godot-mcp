package tools

import (
	"strings"
	"testing"

	"github.com/vilppuvuorinen/go-dot-mcp/internal/godot"
	"github.com/vilppuvuorinen/go-dot-mcp/internal/mcp"
)

func TestManageAnimationSchema(t *testing.T) {
	tool := manageAnimation()
	if tool.Name != "manage_animation" {
		t.Errorf("name = %q", tool.Name)
	}
	if tool.Description != "Create or modify Animation resources and their tracks" {
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
	wantTypes := map[string]string{
		"resourcePath":  "string",
		"action":        "string",
		"length":        "number",
		"loopMode":      "string",
		"step":          "number",
		"trackType":     "string",
		"trackPath":     "string",
		"trackIndex":    "integer",
		"interpolation": "string",
		"keys":          "array",
	}
	for key, want := range wantTypes {
		prop, propOK := props[key].(map[string]any)
		if !propOK || prop["type"] != want {
			t.Errorf("property %q = %v, want %s", key, props[key], want)
		}
	}
	action, _ := props["action"].(map[string]any)
	enum, _ := action["enum"].([]string)
	if strings.Join(enum, ",") != "create,modify,add,set,remove" {
		t.Errorf("action enum = %v", action["enum"])
	}
}

func TestReadAnimationSchema(t *testing.T) {
	tool := readAnimation()
	if tool.Name != "read_animation" {
		t.Errorf("name = %q", tool.Name)
	}
	if tool.Description != "Read Animation resources and their tracks" {
		t.Errorf("description = %q", tool.Description)
	}
	if !hasRequired(tool.InputSchema, "resourcePath") {
		t.Errorf("required = %v", tool.InputSchema["required"])
	}
}

func TestManageAnimationCreateForwardsProperties(t *testing.T) {
	t.Setenv(godot.PathEnv, fakeGodotOps(t, `echo "$@"`))
	root := t.TempDir()
	makeProject(t, root)

	result := callToolArgs(t, manageAnimation(), root, `{"resourcePath":"anims/walk.tres","action":"create","length":2,"loopMode":"linear","step":0.05}`)
	if result.IsError {
		t.Fatalf("unexpected error: %+v", result.Content)
	}
	text := result.Content[0].Text
	for _, want := range []string{`"action":"create"`, `"length":2`, `"loopMode":"linear"`, `"step":0.05`} {
		if !strings.Contains(text, want) {
			t.Errorf("text = %q, want it to contain %q", text, want)
		}
	}
}

func TestManageAnimationAddForwardsTrack(t *testing.T) {
	t.Setenv(godot.PathEnv, fakeGodotOps(t, `echo "$@"`))
	root := t.TempDir()
	makeProject(t, root)

	result := callToolArgs(t, manageAnimation(), root, `{"resourcePath":"anims/walk.tres","action":"add","trackType":"value","trackPath":"Sprite2D:position","interpolation":"cubic","keys":[{"time":0,"value":{"$type":"Vector2","x":0,"y":0}}]}`)
	if result.IsError {
		t.Fatalf("unexpected error: %+v", result.Content)
	}
	text := result.Content[0].Text
	for _, want := range []string{`"trackType":"value"`, `"trackPath":"Sprite2D:position"`, `"interpolation":"cubic"`, `"keys":[`} {
		if !strings.Contains(text, want) {
			t.Errorf("text = %q, want it to contain %q", text, want)
		}
	}
}

func TestManageAnimationSetAndRemoveForwardIndex(t *testing.T) {
	t.Setenv(godot.PathEnv, fakeGodotOps(t, `echo "$@"`))
	root := t.TempDir()
	makeProject(t, root)

	set := callToolArgs(t, manageAnimation(), root, `{"resourcePath":"anims/walk.tres","action":"set","trackIndex":2,"trackPath":"root:x"}`)
	if set.IsError {
		t.Fatalf("unexpected set error: %+v", set.Content)
	}
	if !strings.Contains(set.Content[0].Text, `"trackIndex":2`) {
		t.Errorf("set text = %q, want trackIndex forwarded", set.Content[0].Text)
	}

	remove := callToolArgs(t, manageAnimation(), root, `{"resourcePath":"anims/walk.tres","action":"remove","trackIndex":1}`)
	if remove.IsError {
		t.Fatalf("unexpected remove error: %+v", remove.Content)
	}
	if !strings.Contains(remove.Content[0].Text, `"trackIndex":1`) {
		t.Errorf("remove text = %q, want trackIndex forwarded", remove.Content[0].Text)
	}
}

func TestReadAnimationStructured(t *testing.T) {
	t.Setenv(godot.PathEnv, fakeGodotOps(t, `printf '<<<GO_DOT_MCP_JSON_START>>>\n{"path":"anims/walk.tres","length":2,"loopMode":"linear","step":0.1,"tracks":[]}\n<<<GO_DOT_MCP_JSON_END>>>\n'`))
	root := t.TempDir()
	makeProject(t, root)

	result := callToolArgs(t, readAnimation(), root, `{"resourcePath":"anims/walk.tres"}`)
	if result.IsError {
		t.Fatalf("unexpected error: %+v", result.Content)
	}
	text := result.Content[0].Text
	for _, want := range []string{`"path": "anims/walk.tres"`, `"loopMode": "linear"`, `"tracks": []`} {
		if !strings.Contains(text, want) {
			t.Errorf("text = %q, want it to contain %q", text, want)
		}
	}
	if strings.Contains(text, "succeeded.\n\nOutput:") {
		t.Errorf("text = %q, want the structured payload without the legacy wrapper", text)
	}
}

func TestReadAnimationReadSuccessFallback(t *testing.T) {
	t.Setenv(godot.PathEnv, fakeGodotOps(t, `echo "$@"`))
	root := t.TempDir()
	makeProject(t, root)

	result := callToolArgs(t, readAnimation(), root, `{"resourcePath":"anims/walk.tres"}`)
	if result.IsError {
		t.Fatalf("unexpected error: %+v", result.Content)
	}
	text := result.Content[0].Text
	for _, want := range []string{`"action":"read"`, `"resourcePath":"anims/walk.tres"`} {
		if !strings.Contains(text, want) {
			t.Errorf("text = %q, want it to contain %q", text, want)
		}
	}
}

func TestManageAnimationValidation(t *testing.T) {
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
		{name: "missing params", tool: manageAnimation, args: `{}`, want: "resourcePath and action are required."},
		{name: "missing action", tool: manageAnimation, project: root, args: `{"resourcePath":"anim.tres"}`, want: "resourcePath and action are required."},
		{name: "missing resource path on read", tool: readAnimation, project: root, args: `{}`, want: "resourcePath is required."},
		{name: "modify without properties", tool: manageAnimation, project: root, args: `{"resourcePath":"anim.tres","action":"modify"}`, want: "The modify action requires at least one of: length, loopMode, step."},
		{name: "add without track type", tool: manageAnimation, project: root, args: `{"resourcePath":"anim.tres","action":"add"}`, want: "trackType is required for the add action."},
		{name: "set without track index", tool: manageAnimation, project: root, args: `{"resourcePath":"anim.tres","action":"set"}`, want: "trackIndex is required for the set action."},
		{name: "remove without track index", tool: manageAnimation, project: root, args: `{"resourcePath":"anim.tres","action":"remove"}`, want: "trackIndex is required for the remove action."},
		{name: "unknown action", tool: manageAnimation, project: root, args: `{"resourcePath":"anim.tres","action":"bogus"}`, want: "Unknown action: bogus. Use create, modify, add, set, or remove."},
		{name: "invalid project path", tool: readAnimation, project: "../escape", args: `{"resourcePath":"anim.tres"}`, want: "Invalid path."},
		{name: "invalid resource path", tool: readAnimation, project: root, args: `{"resourcePath":"../escape.tres"}`, want: "Invalid path."},
		{name: "not a project", tool: readAnimation, project: notAProject, args: `{"resourcePath":"anim.tres"}`, want: "Not a valid Godot project"},
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

func TestManageAnimationScriptFailure(t *testing.T) {
	t.Setenv(godot.PathEnv, fakeGodotOps(t, `echo 'Animation resource not found: boom' >&2; exit 1`))
	root := t.TempDir()
	makeProject(t, root)

	result := callToolArgs(t, readAnimation(), root, `{"resourcePath":"anim.tres"}`)
	if !result.IsError {
		t.Fatalf("expected an error result: %+v", result.Content)
	}
	if !strings.HasPrefix(result.Content[0].Text, "manage_animation failed: ") {
		t.Errorf("message = %q", result.Content[0].Text)
	}
	if len(result.Content) != 2 || !strings.HasPrefix(result.Content[1].Text, "Possible solutions:") {
		t.Errorf("solutions = %+v", result.Content)
	}
}
