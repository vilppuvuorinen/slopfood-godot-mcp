package tools

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/vilppuvuorinen/slopfood-godot-mcp/internal/mcp"
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

// recordingSession is a Session double that records the operation each Run
// carries and echoes a canned stdout. It proves a handler reached the warm
// session instead of forking a separate engine.
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

func TestReimportProjectSuccess(t *testing.T) {
	root := t.TempDir()
	makeProject(t, root)

	sess := &recordingSession{stdout: "Scanned project sources\n"}
	ctx := WithEngine(withProject(context.Background(), root), sess)
	out, err := reimportProject().Handler(ctx, json.RawMessage(`{}`))
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
	if !strings.HasPrefix(text, "Project reimported successfully.") {
		t.Errorf("text = %q", text)
	}
	if !strings.Contains(text, "Scanned project sources") {
		t.Errorf("text = %q, want the session's output", text)
	}
	// The scan must reach the warm session's reimport_project operation, which
	// reconciles through the editor filesystem.
	if len(sess.runs) != 1 {
		t.Fatalf("session runs = %+v, want one", sess.runs)
	}
	if sess.runs[0].operation != "reimport_project" {
		t.Errorf("session operation = %q, want reimport_project", sess.runs[0].operation)
	}
	if sess.runs[0].project != root {
		t.Errorf("session project = %q, want the bound root %q", sess.runs[0].project, root)
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
	root := t.TempDir()
	makeProject(t, root)

	sess := &recordingSession{stderr: "Failed to reimport project: boom", exit: 1}
	ctx := WithEngine(withProject(context.Background(), root), sess)
	out, err := reimportProject().Handler(ctx, json.RawMessage(`{}`))
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
	if !strings.HasPrefix(result.Content[0].Text, "Failed to reimport project: ") {
		t.Errorf("message = %q", result.Content[0].Text)
	}
	if !strings.HasPrefix(result.Content[1].Text, "Possible solutions:") {
		t.Errorf("solutions = %q", result.Content[1].Text)
	}
}

// TestReimportProjectSessionFailure covers the session itself failing (an engine
// that never started, a dead socket): the tool reports it with the spawn-style
// solutions, mirroring the pre-warm --import spawn failure.
func TestReimportProjectSessionFailure(t *testing.T) {
	root := t.TempDir()
	makeProject(t, root)

	sess := &recordingSession{err: context.DeadlineExceeded}
	ctx := WithEngine(withProject(context.Background(), root), sess)
	out, err := reimportProject().Handler(ctx, json.RawMessage(`{}`))
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
	if !strings.HasPrefix(result.Content[0].Text, "Failed to reimport project: ") {
		t.Errorf("message = %q", result.Content[0].Text)
	}
}
