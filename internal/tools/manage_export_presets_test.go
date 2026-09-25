package tools

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestManageExportPresetsSchema(t *testing.T) {
	tool := manageExportPresets()
	if tool.Name != "manage_export_presets" {
		t.Errorf("name = %q", tool.Name)
	}
	if tool.Description != "Create or modify export preset configuration" {
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
	for _, key := range []string{"action", "name", "platform"} {
		prop, isString := props[key].(map[string]any)
		if !isString || prop["type"] != "string" {
			t.Errorf("%s property = %v", key, props[key])
		}
	}
	runnable, ok := props["runnable"].(map[string]any)
	if !ok || runnable["type"] != "boolean" {
		t.Errorf("runnable property = %v", props["runnable"])
	}
}

func TestManageExportPresetsListEmpty(t *testing.T) {
	root := t.TempDir()
	makeProject(t, root)

	result := callToolArgs(t, manageExportPresets(), `{"projectPath":`+jsonString(root)+`,"action":"list"}`)
	if result.IsError {
		t.Fatalf("unexpected error: %+v", result.Content)
	}
	if got, want := result.Content[0].Text, "{\n  \"presets\": []\n}"; got != want {
		t.Errorf("text = %q, want %q", got, want)
	}
}

func TestManageExportPresetsAddAndList(t *testing.T) {
	root := t.TempDir()
	makeProject(t, root)

	result := callToolArgs(t, manageExportPresets(), `{"projectPath":`+jsonString(root)+`,"action":"add","name":"Linux","platform":"Linux","runnable":true}`)
	if result.IsError {
		t.Fatalf("unexpected error: %+v", result.Content)
	}
	if got, want := result.Content[0].Text, `Export preset "Linux" added for platform "Linux".`; got != want {
		t.Errorf("text = %q, want %q", got, want)
	}
	content, err := os.ReadFile(filepath.Join(root, "export_presets.cfg"))
	if err != nil {
		t.Fatalf("read export_presets.cfg: %v", err)
	}
	for _, want := range []string{"[preset.", `name="Linux"`, `platform="Linux"`, "runnable=true"} {
		if !strings.Contains(string(content), want) {
			t.Errorf("export_presets.cfg = %q, want it to contain %q", content, want)
		}
	}

	list := callToolArgs(t, manageExportPresets(), `{"projectPath":`+jsonString(root)+`,"action":"list"}`)
	if list.IsError {
		t.Fatalf("unexpected error: %+v", list.Content)
	}
	var decoded exportPresetList
	if err := json.Unmarshal([]byte(list.Content[0].Text), &decoded); err != nil {
		t.Fatalf("unmarshal %q: %v", list.Content[0].Text, err)
	}
	if len(decoded.Presets) != 1 || decoded.Presets[0].Name != "Linux" || decoded.Presets[0].Platform != "Linux" {
		t.Errorf("presets = %+v", decoded.Presets)
	}
}

func TestManageExportPresetsAddRunnableDefaultsFalse(t *testing.T) {
	root := t.TempDir()
	makeProject(t, root)

	result := callToolArgs(t, manageExportPresets(), `{"projectPath":`+jsonString(root)+`,"action":"add","name":"Web","platform":"Web"}`)
	if result.IsError {
		t.Fatalf("unexpected error: %+v", result.Content)
	}
	content, err := os.ReadFile(filepath.Join(root, "export_presets.cfg"))
	if err != nil {
		t.Fatalf("read export_presets.cfg: %v", err)
	}
	if !strings.Contains(string(content), "runnable=false") {
		t.Errorf("export_presets.cfg = %q, want runnable=false", content)
	}
}

func TestManageExportPresetsListUnknownPlatform(t *testing.T) {
	root := t.TempDir()
	makeProject(t, root)
	seedFile(t, filepath.Join(root, "export_presets.cfg"), "[preset.1]\n\nname=\"Nameless\"\n")

	result := callToolArgs(t, manageExportPresets(), `{"projectPath":`+jsonString(root)+`,"action":"list"}`)
	if result.IsError {
		t.Fatalf("unexpected error: %+v", result.Content)
	}
	var decoded exportPresetList
	if err := json.Unmarshal([]byte(result.Content[0].Text), &decoded); err != nil {
		t.Fatalf("unmarshal %q: %v", result.Content[0].Text, err)
	}
	if len(decoded.Presets) != 1 || decoded.Presets[0].Platform != "unknown" {
		t.Errorf("presets = %+v", decoded.Presets)
	}
}

func TestManageExportPresetsRemove(t *testing.T) {
	root := t.TempDir()
	makeProject(t, root)
	seedFile(t, filepath.Join(root, "export_presets.cfg"), `[preset.0]

name="Linux"
platform="Linux"
runnable=true

[preset.1]

name="Web"
platform="Web"
runnable=false
`)

	result := callToolArgs(t, manageExportPresets(), `{"projectPath":`+jsonString(root)+`,"action":"remove","name":"Linux"}`)
	if result.IsError {
		t.Fatalf("unexpected error: %+v", result.Content)
	}
	if got, want := result.Content[0].Text, `Export preset "Linux" removed.`; got != want {
		t.Errorf("text = %q, want %q", got, want)
	}
	content, err := os.ReadFile(filepath.Join(root, "export_presets.cfg"))
	if err != nil {
		t.Fatalf("read export_presets.cfg: %v", err)
	}
	if strings.Contains(string(content), `name="Linux"`) {
		t.Errorf("export_presets.cfg = %q, want the Linux preset removed", content)
	}
	if !strings.Contains(string(content), `name="Web"`) {
		t.Errorf("export_presets.cfg = %q, want the Web preset kept", content)
	}
}

func TestManageExportPresetsRemoveMissingFile(t *testing.T) {
	root := t.TempDir()
	makeProject(t, root)

	result := callToolArgs(t, manageExportPresets(), `{"projectPath":`+jsonString(root)+`,"action":"remove","name":"Linux"}`)
	if !result.IsError {
		t.Fatalf("expected an error result: %+v", result.Content)
	}
	if got, want := result.Content[0].Text, "No export_presets.cfg file found."; got != want {
		t.Errorf("text = %q, want %q", got, want)
	}
}

// TestManageExportPresetsEscaping covers preset names and platforms that need
// escaping, checks list decodes them, and confirms remove matches the encoded
// name.
func TestManageExportPresetsEscaping(t *testing.T) {
	tests := []struct {
		name     string
		preset   string
		platform string
	}{
		{name: "quote in name and platform", preset: `Evil"Preset`, platform: `Linux"x`},
		{name: "newline in name", preset: "Evil\nPreset", platform: "Linux"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			makeProject(t, root)

			result := callToolArgs(t, manageExportPresets(),
				`{"projectPath":`+jsonString(root)+`,"action":"add","name":`+jsonString(tt.preset)+`,"platform":`+jsonString(tt.platform)+`}`)
			if result.IsError {
				t.Fatalf("unexpected error: %+v", result.Content)
			}
			data, err := os.ReadFile(filepath.Join(root, "export_presets.cfg"))
			if err != nil {
				t.Fatalf("read export_presets.cfg: %v", err)
			}
			wantName := "name=" + `"` + strings.ReplaceAll(strings.ReplaceAll(tt.preset, `\`, `\\`), `"`, `\"`) + `"`
			if !strings.Contains(string(data), wantName) {
				t.Errorf("export_presets.cfg = %q, want it to contain %q", data, wantName)
			}

			list := callToolArgs(t, manageExportPresets(), `{"projectPath":`+jsonString(root)+`,"action":"list"}`)
			var decoded exportPresetList
			if unmarshalErr := json.Unmarshal([]byte(list.Content[0].Text), &decoded); unmarshalErr != nil {
				t.Fatalf("unmarshal %q: %v", list.Content[0].Text, unmarshalErr)
			}
			if len(decoded.Presets) != 1 || decoded.Presets[0].Name != tt.preset || decoded.Presets[0].Platform != tt.platform {
				t.Errorf("presets = %+v, want name %q platform %q", decoded.Presets, tt.preset, tt.platform)
			}

			remove := callToolArgs(t, manageExportPresets(),
				`{"projectPath":`+jsonString(root)+`,"action":"remove","name":`+jsonString(tt.preset)+`}`)
			if remove.IsError {
				t.Fatalf("unexpected error: %+v", remove.Content)
			}
			data, err = os.ReadFile(filepath.Join(root, "export_presets.cfg"))
			if err != nil {
				t.Fatalf("read export_presets.cfg: %v", err)
			}
			if strings.Contains(string(data), "[preset.") {
				t.Errorf("preset not removed:\n%s", data)
			}
		})
	}
}

func TestManageExportPresetsErrors(t *testing.T) {
	projectRoot := t.TempDir()
	makeProject(t, projectRoot)

	tests := []struct {
		name string
		args string
		want string
	}{
		{
			name: "missing params",
			args: `{}`,
			want: "action is required.",
		},
		{
			name: "invalid path",
			args: `{"projectPath":"../escape","action":"list"}`,
			want: "Invalid path.",
		},
		{
			name: "not a project",
			args: `{"projectPath":` + jsonString(t.TempDir()) + `,"action":"list"}`,
			want: "Not a valid Godot project: ",
		},
		{
			name: "add missing platform",
			args: `{"projectPath":` + jsonString(projectRoot) + `,"action":"add","name":"Linux"}`,
			want: "name and platform are required for add action.",
		},
		{
			name: "add missing name",
			args: `{"projectPath":` + jsonString(projectRoot) + `,"action":"add","platform":"Linux"}`,
			want: "name and platform are required for add action.",
		},
		{
			name: "remove missing name",
			args: `{"projectPath":` + jsonString(projectRoot) + `,"action":"remove"}`,
			want: "name is required for remove action.",
		},
		{
			name: "unknown action",
			args: `{"projectPath":` + jsonString(projectRoot) + `,"action":"bogus"}`,
			want: `Invalid action. Use "list", "add", or "remove".`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := callToolArgs(t, manageExportPresets(), tt.args)
			if !result.IsError {
				t.Fatalf("expected an error result: %+v", result.Content)
			}
			if got := result.Content[0].Text; !strings.HasPrefix(got, tt.want) {
				t.Errorf("message = %q, want prefix %q", got, tt.want)
			}
		})
	}
}
