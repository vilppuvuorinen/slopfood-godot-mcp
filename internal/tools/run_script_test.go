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
	run    fakeSessionRun
	output string
}

func (s *runScriptSession) Run(_ context.Context, project, operation string, params map[string]any) (string, string, int, error) {
	s.run = fakeSessionRun{project: project, operation: operation, params: params}
	return s.output, "", 0, nil
}

func TestRunScriptDelegatesToSession(t *testing.T) {
	t.Setenv(godot.PathEnv, filepath.Join(t.TempDir(), "missing-godot"))
	root := t.TempDir()
	makeProject(t, root)

	payload := `{"compiled":true,"result":{"answer":42},"logs":["hi"],"errors":[]}`
	sess := &runScriptSession{output: jsonStartMarker + "\n" + payload + "\n" + jsonEndMarker}
	ctx := WithEngine(withProject(context.Background(), root), sess)
	source := "extends RefCounted\nfunc run():\n\treturn 42\n"
	raw, err := json.Marshal(map[string]any{"script": source})
	if err != nil {
		t.Fatalf("marshal args: %v", err)
	}
	out, err := runScript().Handler(ctx, raw)
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
	if got, _ := sess.run.params["script"].(string); got != source {
		t.Errorf("script param = %q, want the source verbatim", got)
	}
	structured, ok := result.StructuredContent.(map[string]any)
	if !ok {
		t.Fatalf("structuredContent = %T, want object", result.StructuredContent)
	}
	if structured["compiled"] != true {
		t.Errorf("compiled = %v, want true", structured["compiled"])
	}
}

func TestRunScriptRequiresScript(t *testing.T) {
	root := t.TempDir()
	makeProject(t, root)
	ctx := WithEngine(withProject(context.Background(), root), &fakeSession{})
	out, err := runScript().Handler(ctx, json.RawMessage(`{}`))
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
	ctx := WithEngine(withProject(context.Background(), root), &fakeSession{})
	out, err := runScript().Handler(ctx, json.RawMessage(`{"script":"extends RefCounted\n"}`))
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
