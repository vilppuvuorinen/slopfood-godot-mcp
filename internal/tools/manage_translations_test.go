package tools

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestManageTranslationsSchema(t *testing.T) {
	tool := manageTranslations()
	if tool.Name != "manage_translations" {
		t.Errorf("name = %q", tool.Name)
	}
	if tool.Description != "List/add/remove translation files in project" {
		t.Errorf("description = %q", tool.Description)
	}
	required, ok := tool.InputSchema["required"].([]string)
	if !ok || len(required) != 2 || required[0] != "projectPath" || required[1] != "action" {
		t.Errorf("required = %v", tool.InputSchema["required"])
	}
	props, ok := tool.InputSchema["properties"].(map[string]any)
	if !ok || len(props) != 3 {
		t.Fatalf("properties = %v", tool.InputSchema["properties"])
	}
	for _, key := range []string{"projectPath", "action", "translationPath"} {
		prop, ok := props[key].(map[string]any)
		if !ok || prop["type"] != "string" {
			t.Errorf("%s property = %v", key, props[key])
		}
	}
}

// translationsPayload is the list action's contract payload: the
// internationalization/translations setting as a tagged PackedStringArray.
type translationsPayload struct {
	Translations struct {
		Type   string   `json:"$type"`
		Values []string `json:"values"`
	} `json:"translations"`
}

func TestManageTranslationsList(t *testing.T) {
	root := t.TempDir()
	makeProject(t, root)
	seedFile(t, filepath.Join(root, "project.godot"), `config_version=5

[internationalization]

translations=PackedStringArray("res://en.translation", "res://fi.translation")
`)
	result := callToolArgs(t, manageTranslations(), `{"projectPath":`+jsonString(root)+`,"action":"list"}`)
	if result.IsError {
		t.Fatalf("unexpected error: %+v", result.Content)
	}
	var payload translationsPayload
	if err := json.Unmarshal([]byte(result.Content[0].Text), &payload); err != nil {
		t.Fatalf("unmarshal %q: %v", result.Content[0].Text, err)
	}
	if payload.Translations.Type != "PackedStringArray" {
		t.Errorf("$type = %q, want PackedStringArray", payload.Translations.Type)
	}
	want := []string{"res://en.translation", "res://fi.translation"}
	if strings.Join(payload.Translations.Values, ",") != strings.Join(want, ",") {
		t.Errorf("translations = %v, want %v", payload.Translations.Values, want)
	}
}

func TestManageTranslationsListEmpty(t *testing.T) {
	root := t.TempDir()
	makeProject(t, root)
	result := callToolArgs(t, manageTranslations(), `{"projectPath":`+jsonString(root)+`,"action":"list"}`)
	if result.IsError {
		t.Fatalf("unexpected error: %+v", result.Content)
	}
	got := result.Content[0].Text
	if !strings.Contains(got, `"$type": "PackedStringArray"`) || !strings.Contains(got, `"values": []`) {
		t.Errorf("text = %q, want an empty PackedStringArray", got)
	}
}

func TestManageTranslationsAddExisting(t *testing.T) {
	root := t.TempDir()
	makeProject(t, root)
	seedFile(t, filepath.Join(root, "project.godot"), `config_version=5

[internationalization]

translations=PackedStringArray("res://en.translation")
`)
	result := callToolArgs(t, manageTranslations(), `{"projectPath":`+jsonString(root)+`,"action":"add","translationPath":"fi.translation"}`)
	if result.IsError {
		t.Fatalf("unexpected error: %+v", result.Content)
	}
	if got, want := result.Content[0].Text, "Translation added: res://fi.translation"; got != want {
		t.Errorf("message = %q, want %q", got, want)
	}
	content, err := os.ReadFile(filepath.Join(root, "project.godot"))
	if err != nil {
		t.Fatalf("read project.godot: %v", err)
	}
	if want := `translations=PackedStringArray("res://en.translation", "res://fi.translation")`; !strings.Contains(string(content), want) {
		t.Errorf("translations not updated:\n%s", content)
	}
}

func TestManageTranslationsAddCreatesSection(t *testing.T) {
	root := t.TempDir()
	makeProject(t, root)
	seedFile(t, filepath.Join(root, "project.godot"), "config_version=5\n")
	result := callToolArgs(t, manageTranslations(), `{"projectPath":`+jsonString(root)+`,"action":"add","translationPath":"res://en.translation"}`)
	if result.IsError {
		t.Fatalf("unexpected error: %+v", result.Content)
	}
	if got, want := result.Content[0].Text, "Translation added: res://en.translation"; got != want {
		t.Errorf("message = %q, want %q", got, want)
	}
	content, err := os.ReadFile(filepath.Join(root, "project.godot"))
	if err != nil {
		t.Fatalf("read project.godot: %v", err)
	}
	got := string(content)
	if !strings.Contains(got, "[internationalization]\n\ntranslations=PackedStringArray(\"res://en.translation\")") {
		t.Errorf("section not created:\n%s", got)
	}
}

func TestManageTranslationsRemove(t *testing.T) {
	root := t.TempDir()
	makeProject(t, root)
	seedFile(t, filepath.Join(root, "project.godot"), `config_version=5

[internationalization]

translations=PackedStringArray("res://en.translation", "res://fi.translation")
`)
	result := callToolArgs(t, manageTranslations(), `{"projectPath":`+jsonString(root)+`,"action":"remove","translationPath":"en.translation"}`)
	if result.IsError {
		t.Fatalf("unexpected error: %+v", result.Content)
	}
	if got, want := result.Content[0].Text, "Translation removed: res://en.translation"; got != want {
		t.Errorf("message = %q, want %q", got, want)
	}
	content, err := os.ReadFile(filepath.Join(root, "project.godot"))
	if err != nil {
		t.Fatalf("read project.godot: %v", err)
	}
	got := string(content)
	if strings.Contains(got, "res://en.translation") {
		t.Errorf("entry not removed:\n%s", got)
	}
	if !strings.Contains(got, `"res://fi.translation"`) {
		t.Errorf("other entry lost:\n%s", got)
	}
}

// TestManageTranslationsEscaping covers paths that need escaping and checks
// the typed PackedStringArray round-trips without compounding escapes.
func TestManageTranslationsEscaping(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		wantLine string
	}{
		{name: "quote in path", path: `evil"name.translation`, wantLine: `"res://evil\"name.translation"`},
		{name: "newline in path", path: "evil\nname.translation", wantLine: `"res://evil\nname.translation"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			makeProject(t, root)
			seedFile(t, filepath.Join(root, "project.godot"), `config_version=5

[internationalization]

translations=PackedStringArray("res://en.translation")
`)
			result := callToolArgs(t, manageTranslations(),
				`{"projectPath":`+jsonString(root)+`,"action":"add","translationPath":`+jsonString(tt.path)+`}`)
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

			list := callToolArgs(t, manageTranslations(), `{"projectPath":`+jsonString(root)+`,"action":"list"}`)
			var payload translationsPayload
			if err := json.Unmarshal([]byte(list.Content[0].Text), &payload); err != nil {
				t.Fatalf("unmarshal %q: %v", list.Content[0].Text, err)
			}
			want := []string{"res://en.translation", "res://" + tt.path}
			if strings.Join(payload.Translations.Values, ",") != strings.Join(want, ",") {
				t.Errorf("translations = %q, want %q", payload.Translations.Values, want)
			}
		})
	}
}

func TestManageTranslationsErrors(t *testing.T) {
	valid := t.TempDir()
	makeProject(t, valid)

	tests := []struct {
		name string
		args string
		want string
	}{
		{name: "missing args", args: `{}`, want: "projectPath and action are required."},
		{name: "missing action", args: `{"projectPath":` + jsonString(valid) + `}`, want: "projectPath and action are required."},
		{name: "missing translationPath", args: `{"projectPath":` + jsonString(valid) + `,"action":"add"}`, want: "translationPath is required."},
		{name: "unknown action", args: `{"projectPath":` + jsonString(valid) + `,"action":"bogus"}`, want: "Unknown action: bogus"},
		{name: "invalid path", args: `{"projectPath":"../escape","action":"list"}`, want: "Invalid path."},
		{name: "not a project", args: `{"projectPath":` + jsonString(t.TempDir()) + `,"action":"list"}`, want: "Not a valid Godot project"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := callToolArgs(t, manageTranslations(), tt.args)
			if !result.IsError {
				t.Fatalf("expected an error result: %+v", result.Content)
			}
			if got := result.Content[0].Text; !strings.HasPrefix(got, tt.want) {
				t.Errorf("message = %q, want prefix %q", got, tt.want)
			}
		})
	}
}
