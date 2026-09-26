package tools

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/vilppuvuorinen/slopfood-godot-mcp/internal/godot"
)

func TestReimportProjectSchema(t *testing.T) {
	tool := reimportProject()
	if tool.Name != "reimport_project" {
		t.Errorf("name = %q", tool.Name)
	}
	if tool.Description != "Reimport a Godot project by scanning it in the editor, generating .import sidecars and missing resource UIDs" {
		t.Errorf("description = %q", tool.Description)
	}
	// The scan is project-wide, so the tool takes no arguments.
	required, ok := tool.InputSchema["required"].([]string)
	if !ok || len(required) != 0 {
		t.Errorf("required = %v", tool.InputSchema["required"])
	}
	props, ok := tool.InputSchema["properties"].(map[string]any)
	if !ok {
		t.Fatalf("properties = %T", tool.InputSchema["properties"])
	}
	if len(props) != 0 {
		t.Errorf("properties = %v, want none", props)
	}
}

func TestReimportProjectSuccess(t *testing.T) {
	fake := fakeGodotOps(t, `echo "$@"`)
	t.Setenv(godot.PathEnv, fake)
	root := t.TempDir()
	makeProject(t, root)

	result := callToolArgs(t, reimportProject(), root, `{}`)
	if result.IsError {
		t.Fatalf("unexpected error: %+v", result.Content)
	}
	if len(result.Content) != 1 {
		t.Fatalf("content = %+v", result.Content)
	}
	text := result.Content[0].Text
	if !strings.HasPrefix(text, "Project reimported successfully.") {
		t.Errorf("text = %q", text)
	}
	// The scan must be driven by --import: a runtime script (resave_resources)
	// cannot generate UIDs or apply imports, and resaving resources would strip
	// existing UIDs.
	if !strings.Contains(text, "--import") {
		t.Errorf("text = %q, want it to contain --import", text)
	}
	if strings.Contains(text, "--script") {
		t.Errorf("text = %q, want no --script invocation", text)
	}
	if !strings.Contains(text, root) {
		t.Errorf("text = %q, want it to contain the project root", text)
	}
}

func TestReimportProjectValidation(t *testing.T) {
	root := t.TempDir()
	makeProject(t, root)
	notAProject := t.TempDir()

	tests := []struct {
		name    string
		project string
		args    string
		want    string
	}{
		{name: "missing", args: `{}`, want: "Invalid path."},
		{name: "invalid path", project: "../escape", args: `{}`, want: "Invalid path."},
		{name: "not a project", project: notAProject, args: `{}`, want: "Not a valid Godot project"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := callToolArgs(t, reimportProject(), tt.project, tt.args)
			if !result.IsError {
				t.Fatalf("expected an error result: %+v", result.Content)
			}
			if !strings.HasPrefix(result.Content[0].Text, tt.want) {
				t.Errorf("message = %q, want prefix %q", result.Content[0].Text, tt.want)
			}
		})
	}
}

func TestReimportProjectScriptFailure(t *testing.T) {
	fake := fakeGodotOps(t, `echo 'Failed to reimport project: boom' >&2; exit 1`)
	t.Setenv(godot.PathEnv, fake)
	root := t.TempDir()
	makeProject(t, root)

	result := callToolArgs(t, reimportProject(), root, `{}`)
	if !result.IsError {
		t.Fatalf("expected an error result: %+v", result.Content)
	}
	if len(result.Content) != 2 {
		t.Fatalf("content = %+v", result.Content)
	}
	if !strings.HasPrefix(result.Content[0].Text, "Failed to reimport project: ") {
		t.Errorf("message = %q", result.Content[0].Text)
	}
	if !strings.HasPrefix(result.Content[1].Text, "Possible solutions:") {
		t.Errorf("solutions = %q", result.Content[1].Text)
	}
}

func TestReimportProjectSpawnFailure(t *testing.T) {
	t.Setenv(godot.PathEnv, filepath.Join(t.TempDir(), "missing-godot"))
	root := t.TempDir()
	makeProject(t, root)

	result := callToolArgs(t, reimportProject(), root, `{}`)
	if !result.IsError {
		t.Fatalf("expected an error result: %+v", result.Content)
	}
	if !strings.HasPrefix(result.Content[0].Text, "Failed to reimport project: ") {
		t.Errorf("message = %q", result.Content[0].Text)
	}
}
