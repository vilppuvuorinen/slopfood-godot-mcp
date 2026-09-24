package tools

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vilppuvuorinen/go-dot-mcp/internal/godot"
)

func TestValidateScriptSchema(t *testing.T) {
	tool := validateScript()
	if tool.Name != "validate_script" {
		t.Errorf("name = %q", tool.Name)
	}
	if tool.Description != "Check a GDScript file for syntax/type errors (headless, no run)" {
		t.Errorf("description = %q", tool.Description)
	}
	required, ok := tool.InputSchema["required"].([]string)
	if !ok || strings.Join(required, ",") != "projectPath,scriptPath" {
		t.Errorf("required = %v", tool.InputSchema["required"])
	}
	props, ok := tool.InputSchema["properties"].(map[string]any)
	if !ok {
		t.Fatalf("properties = %T", tool.InputSchema["properties"])
	}
	for _, key := range []string{"projectPath", "scriptPath"} {
		prop, exists := props[key]
		if !exists {
			t.Fatalf("missing property %q", key)
		}
		if prop.(map[string]any)["type"] != "string" {
			t.Errorf("property %q = %v", key, prop)
		}
	}
}

func TestValidateScriptValid(t *testing.T) {
	t.Setenv(godot.PathEnv, fakeGodotOps(t, ``))
	root := t.TempDir()
	makeProject(t, root)
	seedFile(t, filepath.Join(root, "scripts", "player.gd"), "extends Node\n")

	result := callToolArgs(t, validateScript(), `{"projectPath":`+jsonString(root)+`,"scriptPath":"scripts/player.gd"}`)
	if result.IsError {
		t.Fatalf("unexpected error: %+v", result.Content)
	}
	var payload validateScriptResult
	if err := json.Unmarshal([]byte(result.Content[0].Text), &payload); err != nil {
		t.Fatalf("unmarshal %q: %v", result.Content[0].Text, err)
	}
	if !payload.Valid || payload.ErrorCount != 0 || len(payload.Errors) != 0 {
		t.Errorf("payload = %+v", payload)
	}
	if payload.ScriptPath != "scripts/player.gd" {
		t.Errorf("scriptPath = %q", payload.ScriptPath)
	}
}

func TestValidateScriptDiagnostics(t *testing.T) {
	// Godot exits 0 even when the script fails to compile, so the diagnostic on
	// stderr is the only signal the checker reports.
	t.Setenv(godot.PathEnv, fakeGodotOps(t, `echo "SCRIPT ERROR: Parse Error: boom" >&2
echo "   at: GDScript::reload (res://scripts/player.gd:3)" >&2
exit 0`))
	root := t.TempDir()
	makeProject(t, root)
	seedFile(t, filepath.Join(root, "scripts", "player.gd"), "extends Node\nfunc _ready(:\n")

	result := callToolArgs(t, validateScript(), `{"projectPath":`+jsonString(root)+`,"scriptPath":"scripts/player.gd"}`)
	if result.IsError {
		t.Fatalf("unexpected error: %+v", result.Content)
	}
	var payload validateScriptResult
	if err := json.Unmarshal([]byte(result.Content[0].Text), &payload); err != nil {
		t.Fatalf("unmarshal %q: %v", result.Content[0].Text, err)
	}
	if payload.Valid || payload.ErrorCount != 1 || len(payload.Errors) != 1 {
		t.Fatalf("payload = %+v", payload)
	}
	got := payload.Errors[0]
	if got.Message != "boom" || got.File != "res://scripts/player.gd" || got.Line != 3 {
		t.Errorf("diagnostic = %+v", got)
	}
}

func TestValidateScriptCheckerFailure(t *testing.T) {
	// A non-zero exit with no recognizable diagnostic is reported as a checker
	// failure: the exit code and stderr are combined.
	t.Setenv(godot.PathEnv, fakeGodotOps(t, `echo "boom" >&2
exit 3`))
	root := t.TempDir()
	makeProject(t, root)
	seedFile(t, filepath.Join(root, "scripts", "player.gd"), "extends Node\n")

	result := callToolArgs(t, validateScript(), `{"projectPath":`+jsonString(root)+`,"scriptPath":"scripts/player.gd"}`)
	if !result.IsError {
		t.Fatalf("expected an error result: %+v", result.Content)
	}
	want := "validate_script could not check the script; "
	if !strings.HasPrefix(result.Content[0].Text, want) {
		t.Errorf("message = %q, want prefix %q", result.Content[0].Text, want)
	}
}

func TestValidateScriptAlias(t *testing.T) {
	t.Setenv(godot.PathEnv, fakeGodotOps(t, ``))
	root := t.TempDir()
	makeProject(t, root)
	seedFile(t, filepath.Join(root, "player.gd"), "extends Node\n")

	result := callToolArgs(t, validateScript(), `{"project_path":`+jsonString(root)+`,"script_path":"player.gd"}`)
	if result.IsError {
		t.Fatalf("unexpected error: %+v", result.Content)
	}
}

func TestValidateScriptValidation(t *testing.T) {
	root := t.TempDir()
	makeProject(t, root)
	seedFile(t, filepath.Join(root, "scripts", "player.gd"), "extends Node\n")

	tests := []struct {
		name string
		args string
		want string
	}{
		{name: "missing params", args: `{}`, want: "projectPath and scriptPath are required."},
		{name: "missing scriptPath", args: `{"projectPath":` + jsonString(root) + `}`, want: "projectPath and scriptPath are required."},
		{name: "invalid path", args: `{"projectPath":"../escape","scriptPath":"player.gd"}`, want: "Invalid path."},
		{name: "non gd", args: `{"projectPath":` + jsonString(root) + `,"scriptPath":"player.cs"}`, want: "validate_script only checks GDScript (.gd) files."},
		{name: "not a project", args: `{"projectPath":` + jsonString(t.TempDir()) + `,"scriptPath":"player.gd"}`, want: "Not a valid Godot project: "},
		{name: "missing script", args: `{"projectPath":` + jsonString(root) + `,"scriptPath":"scripts/missing.gd"}`, want: "Script does not exist: scripts/missing.gd"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := callToolArgs(t, validateScript(), tt.args)
			if !result.IsError {
				t.Fatalf("expected an error result: %+v", result.Content)
			}
			if !strings.HasPrefix(result.Content[0].Text, tt.want) {
				t.Errorf("message = %q, want prefix %q", result.Content[0].Text, tt.want)
			}
		})
	}
}

func TestValidateScriptSpawnFailure(t *testing.T) {
	t.Setenv(godot.PathEnv, filepath.Join(t.TempDir(), "missing-godot"))
	root := t.TempDir()
	makeProject(t, root)
	seedFile(t, filepath.Join(root, "scripts", "player.gd"), "extends Node\n")

	result := callToolArgs(t, validateScript(), `{"projectPath":`+jsonString(root)+`,"scriptPath":"scripts/player.gd"}`)
	if !result.IsError {
		t.Fatalf("expected an error result: %+v", result.Content)
	}
	want := "validate_script could not check the script; "
	if !strings.HasPrefix(result.Content[0].Text, want) {
		t.Errorf("message = %q, want prefix %q", result.Content[0].Text, want)
	}
}
