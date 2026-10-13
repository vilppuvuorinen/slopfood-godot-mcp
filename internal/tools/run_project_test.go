package tools

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/vilppuvuorinen/slopfood-godot-mcp/internal/godot"
	"github.com/vilppuvuorinen/slopfood-godot-mcp/internal/mcp"
)

func TestRunProjectSchema(t *testing.T) {
	tool := newToolset("", nil).runProject()
	if tool.Name != "run_project" {
		t.Errorf("name = %q", tool.Name)
	}
	if required, ok := tool.InputSchema["required"].([]string); !ok || len(required) != 0 {
		t.Errorf("required = %v, want empty", tool.InputSchema["required"])
	}
	props, ok := tool.InputSchema["properties"].(map[string]any)
	if !ok {
		t.Fatalf("properties = %T", tool.InputSchema["properties"])
	}
	quitAfter, ok := props["quitAfter"].(map[string]any)
	if !ok || quitAfter["type"] != "integer" || quitAfter["minimum"] != 1 {
		t.Errorf("quitAfter property = %v", props["quitAfter"])
	}
	if prop, ok := props["args"].(map[string]any); !ok || prop["type"] != "array" {
		t.Errorf("args property = %v", props["args"])
	}
}

func TestRunProjectDefaultsToOneFrame(t *testing.T) {
	t.Setenv(godot.PathEnv, fakeGodotOps(t, `echo "$@"`))
	root := t.TempDir()
	makeProject(t, root)

	result := callToolArgs(t, (*toolset).runProject, root, `{}`)
	if result.IsError {
		t.Fatalf("unexpected error: %+v", result.Content)
	}
	text := result.Content[0].Text
	for _, want := range []string{"--headless", "--quit-after 1", "exit code 0"} {
		if !strings.Contains(text, want) {
			t.Errorf("text = %q, want it to contain %q", text, want)
		}
	}
	structured, ok := result.StructuredContent.(map[string]any)
	if !ok {
		t.Fatalf("structuredContent = %T, want object", result.StructuredContent)
	}
	if structured["quitAfter"] != 1 {
		t.Errorf("quitAfter = %v, want 1", structured["quitAfter"])
	}
	if structured["exitCode"] != 0 {
		t.Errorf("exitCode = %v, want 0", structured["exitCode"])
	}
}

func TestRunProjectHonorsQuitAfterAndArgs(t *testing.T) {
	t.Setenv(godot.PathEnv, fakeGodotOps(t, `echo "$@"`))
	root := t.TempDir()
	makeProject(t, root)

	result := callToolArgs(t, (*toolset).runProject, root, `{"quitAfter":5,"args":["--level","2"]}`)
	if result.IsError {
		t.Fatalf("unexpected error: %+v", result.Content)
	}
	structured, ok := result.StructuredContent.(map[string]any)
	if !ok {
		t.Fatalf("structuredContent = %T, want object", result.StructuredContent)
	}
	if structured["quitAfter"] != 5 {
		t.Errorf("quitAfter = %v, want 5", structured["quitAfter"])
	}
	stdout, _ := structured["stdout"].(string)
	for _, want := range []string{"--quit-after 5", "-- --level 2"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("stdout = %q, want it to contain %q", stdout, want)
		}
	}
}

func TestRunProjectNonZeroExitIsAResult(t *testing.T) {
	t.Setenv(godot.PathEnv, fakeGodotOps(t, `echo 'no main scene' >&2; exit 1`))
	root := t.TempDir()
	makeProject(t, root)

	result := callToolArgs(t, (*toolset).runProject, root, `{}`)
	if result.IsError {
		t.Fatalf("a non-zero exit is a domain result, not a tool error: %+v", result.Content)
	}
	structured, ok := result.StructuredContent.(map[string]any)
	if !ok {
		t.Fatalf("structuredContent = %T, want object", result.StructuredContent)
	}
	if structured["exitCode"] != 1 {
		t.Errorf("exitCode = %v, want 1", structured["exitCode"])
	}
	if stderr, _ := structured["stderr"].(string); !strings.Contains(stderr, "no main scene") {
		t.Errorf("stderr = %q, want the failure output", stderr)
	}
}

func TestRunProjectSpawnFailure(t *testing.T) {
	t.Setenv(godot.PathEnv, filepath.Join(t.TempDir(), "missing-godot"))
	root := t.TempDir()
	makeProject(t, root)

	result := callToolArgs(t, (*toolset).runProject, root, `{}`)
	if !result.IsError {
		t.Fatalf("expected an error result: %+v", result.Content)
	}
	if got := result.Content[0].Text; !strings.HasPrefix(got, "Failed to run project: ") {
		t.Errorf("message = %q", got)
	}
}

func TestRunProjectValidation(t *testing.T) {
	root := t.TempDir()
	makeProject(t, root)
	notAProject := t.TempDir()

	tests := []struct {
		name    string
		project string
		args    string
		want    string
	}{
		{name: "zero frames", project: root, args: `{"quitAfter":0}`, want: "quitAfter must be at least 1."},
		{name: "negative frames", project: root, args: `{"quitAfter":-3}`, want: "quitAfter must be at least 1."},
		{name: "invalid path", project: "../escape", args: `{}`, want: "Invalid path."},
		{name: "not a project", project: notAProject, args: `{}`, want: "Not a valid Godot project: "},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := callToolArgs(t, (*toolset).runProject, tt.project, tt.args)
			if !result.IsError {
				t.Fatalf("expected an error result: %+v", result.Content)
			}
			if got := result.Content[0].Text; !strings.HasPrefix(got, tt.want) {
				t.Errorf("message = %q, want prefix %q", got, tt.want)
			}
		})
	}
}

func TestRunProjectTimesOut(t *testing.T) {
	root := t.TempDir()
	makeProject(t, root)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	out, err := newToolset(root, &blockingHeadlessSession{blockProject: true}).runProject().Handler(ctx, []byte(`{}`))
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
}
