package tools

import (
	"strings"
	"testing"

	"github.com/vilppuvuorinen/slopfood-godot-mcp/internal/godot"
	"github.com/vilppuvuorinen/slopfood-godot-mcp/internal/mcp"
)

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
	for _, want := range []string{`read_animation`, `"resourcePath":"anims/walk.tres"`} {
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
		{name: "read missing resource path", tool: readAnimation, project: root, args: `{}`, want: "resourcePath is required."},
		{name: "invalid project path", tool: readAnimation, project: "../escape", args: `{"resourcePath":"anim.tres"}`, want: "Invalid path."},
		{name: "invalid resource path", tool: readAnimation, project: root, args: `{"resourcePath":"../escape.tres"}`, want: "Invalid path."},
		{name: "absolute resource path", tool: readAnimation, project: root, args: `{"resourcePath":"/etc/passwd.tres"}`, want: "Invalid path."},
		{name: "normalized parent resource path", tool: readAnimation, project: root, args: `{"resourcePath":"foo/../bar.tres"}`, want: "Invalid path."},
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
	if !strings.HasPrefix(result.Content[0].Text, "read_animation failed: ") {
		t.Errorf("message = %q", result.Content[0].Text)
	}
	if len(result.Content) != 2 || !strings.HasPrefix(result.Content[1].Text, "Possible solutions:") {
		t.Errorf("solutions = %+v", result.Content)
	}
}
