package tools

import (
	"strings"
	"testing"

	"github.com/vilppuvuorinen/slopfood-godot-mcp/internal/godot"
	"github.com/vilppuvuorinen/slopfood-godot-mcp/internal/mcp"
)

func TestCreateThemeSchema(t *testing.T) {
	tool := createTheme()
	if tool.Name != "create_theme" {
		t.Errorf("name = %q", tool.Name)
	}
	if tool.Description != "Create a Theme resource" {
		t.Errorf("description = %q", tool.Description)
	}
	if !hasRequired(tool.InputSchema, "resourcePath") {
		t.Errorf("required = %v", tool.InputSchema["required"])
	}
	props, ok := tool.InputSchema["properties"].(map[string]any)
	if !ok {
		t.Fatalf("properties = %T", tool.InputSchema["properties"])
	}
	wantTypes := map[string]string{
		"resourcePath":     "string",
		"defaultFontSize":  "integer",
		"defaultBaseScale": "number",
	}
	for key, want := range wantTypes {
		prop, propOK := props[key].(map[string]any)
		if !propOK || prop["type"] != want {
			t.Errorf("property %q = %v, want %s", key, props[key], want)
		}
	}
	if prop, propOK := props["defaultFont"].(map[string]any); !propOK || prop["anyOf"] == nil {
		t.Errorf("defaultFont = %v, want a value-contract schema", props["defaultFont"])
	}
}

func TestModifyThemeSchema(t *testing.T) {
	tool := modifyTheme()
	if tool.Name != "modify_theme" {
		t.Errorf("name = %q", tool.Name)
	}
	if tool.Description != "Set a Theme's default font, font size, or base scale" {
		t.Errorf("description = %q", tool.Description)
	}
	if !hasRequired(tool.InputSchema, "resourcePath") {
		t.Errorf("required = %v", tool.InputSchema["required"])
	}
	props, ok := tool.InputSchema["properties"].(map[string]any)
	if !ok {
		t.Fatalf("properties = %T", tool.InputSchema["properties"])
	}
	for _, key := range []string{"resourcePath", "defaultFont", "defaultFontSize", "defaultBaseScale"} {
		if _, propOK := props[key].(map[string]any); !propOK {
			t.Errorf("missing property %q", key)
		}
	}
}

func TestReadThemeSchema(t *testing.T) {
	tool := readTheme()
	if tool.Name != "read_theme" {
		t.Errorf("name = %q", tool.Name)
	}
	if tool.Description != "Read Theme resources, their types, variations, and items" {
		t.Errorf("description = %q", tool.Description)
	}
	if !hasRequired(tool.InputSchema, "resourcePath") {
		t.Errorf("required = %v", tool.InputSchema["required"])
	}
}

func TestCreateThemeForwardsDefaults(t *testing.T) {
	t.Setenv(godot.PathEnv, fakeGodotOps(t, `echo "$@"`))
	root := t.TempDir()
	makeProject(t, root)

	result := callToolArgs(t, createTheme(), root, `{"resourcePath":"themes/main.tres","defaultFontSize":16,"defaultBaseScale":1.5,"defaultFont":"themes/font.tres"}`)
	if result.IsError {
		t.Fatalf("unexpected error: %+v", result.Content)
	}
	text := result.Content[0].Text
	for _, want := range []string{`"action":"create"`, `"defaultFontSize":16`, `"defaultBaseScale":1.5`, `"defaultFont":"themes/font.tres"`} {
		if !strings.Contains(text, want) {
			t.Errorf("text = %q, want it to contain %q", text, want)
		}
	}
}

func TestModifyThemeForwardsDefaults(t *testing.T) {
	t.Setenv(godot.PathEnv, fakeGodotOps(t, `echo "$@"`))
	root := t.TempDir()
	makeProject(t, root)

	result := callToolArgs(t, modifyTheme(), root, `{"resourcePath":"themes/main.tres","defaultFontSize":20}`)
	if result.IsError {
		t.Fatalf("unexpected error: %+v", result.Content)
	}
	text := result.Content[0].Text
	for _, want := range []string{`"action":"modify"`, `"defaultFontSize":20`} {
		if !strings.Contains(text, want) {
			t.Errorf("text = %q, want it to contain %q", text, want)
		}
	}
}

func TestReadThemeStructured(t *testing.T) {
	t.Setenv(godot.PathEnv, fakeGodotOps(t, `printf '<<<GODOT_MCP_JSON_START>>>\n{"path":"themes/main.tres","defaultFont":null,"defaultFontSize":16,"defaultBaseScale":null,"types":[]}\n<<<GODOT_MCP_JSON_END>>>\n'`))
	root := t.TempDir()
	makeProject(t, root)

	result := callToolArgs(t, readTheme(), root, `{"resourcePath":"themes/main.tres"}`)
	if result.IsError {
		t.Fatalf("unexpected error: %+v", result.Content)
	}
	text := result.Content[0].Text
	for _, want := range []string{`"path": "themes/main.tres"`, `"defaultFontSize": 16`, `"types": []`} {
		if !strings.Contains(text, want) {
			t.Errorf("text = %q, want it to contain %q", text, want)
		}
	}
	if strings.Contains(text, "succeeded.\n\nOutput:") {
		t.Errorf("text = %q, want the structured payload without the legacy wrapper", text)
	}
}

func TestReadThemeReadSuccessFallback(t *testing.T) {
	t.Setenv(godot.PathEnv, fakeGodotOps(t, `echo "$@"`))
	root := t.TempDir()
	makeProject(t, root)

	result := callToolArgs(t, readTheme(), root, `{"resourcePath":"themes/main.tres"}`)
	if result.IsError {
		t.Fatalf("unexpected error: %+v", result.Content)
	}
	text := result.Content[0].Text
	for _, want := range []string{`"action":"read"`, `"resourcePath":"themes/main.tres"`} {
		if !strings.Contains(text, want) {
			t.Errorf("text = %q, want it to contain %q", text, want)
		}
	}
}

func TestThemeValidation(t *testing.T) {
	root := t.TempDir()
	makeProject(t, root)
	notAProject := t.TempDir()

	tests := []struct {
		name    string
		tool    func() mcp.Tool
		project string
		args    string
		want    string
	}{
		{name: "create missing resource path", tool: createTheme, args: `{}`, want: "resourcePath is required."},
		{name: "modify without defaults", tool: modifyTheme, project: root, args: `{"resourcePath":"themes/main.tres"}`, want: "The modify_theme tool requires at least one of: defaultFont, defaultFontSize, defaultBaseScale."},
		{name: "read missing resource path", tool: readTheme, project: root, args: `{}`, want: "resourcePath is required."},
		{name: "invalid project path", tool: readTheme, project: "../escape", args: `{"resourcePath":"themes/main.tres"}`, want: "Invalid path."},
		{name: "invalid resource path", tool: readTheme, project: root, args: `{"resourcePath":"../escape.tres"}`, want: "Invalid path."},
		{name: "not a project", tool: readTheme, project: notAProject, args: `{"resourcePath":"themes/main.tres"}`, want: "Not a valid Godot project"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := callToolArgs(t, tt.tool(), tt.project, tt.args)
			if !result.IsError {
				t.Fatalf("expected an error result: %+v", result.Content)
			}
			if !strings.HasPrefix(result.Content[0].Text, tt.want) {
				t.Errorf("message = %q, want prefix %q", result.Content[0].Text, tt.want)
			}
		})
	}
}

func TestThemeScriptFailure(t *testing.T) {
	t.Setenv(godot.PathEnv, fakeGodotOps(t, `echo 'Theme resource not found: boom' >&2; exit 1`))
	root := t.TempDir()
	makeProject(t, root)

	result := callToolArgs(t, readTheme(), root, `{"resourcePath":"themes/main.tres"}`)
	if !result.IsError {
		t.Fatalf("expected an error result: %+v", result.Content)
	}
	if !strings.HasPrefix(result.Content[0].Text, "manage_theme failed: ") {
		t.Errorf("message = %q", result.Content[0].Text)
	}
	if len(result.Content) != 2 || !strings.HasPrefix(result.Content[1].Text, "Possible solutions:") {
		t.Errorf("solutions = %+v", result.Content)
	}
}
