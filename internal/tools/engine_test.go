package tools

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vilppuvuorinen/slopfood-godot-mcp/internal/godot"
	"github.com/vilppuvuorinen/slopfood-godot-mcp/internal/mcp"
)

type fakeSession struct {
	runs []fakeSessionRun
}

type fakeSessionRun struct {
	project   string
	operation string
	params    map[string]any
}

func (f *fakeSession) Run(_ context.Context, project, operation string, params map[string]any) (string, string, int, error) {
	f.runs = append(f.runs, fakeSessionRun{project: project, operation: operation, params: params})
	return "", "", 0, nil
}

func TestContextRunRequiresSession(t *testing.T) {
	ctx := Context{Context: context.Background()}
	_, _, _, err := ctx.Run("/proj", "attach_script", nil)
	if err == nil {
		t.Fatal("Run with no session: err = nil, want an error")
	}
	if got := err.Error(); !strings.Contains(got, "engine session required") {
		t.Errorf("err = %q, want the session-required message", got)
	}
}

func TestContextWithSessionDelegates(t *testing.T) {
	sess := &fakeSession{}
	ctx := Context{Context: WithEngine(context.Background(), sess)}

	if _, _, _, err := ctx.Run("/proj", "read_scene", map[string]any{"scenePath": "main.tscn"}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(sess.runs) != 1 {
		t.Fatalf("session runs = %d, want 1", len(sess.runs))
	}
	if got := sess.runs[0]; got.project != "/proj" || got.operation != "read_scene" {
		t.Errorf("session call = %+v", got)
	}
}

func TestAttachScriptReconcilesInPlace(t *testing.T) {
	t.Setenv(godot.PathEnv, filepath.Join(t.TempDir(), "missing-godot"))
	root := t.TempDir()
	makeProject(t, root)
	seedFile(t, filepath.Join(root, "scenes", "main.tscn"), "[gd_scene format=3]\n")
	seedFile(t, filepath.Join(root, "player.gd"), "extends Node\n")

	sess := &fakeSession{}
	ctx := WithEngine(withProject(context.Background(), root), sess)
	out, err := attachScript().Handler(ctx, json.RawMessage(
		`{"scenePath":"scenes/main.tscn","nodePath":"root","scriptPath":"player.gd"}`))
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
	if len(sess.runs) != 1 || sess.runs[0].operation != "attach_script" {
		t.Fatalf("session runs = %+v", sess.runs)
	}
}

func TestRescanProjectReconcilesSession(t *testing.T) {
	root := t.TempDir()
	makeProject(t, root)

	sess := &fakeSession{}
	ctx := WithEngine(withProject(context.Background(), root), sess)
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
	if len(sess.runs) != 1 || sess.runs[0].operation != "rescan_project" {
		t.Fatalf("session runs = %+v, want one rescan_project", sess.runs)
	}
	if sess.runs[0].project != root {
		t.Errorf("session project = %q, want %q", sess.runs[0].project, root)
	}
}
