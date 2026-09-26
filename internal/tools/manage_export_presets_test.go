package tools

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vilppuvuorinen/slopfood-godot-mcp/internal/mcp"
)

func TestManageExportPresetsSchema(t *testing.T) {
	tool := manageExportPresets()
	if tool.Name != "manage_export_presets" {
		t.Errorf("name = %q", tool.Name)
	}
	if tool.Description != "Create or remove export preset configuration" {
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

// TestManageExportPresetsEchoesAction pins that a success envelope reports the
// action the caller sent, so a client can map the result back to it.
func TestManageExportPresetsEchoesAction(t *testing.T) {
	tests := []struct {
		name string
		args string
		want string
	}{
		{name: "add", args: `{"action":"add","name":"Linux","platform":"Linux"}`, want: "add"},
		{name: "remove", args: `{"action":"remove","name":"Linux"}`, want: "remove"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			makeProject(t, root)
			seedFile(t, filepath.Join(root, "export_presets.cfg"), "[preset.0]\n\nname=\"Linux\"\nplatform=\"Linux\"\n")

			result := callToolArgs(t, manageExportPresets(), root, tt.args)
			if result.IsError {
				t.Fatalf("unexpected error: %+v", result.Content)
			}
			structured, ok := result.StructuredContent.(map[string]any)
			if !ok {
				t.Fatalf("structuredContent = %T, want object", result.StructuredContent)
			}
			if structured["action"] != tt.want {
				t.Errorf("action = %v, want %q", structured["action"], tt.want)
			}
		})
	}
}

func TestManageExportPresetsAdd(t *testing.T) {
	root := t.TempDir()
	makeProject(t, root)

	result := callToolArgs(t, manageExportPresets(), root, `{"action":"add","name":"Linux","platform":"Linux","runnable":true}`)
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
	for _, want := range []string{"[preset.0]", `name="Linux"`, `platform="Linux"`, "runnable=true"} {
		if !strings.Contains(string(content), want) {
			t.Errorf("export_presets.cfg = %q, want it to contain %q", content, want)
		}
	}
}

func TestManageExportPresetsAddRunnableDefaultsFalse(t *testing.T) {
	root := t.TempDir()
	makeProject(t, root)

	result := callToolArgs(t, manageExportPresets(), root, `{"action":"add","name":"Web","platform":"Web"}`)
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

func TestManageExportPresetsAddSequentialIndices(t *testing.T) {
	root := t.TempDir()
	makeProject(t, root)

	first := callToolArgs(t, manageExportPresets(), root, `{"action":"add","name":"Linux","platform":"Linux"}`)
	if first.IsError {
		t.Fatalf("unexpected error: %+v", first.Content)
	}
	content, err := os.ReadFile(filepath.Join(root, "export_presets.cfg"))
	if err != nil {
		t.Fatalf("read export_presets.cfg: %v", err)
	}
	if !strings.Contains(string(content), "[preset.0]") {
		t.Errorf("export_presets.cfg = %q, want [preset.0]", content)
	}

	second := callToolArgs(t, manageExportPresets(), root, `{"action":"add","name":"Web","platform":"Web"}`)
	if second.IsError {
		t.Fatalf("unexpected error: %+v", second.Content)
	}
	content, err = os.ReadFile(filepath.Join(root, "export_presets.cfg"))
	if err != nil {
		t.Fatalf("read export_presets.cfg: %v", err)
	}
	if !strings.Contains(string(content), "[preset.1]") {
		t.Errorf("export_presets.cfg = %q, want [preset.1]", content)
	}
}

// TestManageExportPresetsAddAfterGap checks that adding to a file whose
// indices are not contiguous (for example after a remove) still picks a fresh,
// non-colliding index.
func TestManageExportPresetsAddAfterGap(t *testing.T) {
	root := t.TempDir()
	makeProject(t, root)
	seedFile(t, filepath.Join(root, "export_presets.cfg"), "[preset.1]\n\nname=\"Nameless\"\n")

	result := callToolArgs(t, manageExportPresets(), root, `{"action":"add","name":"Web","platform":"Web"}`)
	if result.IsError {
		t.Fatalf("unexpected error: %+v", result.Content)
	}
	content, err := os.ReadFile(filepath.Join(root, "export_presets.cfg"))
	if err != nil {
		t.Fatalf("read export_presets.cfg: %v", err)
	}
	if !strings.Contains(string(content), "[preset.2]") {
		t.Errorf("export_presets.cfg = %q, want [preset.2]", content)
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

	result := callToolArgs(t, manageExportPresets(), root, `{"action":"remove","name":"Linux"}`)
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

	result := callToolArgs(t, manageExportPresets(), root, `{"action":"remove","name":"Linux"}`)
	if !result.IsError {
		t.Fatalf("expected an error result: %+v", result.Content)
	}
	if got, want := result.Content[0].Text, "No export_presets.cfg file found."; got != want {
		t.Errorf("text = %q, want %q", got, want)
	}
}

// TestManageExportPresetsEscaping covers preset names and platforms that need
// escaping and confirms remove matches the encoded name.
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

			result := callToolArgs(t, manageExportPresets(), root, `{"action":"add","name":`+jsonString(tt.preset)+`,"platform":`+jsonString(tt.platform)+`}`)
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

			remove := callToolArgs(t, manageExportPresets(), root, `{"action":"remove","name":`+jsonString(tt.preset)+`}`)
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
	notAProject := t.TempDir()

	tests := []struct {
		name    string
		tool    func() mcp.Tool
		project string
		args    string
		want    string
	}{
		{
			name: "missing params",
			tool: manageExportPresets,
			args: `{}`,
			want: "action is required.",
		},
		{
			name:    "invalid path",
			tool:    manageExportPresets,
			project: "../escape",
			args:    `{"action":"remove","name":"Linux"}`,
			want:    "Invalid path.",
		},
		{
			name:    "not a project",
			tool:    manageExportPresets,
			project: notAProject,
			args:    `{"action":"remove","name":"Linux"}`,
			want:    "Not a valid Godot project: ",
		},
		{
			name:    "add missing platform",
			tool:    manageExportPresets,
			project: projectRoot,
			args:    `{"action":"add","name":"Linux"}`,
			want:    "name and platform are required for add action.",
		},
		{
			name:    "add missing name",
			tool:    manageExportPresets,
			project: projectRoot,
			args:    `{"action":"add","platform":"Linux"}`,
			want:    "name and platform are required for add action.",
		},
		{
			name:    "remove missing name",
			tool:    manageExportPresets,
			project: projectRoot,
			args:    `{"action":"remove"}`,
			want:    "name is required for remove action.",
		},
		{
			name:    "unknown action",
			tool:    manageExportPresets,
			project: projectRoot,
			args:    `{"action":"bogus"}`,
			want:    `Invalid action. Use "add" or "remove".`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := callToolArgs(t, tt.tool(), tt.project, tt.args)
			if !result.IsError {
				t.Fatalf("expected an error result: %+v", result.Content)
			}
			if got := result.Content[0].Text; !strings.HasPrefix(got, tt.want) {
				t.Errorf("message = %q, want prefix %q", got, tt.want)
			}
		})
	}
}
