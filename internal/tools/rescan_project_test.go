package tools

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/vilppuvuorinen/slopfood-godot-mcp/internal/mcp"
)

func TestRescanProjectSchema(t *testing.T) {
	tool := rescanProject()
	if tool.Name != "rescan_project" {
		t.Errorf("name = %q", tool.Name)
	}
	if tool.Description != "Rescan a Godot project in the editor, generating .import sidecars and missing resource UIDs" {
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

type recordingSession struct {
	runs   []fakeSessionRun
	stdout string
	stderr string
	exit   int
	err    error
}

func (s *recordingSession) Run(_ context.Context, project, operation string, params map[string]any) (string, string, int, error) {
	s.runs = append(s.runs, fakeSessionRun{project: project, operation: operation, params: params})
	return s.stdout, s.stderr, s.exit, s.err
}

func TestRescanProjectSuccess(t *testing.T) {
	root := t.TempDir()
	makeProject(t, root)

	sess := &recordingSession{stdout: "Scanned project sources\n"}
	ctx := WithSession(withProject(context.Background(), root), sess)
	out, err := rescanProject().Handler(ctx, json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	result, ok := out.(mcp.ToolResult)
	if !ok {
		t.Fatalf("result type %T", out)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %+v", result.Content)
	}
	if len(result.Content) != 1 {
		t.Fatalf("content = %+v", result.Content)
	}
	text := result.Content[0].Text
	if !strings.HasPrefix(text, "Project rescanned successfully.") {
		t.Errorf("text = %q", text)
	}
	if !strings.Contains(text, "Scanned project sources") {
		t.Errorf("text = %q, want the session's output", text)
	}
	if len(sess.runs) != 1 {
		t.Fatalf("session runs = %+v, want one", sess.runs)
	}
	if sess.runs[0].operation != "rescan_project" {
		t.Errorf("session operation = %q, want rescan_project", sess.runs[0].operation)
	}
	if sess.runs[0].project != root {
		t.Errorf("session project = %q, want the bound root %q", sess.runs[0].project, root)
	}
}

func TestRescanProjectValidation(t *testing.T) {
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
			result := callToolArgs(t, rescanProject(), tt.project, tt.args)
			if !result.IsError {
				t.Fatalf("expected an error result: %+v", result.Content)
			}
			if !strings.HasPrefix(result.Content[0].Text, tt.want) {
				t.Errorf("message = %q, want prefix %q", result.Content[0].Text, tt.want)
			}
		})
	}
}

func TestRescanProjectScriptFailure(t *testing.T) {
	root := t.TempDir()
	makeProject(t, root)

	sess := &recordingSession{stderr: "Failed to rescan project: boom", exit: 1}
	ctx := WithSession(withProject(context.Background(), root), sess)
	out, err := rescanProject().Handler(ctx, json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	result, ok := out.(mcp.ToolResult)
	if !ok {
		t.Fatalf("result type %T", out)
	}
	if !result.IsError {
		t.Fatalf("expected an error result: %+v", result.Content)
	}
	if len(result.Content) != 2 {
		t.Fatalf("content = %+v", result.Content)
	}
	if !strings.HasPrefix(result.Content[0].Text, "Failed to rescan project: ") {
		t.Errorf("message = %q", result.Content[0].Text)
	}
	if !strings.HasPrefix(result.Content[1].Text, "Possible solutions:") {
		t.Errorf("solutions = %q", result.Content[1].Text)
	}
}

func TestRescanProjectSessionFailure(t *testing.T) {
	root := t.TempDir()
	makeProject(t, root)

	sess := &recordingSession{err: context.DeadlineExceeded}
	ctx := WithSession(withProject(context.Background(), root), sess)
	out, err := rescanProject().Handler(ctx, json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	result, ok := out.(mcp.ToolResult)
	if !ok {
		t.Fatalf("result type %T", out)
	}
	if !result.IsError {
		t.Fatalf("expected an error result: %+v", result.Content)
	}
	if !strings.HasPrefix(result.Content[0].Text, "Failed to rescan project: ") {
		t.Errorf("message = %q", result.Content[0].Text)
	}
}
