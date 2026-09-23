package tools

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
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
	if !ok || strings.Join(required, ",") != "projectPath,action" {
		t.Errorf("required = %v", tool.InputSchema["required"])
	}
	props, ok := tool.InputSchema["properties"].(map[string]any)
	if !ok {
		t.Fatalf("properties = %T", tool.InputSchema["properties"])
	}
	if len(props) != 5 {
		t.Errorf("properties = %v", props)
	}
	for _, key := range []string{"projectPath", "action", "actionName", "key"} {
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
	var actions map[string]string
	if err := json.Unmarshal([]byte(result.Content[0].Text), &actions); err != nil {
		t.Fatalf("unmarshal %q: %v", result.Content[0].Text, err)
	}
	if len(actions) != 2 {
		t.Fatalf("actions = %v", actions)
	}
	if got := actions["move_left"]; got != `{"deadzone": 0.5}` {
		t.Errorf("move_left = %q", got)
	}
	if got := actions["jump"]; got != `{"deadzone": 0.5}` {
		t.Errorf("jump = %q", got)
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
	if !strings.Contains(content, `move_left={"deadzone": 0.5, "events": [Object(InputEventKey,`) {
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
	if !strings.Contains(string(data), `jump={"deadzone": 1}`) {
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
		content := string(data)
		if strings.Contains(content, "\njump={\"deadzone\": 0.5}") {
			t.Errorf("jump not removed in:\n%s", content)
		}
		if !strings.Contains(content, `double_jump={"deadzone": 0.5}`) {
			t.Errorf("double_jump was corrupted or removed in:\n%s", content)
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
		content := string(data)
		if strings.Contains(content, `jump={"deadzone": 0.5}`) {
			t.Errorf("input jump not removed in:\n%s", content)
		}
		if !strings.Contains(content, `jump="*res://jump.gd"`) {
			t.Errorf("autoload jump unexpectedly removed in:\n%s", content)
		}
	})
}

func TestManageInputMapErrors(t *testing.T) {
	root := t.TempDir()
	makeProject(t, root)

	tests := []struct {
		name string
		args string
		want string
	}{
		{name: "missing params", args: `{}`, want: "projectPath and action are required."},
		{name: "missing action", args: `{"projectPath":` + jsonString(root) + `}`, want: "projectPath and action are required."},
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

func TestManageInputMapSnakeCaseAlias(t *testing.T) {
	root := t.TempDir()
	makeProject(t, root)

	result := callToolArgs(t, manageInputMap(), `{"project_path":`+jsonString(root)+`,"action":"add","action_name":"jump"}`)
	if result.IsError {
		t.Fatalf("unexpected error: %+v", result.Content)
	}
	if got := result.Content[0].Text; got != `Input action "jump" added.` {
		t.Errorf("text = %q", got)
	}
}
