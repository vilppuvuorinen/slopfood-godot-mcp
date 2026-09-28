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

// fakeSession is a live engine session that records the calls the seam routes
// to it, so a test can prove a session-eligible handler used the session
// instead of spawning a separate engine.
type fakeSession struct {
	runs        []fakeSessionRun
	invalidated int
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

func (f *fakeSession) Invalidate() { f.invalidated++ }

// TestContextRunRequiresSession confirms the seam requires an installed engine
// session: with none, Run reports a clear error instead of spawning an engine.
func TestContextRunRequiresSession(t *testing.T) {
	ctx := Context{Context: context.Background()}
	_, _, _, err := ctx.Run("/proj", "create_scene", nil)
	if err == nil {
		t.Fatal("Run with no session: err = nil, want an error")
	}
	if got := err.Error(); !strings.Contains(got, "engine session required") {
		t.Errorf("err = %q, want the session-required message", got)
	}

	ctx.Invalidate()
}

// TestContextWithSessionDelegates confirms an installed session receives the
// call and its Invalidate, so the handler always reaches the session.
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

	ctx.Invalidate()
	if sess.invalidated != 1 {
		t.Errorf("invalidated = %d, want 1", sess.invalidated)
	}
}

// TestSessionEligibleHandlerUsesInstalledSession confirms a handler reaches the
// session through its context. GODOT_PATH points at a missing binary, so any
// attempt to spawn a separate engine would fail the call.
func TestSessionEligibleHandlerUsesInstalledSession(t *testing.T) {
	t.Setenv(godot.PathEnv, filepath.Join(t.TempDir(), "missing-godot"))
	root := t.TempDir()
	makeProject(t, root)

	sess := &fakeSession{}
	ctx := WithEngine(withProject(context.Background(), root), sess)
	out, err := createScene().Handler(ctx, json.RawMessage(`{"scenePath":"main.tscn"}`))
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
	if len(sess.runs) != 1 || sess.runs[0].operation != "create_scene" {
		t.Fatalf("session runs = %+v", sess.runs)
	}
}

// TestAttachScriptReconcilesWithoutInvalidation confirms attach_script routes
// through the session and does not invalidate it: the bridge reconciles the
// attached script with the editor filesystem, so a following class-name lookup
// sees it without a respawn.
func TestAttachScriptReconcilesWithoutInvalidation(t *testing.T) {
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
	if sess.invalidated != 0 {
		t.Errorf("invalidated = %d, want 0: the bridge reconciles in place", sess.invalidated)
	}
}

// TestReimportProjectInvalidatesSession confirms reimport_project invalidates a
// live session whether the scan succeeds or fails, since the scan mutates
// engine-global state either way.
func TestReimportProjectInvalidatesSession(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{name: "success", body: `exit 0`},
		{name: "failure", body: `echo boom >&2; exit 1`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv(godot.PathEnv, fakeGodotOps(t, tt.body))
			root := t.TempDir()
			makeProject(t, root)

			sess := &fakeSession{}
			ctx := WithEngine(withProject(context.Background(), root), sess)
			out, err := reimportProject().Handler(ctx, json.RawMessage(`{}`))
			if err != nil {
				t.Fatalf("handler: %v", err)
			}
			if _, ok := out.(mcp.ToolResult); !ok {
				t.Fatalf("result type %T", out)
			}
			if len(sess.runs) != 0 {
				t.Errorf("session runs = %+v, want none: reimport is one-shot", sess.runs)
			}
			if sess.invalidated != 1 {
				t.Errorf("invalidated = %d, want 1", sess.invalidated)
			}
		})
	}
}
