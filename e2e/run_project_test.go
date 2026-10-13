package e2e

import (
	"path/filepath"
	"strings"
	"testing"
)

func writeRunnableProject(t *testing.T, root string) {
	t.Helper()
	writeE2EFile(t, filepath.Join(root, "project.godot"), `config_version=5

[application]
config/name="run_project_e2e"
run/main_scene="res://main.tscn"
`)
	writeE2EFile(t, filepath.Join(root, "main.gd"), `extends Node
func _ready():
	print("project ready user=", OS.get_cmdline_user_args())
`)
	writeE2EFile(t, filepath.Join(root, "main.tscn"), `[gd_scene load_steps=2 format=3]

[ext_resource type="Script" path="res://main.gd" id="1"]

[node name="Main" type="Node"]
script = ExtResource("1")
`)
}

func TestRunProjectTool(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeRunnableProject(t, root)
	session := connectProject(t, root)

	res := callTool(t, session, "run_project", map[string]any{"quitAfter": 2})
	if res.IsError {
		t.Fatalf("run_project error: %s", firstText(t, res))
	}
	structured, ok := res.StructuredContent.(map[string]any)
	if !ok {
		t.Fatalf("structuredContent type %T", res.StructuredContent)
	}
	if structured["quitAfter"] != float64(2) {
		t.Errorf("quitAfter = %v, want 2", structured["quitAfter"])
	}
	if structured["exitCode"] != float64(0) {
		t.Errorf("exitCode = %v, want 0", structured["exitCode"])
	}
	if stdout, _ := structured["stdout"].(string); !strings.Contains(stdout, "project ready") {
		t.Errorf("stdout = %q, want the main scene's _ready output", stdout)
	}
}

func TestRunProjectDefaultsToOneFrame(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeRunnableProject(t, root)
	session := connectProject(t, root)

	res := callTool(t, session, "run_project", map[string]any{})
	if res.IsError {
		t.Fatalf("run_project error: %s", firstText(t, res))
	}
	structured, ok := res.StructuredContent.(map[string]any)
	if !ok {
		t.Fatalf("structuredContent type %T", res.StructuredContent)
	}
	if structured["quitAfter"] != float64(1) {
		t.Errorf("quitAfter = %v, want 1", structured["quitAfter"])
	}
}

// A project with no main scene fails at boot; the failure is reported as a
// non-zero exit rather than a tool error.
func TestRunProjectWithoutMainScene(t *testing.T) {
	t.Parallel()
	root := newProject(t)
	session := connectProject(t, root)

	res := callTool(t, session, "run_project", map[string]any{"quitAfter": 1})
	if res.IsError {
		t.Fatalf("a boot failure is a domain result, not an error: %s", firstText(t, res))
	}
	structured, ok := res.StructuredContent.(map[string]any)
	if !ok {
		t.Fatalf("structuredContent type %T", res.StructuredContent)
	}
	if structured["exitCode"] == float64(0) {
		t.Errorf("exitCode = %v, want non-zero", structured["exitCode"])
	}
	if stdout, _ := structured["stdout"].(string); !strings.Contains(stdout, "no main scene") {
		t.Errorf("stdout = %q, want the engine's no-main-scene diagnostic", stdout)
	}
}

func TestRunProjectErrors(t *testing.T) {
	t.Parallel()

	t.Run("zero frames", func(t *testing.T) {
		root := newProject(t)
		session := connectProject(t, root)
		requireToolError(t, session, "run_project", map[string]any{"quitAfter": 0}, "quitAfter must be at least 1.")
	})

	t.Run("not a project", func(t *testing.T) {
		notProject := t.TempDir()
		session := connectProject(t, notProject)
		requireToolError(t, session, "run_project", map[string]any{}, "Not a valid Godot project: "+notProject)
	})
}
