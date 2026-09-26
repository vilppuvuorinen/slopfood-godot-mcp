package tools

import (
	"strings"
	"testing"

	"github.com/vilppuvuorinen/go-dot-mcp/internal/godot"
	"github.com/vilppuvuorinen/go-dot-mcp/internal/mcp"
)

func TestManageThemeSchema(t *testing.T) {
	tool := manageTheme()
	if tool.Name != "manage_theme" {
		t.Errorf("name = %q", tool.Name)
	}
	if tool.Description != "Create or modify Theme resources, their types, variations, and items" {
		t.Errorf("description = %q", tool.Description)
	}
	required, ok := tool.InputSchema["required"].([]string)
	if !ok || strings.Join(required, ",") != "resourcePath,action" {
		t.Errorf("required = %v", tool.InputSchema["required"])
	}
	props, ok := tool.InputSchema["properties"].(map[string]any)
	if !ok {
		t.Fatalf("properties = %T", tool.InputSchema["properties"])
	}
	wantTypes := map[string]string{
		"resourcePath":     "string",
		"action":           "string",
		"defaultFontSize":  "integer",
		"defaultBaseScale": "number",
		"themeType":        "string",
		"baseType":         "string",
		"itemType":         "string",
		"itemName":         "string",
	}
	for key, want := range wantTypes {
		prop, propOK := props[key].(map[string]any)
		if !propOK || prop["type"] != want {
			t.Errorf("property %q = %v, want %s", key, props[key], want)
		}
	}
	action, _ := props["action"].(map[string]any)
	enum, _ := action["enum"].([]string)
	if strings.Join(enum, ",") != "create,modify,removeType,setTypeVariation,clearTypeVariation,setItem,clearItem" {
		t.Errorf("action enum = %v", action["enum"])
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

func TestManageThemeCreateForwardsDefaults(t *testing.T) {
	t.Setenv(godot.PathEnv, fakeGodotOps(t, `echo "$@"`))
	root := t.TempDir()
	makeProject(t, root)

	result := callToolArgs(t, manageTheme(), root, `{"resourcePath":"themes/main.tres","action":"create","defaultFontSize":16,"defaultBaseScale":1.5,"defaultFont":"themes/font.tres"}`)
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

func TestManageThemeSetItemForwardsValue(t *testing.T) {
	t.Setenv(godot.PathEnv, fakeGodotOps(t, `echo "$@"`))
	root := t.TempDir()
	makeProject(t, root)

	result := callToolArgs(t, manageTheme(), root, `{"resourcePath":"themes/main.tres","action":"setItem","themeType":"Label","itemType":"color","itemName":"font_color","value":{"$type":"Color","r":1,"g":0,"b":0,"a":1}}`)
	if result.IsError {
		t.Fatalf("unexpected error: %+v", result.Content)
	}
	text := result.Content[0].Text
	for _, want := range []string{`"themeType":"Label"`, `"itemType":"color"`, `"itemName":"font_color"`, `"$type":"Color"`} {
		if !strings.Contains(text, want) {
			t.Errorf("text = %q, want it to contain %q", text, want)
		}
	}
}

func TestReadThemeStructured(t *testing.T) {
	t.Setenv(godot.PathEnv, fakeGodotOps(t, `printf '<<<GO_DOT_MCP_JSON_START>>>\n{"path":"themes/main.tres","defaultFont":null,"defaultFontSize":16,"defaultBaseScale":null,"types":[]}\n<<<GO_DOT_MCP_JSON_END>>>\n'`))
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

func TestManageThemeValidation(t *testing.T) {
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
		{name: "missing params", tool: manageTheme, args: `{}`, want: "resourcePath and action are required."},
		{name: "missing action", tool: manageTheme, project: root, args: `{"resourcePath":"themes/main.tres"}`, want: "resourcePath and action are required."},
		{name: "missing resource path on read", tool: readTheme, project: root, args: `{}`, want: "resourcePath is required."},
		{name: "removeType without theme type", tool: manageTheme, project: root, args: `{"resourcePath":"themes/main.tres","action":"removeType"}`, want: "themeType is required for the removeType action."},
		{name: "setTypeVariation without base type", tool: manageTheme, project: root, args: `{"resourcePath":"themes/main.tres","action":"setTypeVariation","themeType":"Custom"}`, want: "baseType is required for the setTypeVariation action."},
		{name: "setItem without value", tool: manageTheme, project: root, args: `{"resourcePath":"themes/main.tres","action":"setItem","themeType":"Label","itemType":"color","itemName":"font_color"}`, want: "value is required for the setItem action."},
		{name: "clearItem without item name", tool: manageTheme, project: root, args: `{"resourcePath":"themes/main.tres","action":"clearItem","themeType":"Label","itemType":"color"}`, want: "itemName is required for the clearItem action."},
		{name: "unknown action", tool: manageTheme, project: root, args: `{"resourcePath":"themes/main.tres","action":"bogus"}`, want: "Unknown action: bogus. Use create, modify, removeType, setTypeVariation, clearTypeVariation, setItem, or clearItem."},
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

func TestManageThemeScriptFailure(t *testing.T) {
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
