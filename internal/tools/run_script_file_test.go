package tools

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/vilppuvuorinen/slopfood-godot-mcp/internal/godot"
	"github.com/vilppuvuorinen/slopfood-godot-mcp/internal/mcp"
)

func TestRunScriptFileSchema(t *testing.T) {
	tool := newToolset("", nil).runScriptFile()
	if tool.Name != "run_script_file" {
		t.Errorf("name = %q", tool.Name)
	}
	required, ok := tool.InputSchema["required"].([]string)
	if !ok || strings.Join(required, ",") != "script" {
		t.Errorf("required = %v", tool.InputSchema["required"])
	}
	props, ok := tool.InputSchema["properties"].(map[string]any)
	if !ok {
		t.Fatalf("properties = %T", tool.InputSchema["properties"])
	}
	if prop, ok := props["script"].(map[string]any); !ok || prop["type"] != "string" {
		t.Errorf("script property = %v", props["script"])
	}
	if prop, ok := props["args"].(map[string]any); !ok || prop["type"] != "array" {
		t.Errorf("args property = %v", props["args"])
	}
}

func TestRunScriptFileRunsScript(t *testing.T) {
	t.Setenv(godot.PathEnv, fakeGodotOps(t, `echo "$@"`))
	root := t.TempDir()
	makeProject(t, root)
	seedFile(t, filepath.Join(root, "tests", "check.gd"), "extends SceneTree\n")

	result := callToolArgs(t, (*toolset).runScriptFile, root, `{"script":"tests/check.gd"}`)
	if result.IsError {
		t.Fatalf("unexpected error: %+v", result.Content)
	}
	text := result.Content[0].Text
	for _, want := range []string{"--headless", "--script", "res://tests/check.gd", "exit code 0"} {
		if !strings.Contains(text, want) {
			t.Errorf("text = %q, want it to contain %q", text, want)
		}
	}
	structured, ok := result.StructuredContent.(map[string]any)
	if !ok {
		t.Fatalf("structuredContent = %T, want object", result.StructuredContent)
	}
	if structured["script"] != "res://tests/check.gd" {
		t.Errorf("script = %v, want res://tests/check.gd", structured["script"])
	}
	if structured["exitCode"] != 0 {
		t.Errorf("exitCode = %v, want 0", structured["exitCode"])
	}
	if structured["timedOut"] != false {
		t.Errorf("timedOut = %v, want false", structured["timedOut"])
	}
	if stdout, _ := structured["stdout"].(string); !strings.Contains(stdout, "res://tests/check.gd") {
		t.Errorf("stdout = %q, want the script path", stdout)
	}
}

func TestRunScriptFileForwardsArgs(t *testing.T) {
	t.Setenv(godot.PathEnv, fakeGodotOps(t, `echo "$@"`))
	root := t.TempDir()
	makeProject(t, root)
	seedFile(t, filepath.Join(root, "tests", "check.gd"), "extends SceneTree\n")

	result := callToolArgs(t, (*toolset).runScriptFile, root, `{"script":"res://tests/check.gd","args":["alpha","beta"]}`)
	if result.IsError {
		t.Fatalf("unexpected error: %+v", result.Content)
	}
	structured, ok := result.StructuredContent.(map[string]any)
	if !ok {
		t.Fatalf("structuredContent = %T, want object", result.StructuredContent)
	}
	stdout, _ := structured["stdout"].(string)
	if !strings.Contains(stdout, "-- alpha beta") {
		t.Errorf("stdout = %q, want the -- separator and forwarded args", stdout)
	}
}

func TestRunScriptFileNonZeroExitIsAResult(t *testing.T) {
	t.Setenv(godot.PathEnv, fakeGodotOps(t, `echo 'check failed' >&2; exit 3`))
	root := t.TempDir()
	makeProject(t, root)
	seedFile(t, filepath.Join(root, "tests", "check.gd"), "extends SceneTree\n")

	result := callToolArgs(t, (*toolset).runScriptFile, root, `{"script":"tests/check.gd"}`)
	if result.IsError {
		t.Fatalf("a non-zero exit is a domain result, not a tool error: %+v", result.Content)
	}
	structured, ok := result.StructuredContent.(map[string]any)
	if !ok {
		t.Fatalf("structuredContent = %T, want object", result.StructuredContent)
	}
	if structured["exitCode"] != 3 {
		t.Errorf("exitCode = %v, want 3", structured["exitCode"])
	}
	if stderr, _ := structured["stderr"].(string); !strings.Contains(stderr, "check failed") {
		t.Errorf("stderr = %q, want the failure output", stderr)
	}
}

func TestRunScriptFileSpawnFailure(t *testing.T) {
	t.Setenv(godot.PathEnv, filepath.Join(t.TempDir(), "missing-godot"))
	root := t.TempDir()
	makeProject(t, root)
	seedFile(t, filepath.Join(root, "tests", "check.gd"), "extends SceneTree\n")

	result := callToolArgs(t, (*toolset).runScriptFile, root, `{"script":"tests/check.gd"}`)
	if !result.IsError {
		t.Fatalf("expected an error result: %+v", result.Content)
	}
	if got := result.Content[0].Text; !strings.HasPrefix(got, "Failed to run script file: ") {
		t.Errorf("message = %q", got)
	}
}

func TestRunScriptFileValidation(t *testing.T) {
	root := t.TempDir()
	makeProject(t, root)
	notAProject := t.TempDir()

	tests := []struct {
		name    string
		project string
		args    string
		want    string
	}{
		{name: "missing script", project: root, args: `{}`, want: "script is required."},
		{name: "missing file", project: root, args: `{"script":"tests/nope.gd"}`, want: "Script file does not exist: tests/nope.gd"},
		{name: "invalid path", project: "../escape", args: `{"script":"check.gd"}`, want: "Invalid path."},
		{name: "escapes root", project: root, args: `{"script":"../escape.gd"}`, want: "Path escapes the project root."},
		{name: "not a project", project: notAProject, args: `{"script":"check.gd"}`, want: "Not a valid Godot project: "},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := callToolArgs(t, (*toolset).runScriptFile, tt.project, tt.args)
			if !result.IsError {
				t.Fatalf("expected an error result: %+v", result.Content)
			}
			if got := result.Content[0].Text; !strings.HasPrefix(got, tt.want) {
				t.Errorf("message = %q, want prefix %q", got, tt.want)
			}
		})
	}
}

type blockingHeadlessSession struct {
	fakeSession
	blockScript  bool
	blockProject bool
}

func (s *blockingHeadlessSession) RunHeadless(ctx context.Context, _ string, _ string, _ ...string) (string, string, int, error) {
	if s.blockScript {
		<-ctx.Done()
		return "", "", 0, fmt.Errorf("run godot: %w", ctx.Err())
	}
	return "", "", 0, nil
}

func (s *blockingHeadlessSession) RunProject(ctx context.Context, _ string, _ int, _ ...string) (string, string, int, error) {
	if s.blockProject {
		<-ctx.Done()
		return "", "", 0, fmt.Errorf("run godot: %w", ctx.Err())
	}
	return "", "", 0, nil
}

func TestRunScriptFileTimesOut(t *testing.T) {
	root := t.TempDir()
	makeProject(t, root)
	seedFile(t, filepath.Join(root, "tests", "check.gd"), "extends SceneTree\n")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	out, err := newToolset(root, &blockingHeadlessSession{blockScript: true}).runScriptFile().Handler(ctx, []byte(`{"script":"tests/check.gd"}`))
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	result, ok := out.(mcp.ToolResult)
	if !ok {
		t.Fatalf("result type %T", out)
	}
	if result.IsError {
		t.Fatalf("a timeout is a domain result: %+v", result.Content)
	}
	structured, ok := result.StructuredContent.(map[string]any)
	if !ok {
		t.Fatalf("structuredContent = %T, want object", result.StructuredContent)
	}
	if structured["timedOut"] != true {
		t.Errorf("timedOut = %v, want true", structured["timedOut"])
	}
	if structured["exitCode"] != -1 {
		t.Errorf("exitCode = %v, want -1", structured["exitCode"])
	}
	if !strings.Contains(result.Content[0].Text, "stopped at the timeout") {
		t.Errorf("text = %q, want a timeout note", result.Content[0].Text)
	}
}
