package tools

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/vilppuvuorinen/go-dot-mcp/internal/godot"
)

func TestUpdateProjectUIDsSchema(t *testing.T) {
	tool := updateProjectUIDs()
	if tool.Name != "update_project_uids" {
		t.Errorf("name = %q", tool.Name)
	}
	if tool.Description != "Generate missing resource UIDs in a Godot project by scanning it in the editor (for Godot 4.4+)" {
		t.Errorf("description = %q", tool.Description)
	}
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

func TestUpdateProjectUIDsSuccess(t *testing.T) {
	fake := fakeGodotOps(t, fakeUIDVersion+"\necho \"$@\"")
	t.Setenv(godot.PathEnv, fake)
	root := t.TempDir()
	makeProject(t, root)

	result := callToolArgs(t, updateProjectUIDs(), root, `{}`)
	if result.IsError {
		t.Fatalf("unexpected error: %+v", result.Content)
	}
	if len(result.Content) != 1 {
		t.Fatalf("content = %+v", result.Content)
	}
	text := result.Content[0].Text
	if !strings.HasPrefix(text, "Project UIDs updated successfully.") {
		t.Errorf("text = %q", text)
	}
	// The scan must be driven by --import: a runtime script (resave_resources)
	// cannot generate UIDs, and resaving resources would strip existing ones.
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

func TestUpdateProjectUIDsVersionGate(t *testing.T) {
	fake := fakeGodotOps(t, `if [ "$1" = "--version" ]; then echo "4.3.1.stable"; exit 0; fi`+"\nexit 1")
	t.Setenv(godot.PathEnv, fake)
	root := t.TempDir()
	makeProject(t, root)

	result := callToolArgs(t, updateProjectUIDs(), root, `{}`)
	if !result.IsError {
		t.Fatalf("expected an error result: %+v", result.Content)
	}
	if len(result.Content) != 2 {
		t.Fatalf("content = %+v", result.Content)
	}
	if got := result.Content[0].Text; got != "UIDs are only supported in Godot 4.4 or later. Current version: 4.3.1.stable" {
		t.Errorf("message = %q", got)
	}
}

func TestUpdateProjectUIDsValidation(t *testing.T) {
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
			result := callToolArgs(t, updateProjectUIDs(), tt.project, tt.args)
			if !result.IsError {
				t.Fatalf("expected an error result: %+v", result.Content)
			}
			if !strings.HasPrefix(result.Content[0].Text, tt.want) {
				t.Errorf("message = %q, want prefix %q", result.Content[0].Text, tt.want)
			}
		})
	}
}

func TestUpdateProjectUIDsScriptFailure(t *testing.T) {
	fake := fakeGodotOps(t, fakeUIDVersion+"\necho 'Failed to update project UIDs: boom' >&2; exit 1")
	t.Setenv(godot.PathEnv, fake)
	root := t.TempDir()
	makeProject(t, root)

	result := callToolArgs(t, updateProjectUIDs(), root, `{}`)
	if !result.IsError {
		t.Fatalf("expected an error result: %+v", result.Content)
	}
	if len(result.Content) != 2 {
		t.Fatalf("content = %+v", result.Content)
	}
	if !strings.HasPrefix(result.Content[0].Text, "Failed to update project UIDs: ") {
		t.Errorf("message = %q", result.Content[0].Text)
	}
	if !strings.HasPrefix(result.Content[1].Text, "Possible solutions:") {
		t.Errorf("solutions = %q", result.Content[1].Text)
	}
}

func TestUpdateProjectUIDsSpawnFailure(t *testing.T) {
	t.Setenv(godot.PathEnv, filepath.Join(t.TempDir(), "missing-godot"))
	root := t.TempDir()
	makeProject(t, root)

	result := callToolArgs(t, updateProjectUIDs(), root, `{}`)
	if !result.IsError {
		t.Fatalf("expected an error result: %+v", result.Content)
	}
	if !strings.HasPrefix(result.Content[0].Text, "Failed to update project UIDs: ") {
		t.Errorf("message = %q", result.Content[0].Text)
	}
}

func TestUpdateProjectUIDsVersionFailure(t *testing.T) {
	fake := fakeGodotOps(t, `echo "boom" >&2; exit 1`)
	t.Setenv(godot.PathEnv, fake)
	root := t.TempDir()
	makeProject(t, root)

	result := callToolArgs(t, updateProjectUIDs(), root, `{}`)
	if !result.IsError {
		t.Fatalf("expected an error result: %+v", result.Content)
	}
	if !strings.HasPrefix(result.Content[0].Text, "Failed to update project UIDs: ") {
		t.Errorf("message = %q", result.Content[0].Text)
	}
}
