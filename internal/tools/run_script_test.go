package tools

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/vilppuvuorinen/slopfood-godot-mcp/internal/godot"
	"github.com/vilppuvuorinen/slopfood-godot-mcp/internal/mcp"
)

type runScriptSession struct {
	fakeSession
	run     fakeSessionRun
	payload string
}

func (s *runScriptSession) Run(_ context.Context, project, operation string, params map[string]any) (godot.RunResult, error) {
	s.run = fakeSessionRun{project: project, operation: operation, params: params}
	return godot.RunResult{Payload: s.payload}, nil
}

func TestRunScriptDelegatesToSession(t *testing.T) {
	t.Setenv(godot.PathEnv, filepath.Join(t.TempDir(), "missing-godot"))
	root := t.TempDir()
	makeProject(t, root)

	payload := `{"compiled":true,"result":{"answer":42},"logs":["hi"],"errors":[]}`
	sess := &runScriptSession{payload: payload}
	body := "return 42\n"
	raw, err := json.Marshal(map[string]any{"body": body})
	if err != nil {
		t.Fatalf("marshal args: %v", err)
	}
	out, err := newToolset(root, sess).runScript().Handler(context.Background(), raw)
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
	if sess.run.operation != "run_script" {
		t.Errorf("operation = %q, want run_script", sess.run.operation)
	}
	if got, _ := sess.run.params["body"].(string); got != body {
		t.Errorf("body param = %q, want the body verbatim", got)
	}
	structured, ok := result.StructuredContent.(map[string]any)
	if !ok {
		t.Fatalf("structuredContent = %T, want object", result.StructuredContent)
	}
	if structured["compiled"] != true {
		t.Errorf("compiled = %v, want true", structured["compiled"])
	}
}

func TestRunScriptRequiresBody(t *testing.T) {
	root := t.TempDir()
	makeProject(t, root)
	out, err := newToolset(root, &fakeSession{}).runScript().Handler(context.Background(), json.RawMessage(`{}`))
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
}

func TestRunScriptNotAProject(t *testing.T) {
	root := t.TempDir()
	out, err := newToolset(root, &fakeSession{}).runScript().Handler(context.Background(), json.RawMessage(`{"body":"return 0\n"}`))
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
}
