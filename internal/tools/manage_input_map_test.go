package tools

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vilppuvuorinen/go-dot-mcp/internal/godotconfig"
)

func TestManageInputMapSchema(t *testing.T) {
	tool := manageInputMap()
	if tool.Name != "manage_input_map" {
		t.Errorf("name = %q", tool.Name)
	}
	if tool.Description != "Add, remove, or list input actions and bindings" {
		t.Errorf("description = %q", tool.Description)
	}
	required, ok := tool.InputSchema["required"].([]string)
	if !ok || strings.Join(required, ",") != "action" {
		t.Errorf("required = %v", tool.InputSchema["required"])
	}
	props, ok := tool.InputSchema["properties"].(map[string]any)
	if !ok {
		t.Fatalf("properties = %T", tool.InputSchema["properties"])
	}
	if len(props) != 4 {
		t.Errorf("properties = %v", props)
	}
	for _, key := range []string{"action", "actionName", "key"} {
		property, isObject := props[key].(map[string]any)
		if !isObject || property["type"] != "string" {
			t.Errorf("%s property = %v", key, props[key])
		}
	}
	deadzone, ok := props["deadzone"].(map[string]any)
	if !ok || deadzone["type"] != "number" {
		t.Errorf("deadzone property = %v", props["deadzone"])
	}
}

func TestManageInputMapList(t *testing.T) {
	root := t.TempDir()
	makeProject(t, root)
	seedFile(t, filepath.Join(root, "project.godot"), `config_version=5

[input]

move_left={"deadzone": 0.5}
jump={"deadzone": 0.5}

[application]

config/name="Demo"
`)

	result := callToolArgs(t, manageInputMap(), `{"projectPath":`+jsonString(root)+`,"action":"list"}`)
	if result.IsError {
		t.Fatalf("unexpected error: %+v", result.Content)
	}
	var actions map[string]any
	if err := json.Unmarshal([]byte(result.Content[0].Text), &actions); err != nil {
		t.Fatalf("unmarshal %q: %v", result.Content[0].Text, err)
	}
	if len(actions) != 2 {
		t.Fatalf("actions = %v", actions)
	}
	for _, name := range []string{"move_left", "jump"} {
		action, ok := actions[name].(map[string]any)
		if !ok || action["deadzone"] != 0.5 {
			t.Errorf("%s = %#v, want a dictionary with deadzone 0.5", name, actions[name])
		}
	}
}

func TestManageInputMapListEmpty(t *testing.T) {
	root := t.TempDir()
	makeProject(t, root)

	result := callToolArgs(t, manageInputMap(), `{"projectPath":`+jsonString(root)+`,"action":"list"}`)
	if result.IsError {
		t.Fatalf("unexpected error: %+v", result.Content)
	}
	if got := strings.TrimSpace(result.Content[0].Text); got != "{}" {
		t.Errorf("text = %q, want {}", got)
	}
}

func TestManageInputMapAddWithKey(t *testing.T) {
	root := t.TempDir()
	makeProject(t, root)

	result := callToolArgs(t, manageInputMap(), `{"projectPath":`+jsonString(root)+`,"action":"add","actionName":"move_left","key":"W"}`)
	if result.IsError {
		t.Fatalf("unexpected error: %+v", result.Content)
	}
	if got := result.Content[0].Text; got != `Input action "move_left" added.` {
		t.Errorf("text = %q", got)
	}
	data, err := os.ReadFile(filepath.Join(root, "project.godot"))
	if err != nil {
		t.Fatalf("read project.godot: %v", err)
	}
	content := string(data)
	if !strings.Contains(content, "move_left={\n\"deadzone\": 0.5,\n\"events\": [Object(InputEventKey,") {
		t.Errorf("content missing new action:\n%s", content)
	}
	if !strings.Contains(content, `"physical_keycode":87`) {
		t.Errorf("content missing scancode:\n%s", content)
	}
}

func TestManageInputMapAddWithoutKey(t *testing.T) {
	root := t.TempDir()
	makeProject(t, root)

	result := callToolArgs(t, manageInputMap(), `{"projectPath":`+jsonString(root)+`,"action":"add","actionName":"jump","deadzone":1}`)
	if result.IsError {
		t.Fatalf("unexpected error: %+v", result.Content)
	}
	if got := result.Content[0].Text; got != `Input action "jump" added.` {
		t.Errorf("text = %q", got)
	}
	data, err := os.ReadFile(filepath.Join(root, "project.godot"))
	if err != nil {
		t.Fatalf("read project.godot: %v", err)
	}
	if !strings.Contains(string(data), "jump={\n\"deadzone\": 1.0\n}") {
		t.Errorf("content missing action:\n%s", data)
	}
}

func TestManageInputMapMerge(t *testing.T) {
	root := t.TempDir()
	makeProject(t, root)
	existingEvent := `Object(InputEventKey,"resource_local_to_scene":false,"resource_name":"","device":-1,"window_id":0,"alt_pressed":false,"shift_pressed":false,"ctrl_pressed":false,"meta_pressed":false,"pressed":false,"keycode":0,"physical_keycode":87,"key_label":0,"unicode":0,"location":0,"echo":false,"script":null)`
	seedFile(t, filepath.Join(root, "project.godot"), "config_version=5\n\n[input]\n\nmove_left={\"deadzone\": 0.5, \"events\": ["+existingEvent+"]}\n")

	result := callToolArgs(t, manageInputMap(), `{"projectPath":`+jsonString(root)+`,"action":"add","actionName":"move_left","key":"D"}`)
	if result.IsError {
		t.Fatalf("unexpected error: %+v", result.Content)
	}
	if got := result.Content[0].Text; got != `Input action "move_left" updated (event merged into existing action).` {
		t.Errorf("text = %q", got)
	}
	data, err := os.ReadFile(filepath.Join(root, "project.godot"))
	if err != nil {
		t.Fatalf("read project.godot: %v", err)
	}
	content := string(data)
	if !strings.Contains(content, `"physical_keycode":87`) {
		t.Errorf("content lost existing event:\n%s", content)
	}
	if !strings.Contains(content, `"physical_keycode":68`) {
		t.Errorf("content missing merged event:\n%s", content)
	}
}

func TestManageInputMapRemove(t *testing.T) {
	root := t.TempDir()
	makeProject(t, root)
	seedFile(t, filepath.Join(root, "project.godot"), `config_version=5

[input]

move_left={"deadzone": 0.5}
jump={"deadzone": 0.5}
`)

	result := callToolArgs(t, manageInputMap(), `{"projectPath":`+jsonString(root)+`,"action":"remove","actionName":"move_left"}`)
	if result.IsError {
		t.Fatalf("unexpected error: %+v", result.Content)
	}
	if got := result.Content[0].Text; got != `Input action "move_left" removed.` {
		t.Errorf("text = %q", got)
	}
	data, err := os.ReadFile(filepath.Join(root, "project.godot"))
	if err != nil {
		t.Fatalf("read project.godot: %v", err)
	}
	content := string(data)
	if strings.Contains(content, "move_left") {
		t.Errorf("move_left not removed:\n%s", content)
	}
	if !strings.Contains(content, "jump=") {
		t.Errorf("jump unexpectedly removed:\n%s", content)
	}
}

func TestManageInputMapRemoveScoped(t *testing.T) {
	t.Run("does not match longer key", func(t *testing.T) {
		root := t.TempDir()
		makeProject(t, root)
		seedFile(t, filepath.Join(root, "project.godot"), `config_version=5

[input]

jump={"deadzone": 0.5}
double_jump={"deadzone": 0.5}
`)
		result := callToolArgs(t, manageInputMap(), `{"projectPath":`+jsonString(root)+`,"action":"remove","actionName":"jump"}`)
		if result.IsError {
			t.Fatalf("unexpected error: %+v", result.Content)
		}
		data, err := os.ReadFile(filepath.Join(root, "project.godot"))
		if err != nil {
			t.Fatalf("read project.godot: %v", err)
		}
		cfg, err := godotconfig.Parse(string(data))
		if err != nil {
			t.Fatalf("parse project.godot: %v", err)
		}
		if _, ok := cfg.Get("input", "jump"); ok {
			t.Errorf("jump not removed in:\n%s", data)
		}
		if _, ok := cfg.Get("input", "double_jump"); !ok {
			t.Errorf("double_jump was corrupted or removed in:\n%s", data)
		}
	})

	t.Run("does not match another section", func(t *testing.T) {
		root := t.TempDir()
		makeProject(t, root)
		seedFile(t, filepath.Join(root, "project.godot"), `config_version=5

[input]

jump={"deadzone": 0.5}

[autoload]

jump="*res://jump.gd"
`)
		result := callToolArgs(t, manageInputMap(), `{"projectPath":`+jsonString(root)+`,"action":"remove","actionName":"jump"}`)
		if result.IsError {
			t.Fatalf("unexpected error: %+v", result.Content)
		}
		data, err := os.ReadFile(filepath.Join(root, "project.godot"))
		if err != nil {
			t.Fatalf("read project.godot: %v", err)
		}
		cfg, err := godotconfig.Parse(string(data))
		if err != nil {
			t.Fatalf("parse project.godot: %v", err)
		}
		if _, ok := cfg.Get("input", "jump"); ok {
			t.Errorf("input jump not removed in:\n%s", data)
		}
		if _, ok := cfg.Get("autoload", "jump"); !ok {
			t.Errorf("autoload jump unexpectedly removed in:\n%s", data)
		}
	})
}

// TestManageInputMapEscaping covers action names that need an encoded key and
// checks they round-trip through list and remove.
func TestManageInputMapEscaping(t *testing.T) {
	tests := []struct {
		name     string
		action   string
		wantLine string
	}{
		{name: "quote in name", action: `jump"x`, wantLine: `"jump\"x"={`},
		{name: "newline in name", action: "jump\nx", wantLine: "\"jump\nx\"={"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			makeProject(t, root)

			result := callToolArgs(t, manageInputMap(),
				`{"projectPath":`+jsonString(root)+`,"action":"add","actionName":`+jsonString(tt.action)+`,"key":"W"}`)
			if result.IsError {
				t.Fatalf("unexpected error: %+v", result.Content)
			}
			data, err := os.ReadFile(filepath.Join(root, "project.godot"))
			if err != nil {
				t.Fatalf("read project.godot: %v", err)
			}
			if !strings.Contains(string(data), tt.wantLine) {
				t.Errorf("project.godot = %q, want it to contain %q", data, tt.wantLine)
			}

			list := callToolArgs(t, manageInputMap(), `{"projectPath":`+jsonString(root)+`,"action":"list"}`)
			var actions map[string]any
			if unmarshalErr := json.Unmarshal([]byte(list.Content[0].Text), &actions); unmarshalErr != nil {
				t.Fatalf("unmarshal %q: %v", list.Content[0].Text, unmarshalErr)
			}
			if _, ok := actions[tt.action]; !ok {
				t.Errorf("actions = %v, want key %q", actions, tt.action)
			}

			removed := callToolArgs(t, manageInputMap(),
				`{"projectPath":`+jsonString(root)+`,"action":"remove","actionName":`+jsonString(tt.action)+`}`)
			if removed.IsError {
				t.Fatalf("unexpected error: %+v", removed.Content)
			}
			data, err = os.ReadFile(filepath.Join(root, "project.godot"))
			if err != nil {
				t.Fatalf("read project.godot: %v", err)
			}
			if strings.Contains(string(data), "physical_keycode") {
				t.Errorf("action not removed:\n%s", data)
			}
		})
	}
}

// TestManageInputMapMergeMultiline seeds a Godot-style multi-line action whose
// events array holds an Object constructor, then merges a new binding. The
// existing deadzone and event must survive verbatim and a duplicate scancode
// must not be appended twice.
func TestManageInputMapMergeMultiline(t *testing.T) {
	existingEvent := `Object(InputEventKey,"resource_local_to_scene":false,"resource_name":"","device":-1,"window_id":0,"alt_pressed":false,"shift_pressed":false,"ctrl_pressed":false,"meta_pressed":false,"pressed":false,"keycode":0,"physical_keycode":87,"key_label":0,"unicode":0,"location":0,"echo":false,"script":null)`
	root := t.TempDir()
	makeProject(t, root)
	seedFile(t, filepath.Join(root, "project.godot"),
		"config_version=5\n\n[input]\n\nmove_left={\n\"deadzone\": 0.25,\n\"events\": ["+existingEvent+"]\n}\n")

	result := callToolArgs(t, manageInputMap(),
		`{"projectPath":`+jsonString(root)+`,"action":"add","actionName":"move_left","key":"D"}`)
	if result.IsError {
		t.Fatalf("unexpected error: %+v", result.Content)
	}
	data, err := os.ReadFile(filepath.Join(root, "project.godot"))
	if err != nil {
		t.Fatalf("read project.godot: %v", err)
	}
	content := string(data)
	for _, want := range []string{`"deadzone": 0.25`, `"physical_keycode":87`, `"physical_keycode":68`} {
		if !strings.Contains(content, want) {
			t.Errorf("content = %q, want it to contain %q", content, want)
		}
	}
	if !strings.Contains(content, existingEvent) {
		t.Errorf("existing event was rewritten:\n%s", content)
	}

	// Binding the same scancode again is a no-op.
	again := callToolArgs(t, manageInputMap(),
		`{"projectPath":`+jsonString(root)+`,"action":"add","actionName":"move_left","key":"W"}`)
	if again.IsError {
		t.Fatalf("unexpected error: %+v", again.Content)
	}
	data, err = os.ReadFile(filepath.Join(root, "project.godot"))
	if err != nil {
		t.Fatalf("read project.godot: %v", err)
	}
	if got := strings.Count(string(data), `"physical_keycode":87`); got != 1 {
		t.Errorf("physical_keycode 87 appears %d times, want 1:\n%s", got, data)
	}
}

func TestManageInputMapErrors(t *testing.T) {
	root := t.TempDir()
	makeProject(t, root)

	tests := []struct {
		name string
		args string
		want string
	}{
		{name: "missing params", args: `{}`, want: "action is required."},
		{name: "missing action", args: `{"projectPath":` + jsonString(root) + `}`, want: "action is required."},
		{name: "invalid path", args: `{"projectPath":"../escape","action":"list"}`, want: "Invalid path."},
		{name: "not a project", args: `{"projectPath":` + jsonString(t.TempDir()) + `,"action":"list"}`, want: "Not a valid Godot project: "},
		{name: "add missing actionName", args: `{"projectPath":` + jsonString(root) + `,"action":"add"}`, want: "actionName is required for add action."},
		{name: "remove missing actionName", args: `{"projectPath":` + jsonString(root) + `,"action":"remove"}`, want: "actionName is required for remove action."},
		{name: "invalid action", args: `{"projectPath":` + jsonString(root) + `,"action":"bogus"}`, want: `Invalid action. Use "list", "add", or "remove".`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := callToolArgs(t, manageInputMap(), tt.args)
			if !result.IsError {
				t.Fatalf("expected an error result: %+v", result.Content)
			}
			if got := result.Content[0].Text; !strings.HasPrefix(got, tt.want) {
				t.Errorf("message = %q, want prefix %q", got, tt.want)
			}
		})
	}
}

// TestKeyNameToScancodeGodot4 pins the Godot 4 Key values. Godot 3 used a
// different KEY_SPECIAL bit (1<<24), so special keys must not regress to those
// values.
func TestKeyNameToScancodeGodot4(t *testing.T) {
	tests := []struct {
		key  string
		want int
	}{
		{"W", 87},
		{"a", 65},
		{"7", 55},
		{"SPACE", 32},
		{"ESCAPE", 4194305},
		{"TAB", 4194306},
		{"BACKSPACE", 4194308},
		{"ENTER", 4194309},
		{"LEFT", 4194319},
		{"UP", 4194320},
		{"RIGHT", 4194321},
		{"DOWN", 4194322},
		{"SHIFT", 4194325},
		{"CTRL", 4194326},
		{"ALT", 4194328},
		{"F1", 4194332},
		{"F12", 4194343},
		{"unknown", 0},
	}
	for _, tt := range tests {
		if got := keyNameToScancode(schemaGodot4, tt.key); got != tt.want {
			t.Errorf("keyNameToScancode(%q) = %d, want %d", tt.key, got, tt.want)
		}
	}
}

func TestManageInputMapAddSpecialKey(t *testing.T) {
	root := t.TempDir()
	makeProject(t, root)

	result := callToolArgs(t, manageInputMap(), `{"projectPath":`+jsonString(root)+`,"action":"add","actionName":"jump","key":"ENTER"}`)
	if result.IsError {
		t.Fatalf("unexpected error: %+v", result.Content)
	}
	data, err := os.ReadFile(filepath.Join(root, "project.godot"))
	if err != nil {
		t.Fatalf("read project.godot: %v", err)
	}
	content := string(data)
	if !strings.Contains(content, `"physical_keycode":4194309`) {
		t.Errorf("content missing Godot 4 ENTER scancode:\n%s", content)
	}
	if strings.Contains(content, `"physical_keycode":16777221`) {
		t.Errorf("content contains Godot 3 ENTER scancode:\n%s", content)
	}
}

func TestManageInputMapRejectsUnsupportedSchema(t *testing.T) {
	root := t.TempDir()
	seedFile(t, filepath.Join(root, "project.godot"), "config_version=4\n")

	result := callToolArgs(t, manageInputMap(), `{"projectPath":`+jsonString(root)+`,"action":"list"}`)
	if !result.IsError {
		t.Fatalf("expected an error result: %+v", result.Content)
	}
	if got := result.Content[0].Text; !strings.HasPrefix(got, "Unsupported Godot project: ") {
		t.Errorf("message = %q", got)
	}
}
