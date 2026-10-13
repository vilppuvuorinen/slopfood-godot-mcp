package e2e

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestRunScriptFileTool(t *testing.T) {
	t.Parallel()
	root := newProject(t)
	session := connectProject(t, root)
	writeE2EFile(t, filepath.Join(root, "tests", "check.gd"), `extends SceneTree
func _init():
	print("hello from check")
	quit(0)
`)

	res := callTool(t, session, "run_script_file", map[string]any{"script": "tests/check.gd"})
	if res.IsError {
		t.Fatalf("run_script_file error: %s", firstText(t, res))
	}
	structured, ok := res.StructuredContent.(map[string]any)
	if !ok {
		t.Fatalf("structuredContent type %T", res.StructuredContent)
	}
	if structured["script"] != "res://tests/check.gd" {
		t.Errorf("script = %v", structured["script"])
	}
	if structured["exitCode"] != float64(0) {
		t.Errorf("exitCode = %v, want 0", structured["exitCode"])
	}
	if structured["timedOut"] != false {
		t.Errorf("timedOut = %v, want false", structured["timedOut"])
	}
	if stdout, _ := structured["stdout"].(string); !strings.Contains(stdout, "hello from check") {
		t.Errorf("stdout = %q, want the script's print", stdout)
	}
}

func TestRunScriptFileForwardsArgs(t *testing.T) {
	t.Parallel()
	root := newProject(t)
	session := connectProject(t, root)
	writeE2EFile(t, filepath.Join(root, "tests", "args.gd"), `extends SceneTree
func _init():
	print("args=", OS.get_cmdline_user_args())
	quit(0)
`)

	res := callTool(t, session, "run_script_file", map[string]any{
		"script": "tests/args.gd",
		"args":   []any{"alpha", "beta"},
	})
	if res.IsError {
		t.Fatalf("run_script_file error: %s", firstText(t, res))
	}
	structured, ok := res.StructuredContent.(map[string]any)
	if !ok {
		t.Fatalf("structuredContent type %T", res.StructuredContent)
	}
	if stdout, _ := structured["stdout"].(string); !strings.Contains(stdout, `args=["alpha", "beta"]`) {
		t.Errorf("stdout = %q, want the forwarded user args", stdout)
	}
}

func TestRunScriptFileNonZeroExit(t *testing.T) {
	t.Parallel()
	root := newProject(t)
	session := connectProject(t, root)
	writeE2EFile(t, filepath.Join(root, "tests", "fail.gd"), `extends SceneTree
func _init():
	printerr("deliberate failure")
	quit(3)
`)

	res := callTool(t, session, "run_script_file", map[string]any{"script": "tests/fail.gd"})
	if res.IsError {
		t.Fatalf("a non-zero exit is a domain result, not an error: %s", firstText(t, res))
	}
	structured, ok := res.StructuredContent.(map[string]any)
	if !ok {
		t.Fatalf("structuredContent type %T", res.StructuredContent)
	}
	if structured["exitCode"] != float64(3) {
		t.Errorf("exitCode = %v, want 3", structured["exitCode"])
	}
	if stderr, _ := structured["stderr"].(string); !strings.Contains(stderr, "deliberate failure") {
		t.Errorf("stderr = %q, want the failure output", stderr)
	}
}

func TestRunScriptFileErrors(t *testing.T) {
	t.Parallel()

	t.Run("missing script", func(t *testing.T) {
		root := newProject(t)
		session := connectProject(t, root)
		requireToolError(t, session, "run_script_file", map[string]any{}, "script is required.")
	})

	t.Run("missing file", func(t *testing.T) {
		root := newProject(t)
		session := connectProject(t, root)
		requireToolError(t, session, "run_script_file", map[string]any{
			"script": "tests/nope.gd",
		}, "Script file does not exist: tests/nope.gd")
	})

	t.Run("not a project", func(t *testing.T) {
		notProject := t.TempDir()
		session := connectProject(t, notProject)
		requireToolError(t, session, "run_script_file", map[string]any{
			"script": "check.gd",
		}, "Not a valid Godot project: "+notProject)
	})
}
