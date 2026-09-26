package tools

import (
	"strings"
	"testing"

	"github.com/vilppuvuorinen/slopfood-godot-mcp/internal/godot"
	"github.com/vilppuvuorinen/slopfood-godot-mcp/internal/mcp"
)

func TestCreateAnimationSchema(t *testing.T) {
	tool := createAnimation()
	if tool.Name != "create_animation" {
		t.Errorf("name = %q", tool.Name)
	}
	if tool.Description != "Create an Animation resource" {
		t.Errorf("description = %q", tool.Description)
	}
	if !hasRequired(tool.InputSchema, "resourcePath") {
		t.Errorf("required = %v", tool.InputSchema["required"])
	}
	props, ok := tool.InputSchema["properties"].(map[string]any)
	if !ok {
		t.Fatalf("properties = %T", tool.InputSchema["properties"])
	}
	wantTypes := map[string]string{
		"resourcePath": "string",
		"length":       "number",
		"loopMode":     "string",
		"step":         "number",
	}
	for key, want := range wantTypes {
		prop, propOK := props[key].(map[string]any)
		if !propOK || prop["type"] != want {
			t.Errorf("property %q = %v, want %s", key, props[key], want)
		}
	}
	loopMode, _ := props["loopMode"].(map[string]any)
	enum, _ := loopMode["enum"].([]string)
	if strings.Join(enum, ",") != "none,linear,pingpong" {
		t.Errorf("loopMode enum = %v", loopMode["enum"])
	}
}

func TestModifyAnimationSchema(t *testing.T) {
	tool := modifyAnimation()
	if tool.Name != "modify_animation" {
		t.Errorf("name = %q", tool.Name)
	}
	if tool.Description != "Set an Animation resource's length, loop mode, or step" {
		t.Errorf("description = %q", tool.Description)
	}
	if !hasRequired(tool.InputSchema, "resourcePath") {
		t.Errorf("required = %v", tool.InputSchema["required"])
	}
	props, ok := tool.InputSchema["properties"].(map[string]any)
	if !ok {
		t.Fatalf("properties = %T", tool.InputSchema["properties"])
	}
	for _, key := range []string{"resourcePath", "length", "loopMode", "step"} {
		if _, propOK := props[key].(map[string]any); !propOK {
			t.Errorf("missing property %q", key)
		}
	}
}

func TestAddAnimationTrackSchema(t *testing.T) {
	tool := addAnimationTrack()
	if tool.Name != "add_animation_track" {
		t.Errorf("name = %q", tool.Name)
	}
	if tool.Description != "Append a track to an Animation resource" {
		t.Errorf("description = %q", tool.Description)
	}
	if !hasRequired(tool.InputSchema, "resourcePath", "trackType") {
		t.Errorf("required = %v", tool.InputSchema["required"])
	}
	props, ok := tool.InputSchema["properties"].(map[string]any)
	if !ok {
		t.Fatalf("properties = %T", tool.InputSchema["properties"])
	}
	wantTypes := map[string]string{
		"resourcePath":  "string",
		"trackType":     "string",
		"trackPath":     "string",
		"interpolation": "string",
		"keys":          "array",
	}
	for key, want := range wantTypes {
		prop, propOK := props[key].(map[string]any)
		if !propOK || prop["type"] != want {
			t.Errorf("property %q = %v, want %s", key, props[key], want)
		}
	}
	trackType, _ := props["trackType"].(map[string]any)
	enum, _ := trackType["enum"].([]string)
	if strings.Join(enum, ",") != "value,position_3d,rotation_3d,scale_3d,blend_shape,method,bezier,audio,animation" {
		t.Errorf("trackType enum = %v", trackType["enum"])
	}
	interpolation, _ := props["interpolation"].(map[string]any)
	enum, _ = interpolation["enum"].([]string)
	if strings.Join(enum, ",") != "nearest,linear,cubic" {
		t.Errorf("interpolation enum = %v", interpolation["enum"])
	}
}

func TestSetAnimationTrackSchema(t *testing.T) {
	tool := setAnimationTrack()
	if tool.Name != "set_animation_track" {
		t.Errorf("name = %q", tool.Name)
	}
	if tool.Description != "Set an existing Animation track's path, interpolation, and keys" {
		t.Errorf("description = %q", tool.Description)
	}
	if !hasRequired(tool.InputSchema, "resourcePath", "trackIndex") {
		t.Errorf("required = %v", tool.InputSchema["required"])
	}
	props, ok := tool.InputSchema["properties"].(map[string]any)
	if !ok {
		t.Fatalf("properties = %T", tool.InputSchema["properties"])
	}
	wantTypes := map[string]string{
		"resourcePath":  "string",
		"trackIndex":    "integer",
		"trackPath":     "string",
		"interpolation": "string",
		"keys":          "array",
	}
	for key, want := range wantTypes {
		prop, propOK := props[key].(map[string]any)
		if !propOK || prop["type"] != want {
			t.Errorf("property %q = %v, want %s", key, props[key], want)
		}
	}
}

func TestRemoveAnimationTrackSchema(t *testing.T) {
	tool := removeAnimationTrack()
	if tool.Name != "remove_animation_track" {
		t.Errorf("name = %q", tool.Name)
	}
	if tool.Description != "Remove a track from an Animation resource" {
		t.Errorf("description = %q", tool.Description)
	}
	if !hasRequired(tool.InputSchema, "resourcePath", "trackIndex") {
		t.Errorf("required = %v", tool.InputSchema["required"])
	}
	props, ok := tool.InputSchema["properties"].(map[string]any)
	if !ok {
		t.Fatalf("properties = %T", tool.InputSchema["properties"])
	}
	prop, propOK := props["trackIndex"].(map[string]any)
	if !propOK || prop["type"] != "integer" {
		t.Errorf("trackIndex = %v, want integer", props["trackIndex"])
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

func TestCreateAnimationForwardsProperties(t *testing.T) {
	t.Setenv(godot.PathEnv, fakeGodotOps(t, `echo "$@"`))
	root := t.TempDir()
	makeProject(t, root)

	result := callToolArgs(t, createAnimation(), root, `{"resourcePath":"anims/walk.tres","length":2,"loopMode":"linear","step":0.05}`)
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

func TestModifyAnimationForwardsProperties(t *testing.T) {
	t.Setenv(godot.PathEnv, fakeGodotOps(t, `echo "$@"`))
	root := t.TempDir()
	makeProject(t, root)

	result := callToolArgs(t, modifyAnimation(), root, `{"resourcePath":"anims/walk.tres","length":3,"loopMode":"pingpong"}`)
	if result.IsError {
		t.Fatalf("unexpected error: %+v", result.Content)
	}
	text := result.Content[0].Text
	for _, want := range []string{`"action":"modify"`, `"length":3`, `"loopMode":"pingpong"`} {
		if !strings.Contains(text, want) {
			t.Errorf("text = %q, want it to contain %q", text, want)
		}
	}
}

func TestAddAnimationTrackForwardsTrack(t *testing.T) {
	t.Setenv(godot.PathEnv, fakeGodotOps(t, `echo "$@"`))
	root := t.TempDir()
	makeProject(t, root)

	result := callToolArgs(t, addAnimationTrack(), root, `{"resourcePath":"anims/walk.tres","trackType":"value","trackPath":"Sprite2D:position","interpolation":"cubic","keys":[{"time":0,"value":{"$type":"Vector2","x":0,"y":0}}]}`)
	if result.IsError {
		t.Fatalf("unexpected error: %+v", result.Content)
	}
	text := result.Content[0].Text
	for _, want := range []string{`"action":"add"`, `"trackType":"value"`, `"trackPath":"Sprite2D:position"`, `"interpolation":"cubic"`, `"keys":[`} {
		if !strings.Contains(text, want) {
			t.Errorf("text = %q, want it to contain %q", text, want)
		}
	}
}

func TestSetAndRemoveAnimationTrackForwardIndex(t *testing.T) {
	t.Setenv(godot.PathEnv, fakeGodotOps(t, `echo "$@"`))
	root := t.TempDir()
	makeProject(t, root)

	set := callToolArgs(t, setAnimationTrack(), root, `{"resourcePath":"anims/walk.tres","trackIndex":2,"trackPath":"root:x"}`)
	if set.IsError {
		t.Fatalf("unexpected set error: %+v", set.Content)
	}
	for _, want := range []string{`"action":"set"`, `"trackIndex":2`} {
		if !strings.Contains(set.Content[0].Text, want) {
			t.Errorf("set text = %q, want it to contain %q", set.Content[0].Text, want)
		}
	}

	remove := callToolArgs(t, removeAnimationTrack(), root, `{"resourcePath":"anims/walk.tres","trackIndex":1}`)
	if remove.IsError {
		t.Fatalf("unexpected remove error: %+v", remove.Content)
	}
	for _, want := range []string{`"action":"remove"`, `"trackIndex":1`} {
		if !strings.Contains(remove.Content[0].Text, want) {
			t.Errorf("remove text = %q, want it to contain %q", remove.Content[0].Text, want)
		}
	}
}

func TestReadAnimationStructured(t *testing.T) {
	t.Setenv(godot.PathEnv, fakeGodotOps(t, `printf '<<<GODOT_MCP_JSON_START>>>\n{"path":"anims/walk.tres","length":2,"loopMode":"linear","step":0.1,"tracks":[]}\n<<<GODOT_MCP_JSON_END>>>\n'`))
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

func TestAnimationValidation(t *testing.T) {
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
		{name: "create missing resource path", tool: createAnimation, args: `{}`, want: "resourcePath is required."},
		{name: "modify missing resource path", tool: modifyAnimation, args: `{}`, want: "resourcePath is required."},
		{name: "modify without properties", tool: modifyAnimation, project: root, args: `{"resourcePath":"anim.tres"}`, want: "The modify_animation tool requires at least one of: length, loopMode, step."},
		{name: "add missing track type", tool: addAnimationTrack, project: root, args: `{"resourcePath":"anim.tres"}`, want: "resourcePath and trackType are required."},
		{name: "set missing track index", tool: setAnimationTrack, project: root, args: `{"resourcePath":"anim.tres"}`, want: "resourcePath and trackIndex are required."},
		{name: "remove missing track index", tool: removeAnimationTrack, project: root, args: `{"resourcePath":"anim.tres"}`, want: "resourcePath and trackIndex are required."},
		{name: "read missing resource path", tool: readAnimation, project: root, args: `{}`, want: "resourcePath is required."},
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

func TestAnimationScriptFailure(t *testing.T) {
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
