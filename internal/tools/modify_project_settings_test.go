package tools

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vilppuvuorinen/go-dot-mcp/internal/godotconfig"
)

func TestModifyProjectSettingsSchema(t *testing.T) {
	tool := modifyProjectSettings()
	if tool.Name != "modify_project_settings" {
		t.Errorf("name = %q", tool.Name)
	}
	if tool.Description != "Modify a project.godot setting" {
		t.Errorf("description = %q", tool.Description)
	}
	required, ok := tool.InputSchema["required"].([]string)
	if !ok || len(required) != 3 ||
		required[0] != "section" || required[1] != "key" || required[2] != "value" {
		t.Errorf("required = %v", tool.InputSchema["required"])
	}
	props, ok := tool.InputSchema["properties"].(map[string]any)
	if !ok || len(props) != 3 {
		t.Fatalf("properties = %v", tool.InputSchema["properties"])
	}
	for _, key := range []string{"section", "key"} {
		prop, propOK := props[key].(map[string]any)
		if !propOK || prop["type"] != "string" {
			t.Errorf("%s property = %v", key, props[key])
		}
	}
	value, ok := props["value"].(map[string]any)
	if !ok || value["description"] == "" {
		t.Errorf("value property = %v, want a godotValue description", props["value"])
	}
}

func TestModifyProjectSettingsUpdateExistingKey(t *testing.T) {
	root := t.TempDir()
	makeProject(t, root)
	seedFile(t, filepath.Join(root, "project.godot"), `config_version=5

[application]

config/name="Demo"
run/main_scene="res://main.tscn"
`)
	result := callToolArgs(t, modifyProjectSettings(), root, `{"section":"application","key":"config/name","value":"New"}`)
	if result.IsError {
		t.Fatalf("unexpected error: %+v", result.Content)
	}
	if got, want := result.Content[0].Text, `Setting updated: [application] config/name="New"`; got != want {
		t.Errorf("message = %q, want %q", got, want)
	}
	content, err := os.ReadFile(filepath.Join(root, "project.godot"))
	if err != nil {
		t.Fatalf("read project.godot: %v", err)
	}
	if !strings.Contains(string(content), `config/name="New"`) {
		t.Errorf("config/name not updated:\n%s", content)
	}
	if !strings.Contains(string(content), `run/main_scene="res://main.tscn"`) {
		t.Errorf("unrelated key lost:\n%s", content)
	}
}

func TestModifyProjectSettingsAddKeyToExistingSection(t *testing.T) {
	root := t.TempDir()
	makeProject(t, root)
	seedFile(t, filepath.Join(root, "project.godot"), `config_version=5

[application]

config/name="Demo"
`)
	result := callToolArgs(t, modifyProjectSettings(), root, `{"section":"application","key":"config/description","value":"Hi"}`)
	if result.IsError {
		t.Fatalf("unexpected error: %+v", result.Content)
	}
	content, err := os.ReadFile(filepath.Join(root, "project.godot"))
	if err != nil {
		t.Fatalf("read project.godot: %v", err)
	}
	got := string(content)
	if !strings.Contains(got, "\nconfig/description=\"Hi\"\n") {
		t.Errorf("new key not inserted:\n%s", got)
	}
	if !strings.Contains(got, `config/name="Demo"`) {
		t.Errorf("existing key lost:\n%s", got)
	}
}

func TestModifyProjectSettingsAppendNewSection(t *testing.T) {
	root := t.TempDir()
	makeProject(t, root)
	seedFile(t, filepath.Join(root, "project.godot"), `config_version=5

[application]

config/name="Demo"
`)
	result := callToolArgs(t, modifyProjectSettings(), root, `{"section":"display","key":"window/size/viewport_width","value":1280}`)
	if result.IsError {
		t.Fatalf("unexpected error: %+v", result.Content)
	}
	content, err := os.ReadFile(filepath.Join(root, "project.godot"))
	if err != nil {
		t.Fatalf("read project.godot: %v", err)
	}
	got := string(content)
	if !strings.Contains(got, "\n\n[display]\n\nwindow/size/viewport_width=1280\n") {
		t.Errorf("new section not appended:\n%s", got)
	}
}

// TestModifyProjectSettingsValueTypes proves the JSON spelling selects the
// Godot type: a JSON string is a String, an integer is an Int and a decimal is
// a Float. An empty string is now a legitimate empty String, not an error.
func TestModifyProjectSettingsValueTypes(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  string
	}{
		{name: "string", value: `"hello"`, want: `"hello"`},
		{name: "empty string", value: `""`, want: `""`},
		{name: "int", value: `1280`, want: `1280`},
		{name: "float", value: `0.5`, want: `0.5`},
		{name: "bool", value: `true`, want: `true`},
		{name: "null", value: `null`, want: `null`},
		{name: "string that looks numeric", value: `"1280"`, want: `"1280"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			makeProject(t, root)
			seedFile(t, filepath.Join(root, "project.godot"), "config_version=5\n\n[application]\n\nconfig/name=\"Demo\"\n")
			result := callToolArgs(t, modifyProjectSettings(), root, `{"section":"application","key":"test/value","value":`+tt.value+`}`)
			if result.IsError {
				t.Fatalf("unexpected error: %+v", result.Content)
			}
			data, err := os.ReadFile(filepath.Join(root, "project.godot"))
			if err != nil {
				t.Fatalf("read project.godot: %v", err)
			}
			if !strings.Contains(string(data), "test/value="+tt.want) {
				t.Errorf("project.godot = %q, want test/value=%s", data, tt.want)
			}
		})
	}
}

// TestModifyProjectSettingsVectorValue proves an engine-only type is written
// from its tagged form.
func TestModifyProjectSettingsVectorValue(t *testing.T) {
	root := t.TempDir()
	makeProject(t, root)
	seedFile(t, filepath.Join(root, "project.godot"), "config_version=5\n")
	result := callToolArgs(t, modifyProjectSettings(), root, `{"section":"display","key":"window/size/test","value":{"$type":"Vector2i","x":1,"y":2}}`)
	if result.IsError {
		t.Fatalf("unexpected error: %+v", result.Content)
	}
	content, err := os.ReadFile(filepath.Join(root, "project.godot"))
	if err != nil {
		t.Fatalf("read project.godot: %v", err)
	}
	cfg, err := godotconfig.Parse(string(content))
	if err != nil {
		t.Fatalf("written file did not parse: %v", err)
	}
	if v, ok := cfg.Get("display", "window/size/test"); !ok || godotconfig.FormatValue(v) != "Vector2i(1, 2)" {
		t.Errorf("window/size/test = %#v (%v)", v, ok)
	}
}

// TestModifyProjectSettingsUnknownTagRejected proves an unrecognized $type is
// reported as an invalid value and the file is left untouched.
func TestModifyProjectSettingsUnknownTagRejected(t *testing.T) {
	root := t.TempDir()
	makeProject(t, root)
	seedFile(t, filepath.Join(root, "project.godot"), "config_version=5\n\n[application]\n\nconfig/name=\"Demo\"\n")
	before, err := os.ReadFile(filepath.Join(root, "project.godot"))
	if err != nil {
		t.Fatalf("read project.godot: %v", err)
	}
	result := callToolArgs(t, modifyProjectSettings(), root, `{"section":"application","key":"config/name","value":{"$type":"Vectr2","x":1,"y":2}}`)
	if !result.IsError {
		t.Fatalf("expected an error for an unknown $type, got: %+v", result.Content)
	}
	if !strings.HasPrefix(result.Content[0].Text, "Invalid value:") {
		t.Errorf("message = %q, want an 'Invalid value:' prefix", result.Content[0].Text)
	}
	after, err := os.ReadFile(filepath.Join(root, "project.godot"))
	if err != nil {
		t.Fatalf("read project.godot: %v", err)
	}
	if string(after) != string(before) {
		t.Errorf("project.godot was modified by a rejected value:\n%s", after)
	}
}

// TestModifyProjectSettingsEscaping covers encoded section and key names, and
// replacing an existing value that itself contains an escaped quote.
func TestModifyProjectSettingsEscaping(t *testing.T) {
	t.Run("encoded section and key", func(t *testing.T) {
		root := t.TempDir()
		makeProject(t, root)
		seedFile(t, filepath.Join(root, "project.godot"), "config_version=5\n")

		result := callToolArgs(t, modifyProjectSettings(), root, `{"section":`+jsonString("weird]section")+`,"key":`+jsonString("a\nb")+`,"value":1280}`)
		if result.IsError {
			t.Fatalf("unexpected error: %+v", result.Content)
		}
		data, err := os.ReadFile(filepath.Join(root, "project.godot"))
		if err != nil {
			t.Fatalf("read project.godot: %v", err)
		}
		content := string(data)
		if !strings.Contains(content, "[weird\\]section]") {
			t.Errorf("project.godot = %q, want the escaped section header", content)
		}
		if !strings.Contains(content, "\"a\nb\"=1280") {
			t.Errorf("project.godot = %q, want the quoted multi-line key", content)
		}
	})

	t.Run("replaces escaped existing value", func(t *testing.T) {
		root := t.TempDir()
		makeProject(t, root)
		seedFile(t, filepath.Join(root, "project.godot"), "config_version=5\n\n[application]\n\nconfig/description=\"say \\\"hi\\\"\"\nconfig/name=\"Demo\"\n")

		result := callToolArgs(t, modifyProjectSettings(), root, `{"section":"application","key":"config/description","value":"bye"}`)
		if result.IsError {
			t.Fatalf("unexpected error: %+v", result.Content)
		}
		data, err := os.ReadFile(filepath.Join(root, "project.godot"))
		if err != nil {
			t.Fatalf("read project.godot: %v", err)
		}
		content := string(data)
		if strings.Contains(content, "say") {
			t.Errorf("old escaped value still present:\n%s", content)
		}
		if !strings.Contains(content, `config/description="bye"`) {
			t.Errorf("project.godot = %q", content)
		}
		if !strings.Contains(content, `config/name="Demo"`) {
			t.Errorf("unrelated key lost:\n%s", content)
		}
	})
}

// TestModifyProjectSettingsStringCannotInject proves a string value containing
// a newline and a section header is written as a quoted string, so it cannot
// smuggle extra keys or sections into the file.
func TestModifyProjectSettingsStringCannotInject(t *testing.T) {
	root := t.TempDir()
	makeProject(t, root)
	seedFile(t, filepath.Join(root, "project.godot"), `config_version=5

[application]

config/name="Demo"
`)

	injected := "1\n[input]\nmove_left=1"
	result := callToolArgs(t, modifyProjectSettings(), root, `{"section":"application","key":"config/name","value":`+jsonString(injected)+`}`)
	if result.IsError {
		t.Fatalf("unexpected error: %+v", result.Content)
	}

	content, err := os.ReadFile(filepath.Join(root, "project.godot"))
	if err != nil {
		t.Fatalf("read project.godot: %v", err)
	}
	cfg, err := godotconfig.Parse(string(content))
	if err != nil {
		t.Fatalf("written file did not parse: %v", err)
	}
	if _, ok := cfg.Get("input", "move_left"); ok {
		t.Errorf("injected [input] section was created:\n%s", content)
	}
	value, ok := cfg.Get("application", "config/name")
	if !ok || value != godotconfig.String(injected) {
		t.Errorf("config/name = %#v (%v), want the whole injected string", value, ok)
	}

	result = callToolArgs(t, modifyProjectSettings(), root, `{"section":"application","key":"config/name","value":`+jsonString(`say "hi"`)+`}`)
	if result.IsError {
		t.Fatalf("unexpected error: %+v", result.Content)
	}
	content, err = os.ReadFile(filepath.Join(root, "project.godot"))
	if err != nil {
		t.Fatalf("read project.godot: %v", err)
	}
	if !strings.Contains(string(content), `config/name="say \"hi\""`) {
		t.Errorf("escaped value not written:\n%s", content)
	}
}

// TestModifyProjectSettingsSectionInjection proves the section name is
// validated: a line break is rejected (the server's line-based scanner cannot
// represent a multi-line header) and a trailing backslash is rejected (Godot's
// ConfigFile format cannot round-trip it, and Godot itself writes an
// unreadable file). A rejected section must leave the file untouched.
func TestModifyProjectSettingsSectionInjection(t *testing.T) {
	cases := []struct{ name, section string }{
		{name: "newline", section: "x]\n[application"},
		{name: "trailing backslash", section: `trail\`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			makeProject(t, root)
			seedFile(t, filepath.Join(root, "project.godot"), "config_version=5\n\n[application]\n\nconfig/name=\"Demo\"\n")
			before, err := os.ReadFile(filepath.Join(root, "project.godot"))
			if err != nil {
				t.Fatalf("read project.godot: %v", err)
			}
			result := callToolArgs(t, modifyProjectSettings(), root, `{"section":`+jsonString(tc.section)+`,"key":"k","value":1}`)
			if !result.IsError {
				t.Fatalf("expected an error for section %q, got: %+v", tc.section, result.Content)
			}
			if !strings.HasPrefix(result.Content[0].Text, "Invalid section:") {
				t.Errorf("message = %q, want an 'Invalid section:' prefix", result.Content[0].Text)
			}
			after, err := os.ReadFile(filepath.Join(root, "project.godot"))
			if err != nil {
				t.Fatalf("read project.godot: %v", err)
			}
			if string(after) != string(before) {
				t.Errorf("project.godot was modified by a rejected section:\n%s", after)
			}
		})
	}
}

// TestModifyProjectSettingsSectionWithBackslash proves an embedded backslash
// (not adjacent to the closing bracket) is literal in Godot's format and
// round-trips through the written file.
func TestModifyProjectSettingsSectionWithBackslash(t *testing.T) {
	root := t.TempDir()
	makeProject(t, root)
	seedFile(t, filepath.Join(root, "project.godot"), "config_version=5\n")
	result := callToolArgs(t, modifyProjectSettings(), root, `{"section":`+jsonString(`back\mid`)+`,"key":"k","value":1}`)
	if result.IsError {
		t.Fatalf("unexpected error: %+v", result.Content)
	}
	data, err := os.ReadFile(filepath.Join(root, "project.godot"))
	if err != nil {
		t.Fatalf("read project.godot: %v", err)
	}
	content := string(data)
	if !strings.Contains(content, `[back\mid]`) {
		t.Errorf("project.godot = %q, want the literal backslash section", content)
	}
	cfg, err := godotconfig.Parse(content)
	if err != nil {
		t.Fatalf("written file did not parse: %v", err)
	}
	if v, ok := cfg.Get(`back\mid`, "k"); !ok || v != godotconfig.Int(1) {
		t.Errorf(`back\mid/k = %#v, %v`, v, ok)
	}
}

func TestModifyProjectSettingsErrors(t *testing.T) {
	valid := t.TempDir()
	makeProject(t, valid)
	notAProject := t.TempDir()

	tests := []struct {
		name    string
		project string
		args    string
		want    string
	}{
		{name: "missing args", args: `{}`, want: "section, key, and value are required."},
		{name: "missing value", project: valid, args: `{"section":"application","key":"config/name"}`, want: "section, key, and value are required."},
		{name: "invalid path", project: "../escape", args: `{"section":"application","key":"config/name","value":"x"}`, want: "Invalid path."},
		{name: "not a project", project: notAProject, args: `{"section":"application","key":"config/name","value":"x"}`, want: "Not a valid Godot project"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := callToolArgs(t, modifyProjectSettings(), tt.project, tt.args)
			if !result.IsError {
				t.Fatalf("expected an error result: %+v", result.Content)
			}
			if got := result.Content[0].Text; !strings.HasPrefix(got, tt.want) {
				t.Errorf("message = %q, want prefix %q", got, tt.want)
			}
		})
	}
}
