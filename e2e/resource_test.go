package e2e

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const resourceTres = `[gd_resource type="StandardMaterial3D" format=3]

[resource]
metallic = 0.5
`

// seedResource creates a resource through run_script, since resource creation
// goes through run_script rather than a dedicated tool.
func seedResource(t *testing.T, session *mcp.ClientSession, resourcePath, resourceType string) {
	t.Helper()
	requireScriptSave(t, session, `var resource := `+resourceType+`.new()
DirAccess.make_dir_recursive_absolute("res://`+resourceDir(resourcePath)+`")
return ResourceSaver.save(resource, "res://`+resourcePath+`")
`)
}

func resourceDir(resourcePath string) string {
	dir, _ := filepath.Split(resourcePath)
	return strings.TrimSuffix(dir, "/")
}

type resourceData struct {
	Class      string         `json:"class"`
	Properties map[string]any `json:"properties"`
}

// read(type=resource) rejects text .tres, so the fixture must be a binary file.
func seedMaterialRes(t *testing.T, session *mcp.ClientSession, resourcePath string) {
	t.Helper()
	requireScriptSave(t, session, `var material := StandardMaterial3D.new()
material.metallic = 0.5
material.albedo_color = Color(1, 0, 0, 1)
DirAccess.make_dir_recursive_absolute("res://`+resourceDir(resourcePath)+`")
return ResourceSaver.save(material, "res://`+resourcePath+`")
`)
}

func TestResourceTools(t *testing.T) {
	t.Parallel()
	root := newProject(t)
	session := connectProject(t, root)

	t.Run("reads a resource", func(t *testing.T) {
		const path = "resources/read.res"
		seedMaterialRes(t, session, path)

		text := requireToolText(t, session, "read", map[string]any{
			"type": "resource",
			"path": path,
		})
		var env readEnvelope[resourceData]
		if err := json.Unmarshal([]byte(text), &env); err != nil {
			t.Fatalf("unmarshal resource payload: %v\n%s", err, text)
		}
		if env.Kind != "resource" || env.Data.Class != "StandardMaterial3D" {
			t.Errorf("envelope = %+v, want kind resource with class StandardMaterial3D", env)
		}
		requireContains(t, text, `"kind": "resource"`, `"class": "StandardMaterial3D"`)
	})

	t.Run("reads with a property filter", func(t *testing.T) {
		const path = "resources/filter.res"
		seedMaterialRes(t, session, path)

		text := requireToolText(t, session, "read", map[string]any{
			"type":           "resource",
			"path":           path,
			"propertyFilter": []any{"metallic"},
		})
		requireContains(t, text, `"metallic": 0.5`)
		requireNotContains(t, text, `"albedo_color"`)

		text = requireToolText(t, session, "read", map[string]any{
			"type":           "resource",
			"path":           path,
			"propertyFilter": []any{"albedo_*"},
		})
		requireContains(t, text, `"albedo_color"`)
		requireNotContains(t, text, `"metallic"`)
	})

	t.Run("filters with a single-character wildcard", func(t *testing.T) {
		const path = "resources/wildcard.res"
		seedMaterialRes(t, session, path)

		text := requireToolText(t, session, "read", map[string]any{
			"type":           "resource",
			"path":           path,
			"propertyFilter": []any{"albed?_color"},
		})
		requireContains(t, text, `"albedo_color"`)
		requireNotContains(t, text, `"metallic"`)
	})

	t.Run("filter matching nothing yields no properties", func(t *testing.T) {
		const path = "resources/none.res"
		seedMaterialRes(t, session, path)

		text := requireToolText(t, session, "read", map[string]any{
			"type":           "resource",
			"path":           path,
			"propertyFilter": []any{"nonexistent_*"},
		})
		requireContains(t, text, `"properties": {}`)
	})

	t.Run("modifies then re-reads a resource", func(t *testing.T) {
		const path = "resources/modify.res"
		seedMaterialRes(t, session, path)

		requireScriptSave(t, session, `var material: StandardMaterial3D = load("res://`+path+`")
material.metallic = 0.75
return ResourceSaver.save(material, "res://`+path+`")
`)

		readText := requireToolText(t, session, "read", map[string]any{
			"type": "resource",
			"path": path,
		})
		requireContains(t, readText, `"metallic": 0.75`)
	})

	t.Run("creates then reads a resource", func(t *testing.T) {
		requireScriptSave(t, session, `var material := StandardMaterial3D.new()
material.albedo_color = Color(1, 0, 0, 1)
DirAccess.make_dir_recursive_absolute("res://materials")
return ResourceSaver.save(material, "res://materials/red.res")
`)

		readText := requireToolText(t, session, "read", map[string]any{
			"type": "resource",
			"path": "materials/red.res",
		})
		requireContains(t, readText, `"class": "StandardMaterial3D"`, `"path": "materials/red.res"`)
	})

	t.Run("rejects a text .tres", func(t *testing.T) {
		writeE2EFile(t, filepath.Join(root, "resources", "mat.tres"), resourceTres)

		res := callTool(t, session, "read", map[string]any{
			"type": "resource",
			"path": "resources/mat.tres",
		})
		if !res.IsError {
			t.Fatalf("expected an error for a text .tres: %s", firstText(t, res))
		}
		requireContains(t, firstText(t, res), "read(type=resource) reads binary .res resources")
	})
}

func TestResourceTheme(t *testing.T) {
	t.Parallel()
	_, session := sharedResourceSession(t)

	requireScriptSave(t, session, `var theme := Theme.new()
theme.default_font_size = 16
DirAccess.make_dir_recursive_absolute("res://themes")
return ResourceSaver.save(theme, "res://themes/main.res")
`)

	readText := requireToolText(t, session, "read", map[string]any{
		"type": "resource",
		"path": "themes/main.res",
	})
	requireContains(t, readText, `"class": "Theme"`, `"properties"`, `"default_font_size": 16`)
	requireNotContains(t, readText, "THEME_JSON_START")

	requireScriptSave(t, session, `var theme: Theme = load("res://themes/main.res")
theme.default_font_size = 20
return ResourceSaver.save(theme, "res://themes/main.res")
`)

	reReadText := requireToolText(t, session, "read", map[string]any{
		"type": "resource",
		"path": "themes/main.res",
	})
	requireContains(t, reReadText, `"class": "Theme"`, `"default_font_size": 20`)
}

func TestResourceThemeConvertsTypes(t *testing.T) {
	t.Parallel()
	_, session := sharedResourceSession(t)

	requireScriptSave(t, session, `DirAccess.make_dir_recursive_absolute("res://themes")
return ResourceSaver.save(StyleBoxFlat.new(), "res://themes/panel_box.res")
`)

	requireScriptSave(t, session, `var theme := Theme.new()
theme.set_theme_item(Theme.DATA_TYPE_COLOR, "font_color", "Label", Color(1, 0, 0, 1))
theme.set_theme_item(Theme.DATA_TYPE_FONT_SIZE, "font_size", "Label", 24)
theme.set_theme_item(Theme.DATA_TYPE_STYLEBOX, "panel", "Panel", load("res://themes/panel_box.res"))
return ResourceSaver.save(theme, "res://themes/main.res")
`)

	readText := requireToolText(t, session, "read", map[string]any{
		"type": "resource",
		"path": "themes/main.res",
	})
	var readEnvelopeValue readEnvelope[resourceData]
	if err := json.Unmarshal([]byte(readText), &readEnvelopeValue); err != nil {
		t.Fatalf("unmarshal theme payload: %v\n%s", err, readText)
	}
	fontColor, ok := readEnvelopeValue.Data.Properties["Label/colors/font_color"].(map[string]any)
	if !ok || fontColor["$type"] != "Color" || fontColor["r"] != float64(1) {
		t.Errorf("font_color = %#v, want a tagged red Color", readEnvelopeValue.Data.Properties["Label/colors/font_color"])
	}
	if got := readEnvelopeValue.Data.Properties["Label/font_sizes/font_size"]; got != float64(24) {
		t.Errorf("font_size = %#v, want 24", got)
	}
	panel, ok := readEnvelopeValue.Data.Properties["Panel/styles/panel"].(map[string]any)
	panelPath, _ := panel["path"].(string)
	if !ok || !strings.Contains(panelPath, "panel_box.res") {
		t.Errorf("panel = %#v, want a reference to panel_box.res", readEnvelopeValue.Data.Properties["Panel/styles/panel"])
	}

	requireScriptSave(t, session, `var theme: Theme = load("res://themes/main.res")
theme.set_theme_item(Theme.DATA_TYPE_COLOR, "font_color", "Label", Color(0, 0, 1, 1))
return ResourceSaver.save(theme, "res://themes/main.res")
`)

	reReadText := requireToolText(t, session, "read", map[string]any{
		"type": "resource",
		"path": "themes/main.res",
	})
	var reReadEnvelope readEnvelope[resourceData]
	if err := json.Unmarshal([]byte(reReadText), &reReadEnvelope); err != nil {
		t.Fatalf("unmarshal theme payload: %v\n%s", err, reReadText)
	}
	fontColor, ok = reReadEnvelope.Data.Properties["Label/colors/font_color"].(map[string]any)
	if !ok || fontColor["b"] != float64(1) {
		t.Errorf("font_color = %#v, want a tagged blue Color", reReadEnvelope.Data.Properties["Label/colors/font_color"])
	}
}

func TestResourceThemeMoreCategories(t *testing.T) {
	t.Parallel()
	_, session := sharedResourceSession(t)

	requireScriptSave(t, session, `DirAccess.make_dir_recursive_absolute("res://themes")
return ResourceSaver.save(SystemFont.new(), "res://themes/font.res")
`)

	requireScriptSave(t, session, `var theme := Theme.new()
theme.set_theme_item(Theme.DATA_TYPE_CONSTANT, "outline_size", "Label", 4)
theme.set_theme_item(Theme.DATA_TYPE_FONT, "font", "Label", load("res://themes/font.res"))
return ResourceSaver.save(theme, "res://themes/categories.res")
`)

	readText := requireToolText(t, session, "read", map[string]any{
		"type": "resource",
		"path": "themes/categories.res",
	})
	var env readEnvelope[resourceData]
	if err := json.Unmarshal([]byte(readText), &env); err != nil {
		t.Fatalf("unmarshal theme payload: %v\n%s", err, readText)
	}
	if got := env.Data.Properties["Label/constants/outline_size"]; got != float64(4) {
		t.Errorf("outline_size = %#v, want 4", got)
	}
	font, ok := env.Data.Properties["Label/fonts/font"].(map[string]any)
	fontPath, _ := font["path"].(string)
	if !ok || !strings.Contains(fontPath, "font.res") {
		t.Errorf("font = %#v, want a reference to font.res", env.Data.Properties["Label/fonts/font"])
	}
}

func TestResourceErrors(t *testing.T) {
	t.Parallel()
	root, session := sharedResourceSession(t)

	t.Run("not a project", func(t *testing.T) {
		notProject := t.TempDir()
		notProjectSession := connectProject(t, notProject)
		requireToolError(t, notProjectSession, "read", map[string]any{
			"type": "resource",
			"path": "mat.tres",
		}, "Not a valid Godot project: "+notProject)
	})

	t.Run("resource not found", func(t *testing.T) {
		res := callTool(t, session, "read", map[string]any{
			"type": "resource",
			"path": "resources/missing.res",
		})
		if !res.IsError {
			t.Fatal("expected an error result")
		}
		requireContains(t, firstText(t, res), "read_resource failed", "Resource not found: res://resources/missing.res")
	})

	t.Run("unloadable resource", func(t *testing.T) {
		writeE2EFile(t, filepath.Join(root, "resources", "bad.res"), "this is not a valid resource\n")

		res := callTool(t, session, "read", map[string]any{
			"type": "resource",
			"path": "resources/bad.res",
		})
		if !res.IsError {
			t.Fatal("expected an error result")
		}
		requireContains(t, firstText(t, res), "read_resource failed", "Failed to load resource: res://resources/bad.res")
	})
}

// sharedResourceSession returns a fresh project and a session bound to it.
func sharedResourceSession(t *testing.T) (string, *mcp.ClientSession) {
	t.Helper()
	root := newProject(t)
	return root, connectProject(t, root)
}
