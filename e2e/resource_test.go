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
// now goes through the live editor session rather than a dedicated tool.
func seedResource(t *testing.T, session *mcp.ClientSession, resourcePath, resourceType string) {
	t.Helper()
	requireScriptSave(t, session, `extends RefCounted

func run():
	var resource := `+resourceType+`.new()
	DirAccess.make_dir_recursive_absolute("res://`+resourceDir(resourcePath)+`")
	return ResourceSaver.save(resource, "res://`+resourcePath+`")
`)
}

func resourceDir(resourcePath string) string {
	dir, _ := filepath.Split(resourcePath)
	return strings.TrimSuffix(dir, "/")
}

// read_binary_resource rejects text .tres, so the fixture must be a binary file.
func seedMaterialRes(t *testing.T, session *mcp.ClientSession, resourcePath string) {
	t.Helper()
	requireScriptSave(t, session, `extends RefCounted

func run():
	var material := StandardMaterial3D.new()
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

		text := requireToolText(t, session, "read_binary_resource", map[string]any{
			"resourcePath": path,
		})
		var payload map[string]any
		if err := json.Unmarshal([]byte(text), &payload); err != nil {
			t.Fatalf("unmarshal resource payload: %v\n%s", err, text)
		}
		if payload["type"] != "StandardMaterial3D" {
			t.Errorf("type = %v, want StandardMaterial3D", payload["type"])
		}
		requireContains(t, text, `"type": "StandardMaterial3D"`)
	})

	t.Run("reads with a property filter", func(t *testing.T) {
		const path = "resources/filter.res"
		seedMaterialRes(t, session, path)

		text := requireToolText(t, session, "read_binary_resource", map[string]any{
			"resourcePath":   path,
			"propertyFilter": []any{"metallic"},
		})
		requireContains(t, text, `"metallic": 0.5`)
		requireNotContains(t, text, `"albedo_color"`)

		text = requireToolText(t, session, "read_binary_resource", map[string]any{
			"resourcePath":   path,
			"propertyFilter": []any{"albedo_*"},
		})
		requireContains(t, text, `"albedo_color"`)
		requireNotContains(t, text, `"metallic"`)
	})

	t.Run("filters with a single-character wildcard", func(t *testing.T) {
		const path = "resources/wildcard.res"
		seedMaterialRes(t, session, path)

		text := requireToolText(t, session, "read_binary_resource", map[string]any{
			"resourcePath":   path,
			"propertyFilter": []any{"albed?_color"},
		})
		requireContains(t, text, `"albedo_color"`)
		requireNotContains(t, text, `"metallic"`)
	})

	t.Run("filter matching nothing yields no properties", func(t *testing.T) {
		const path = "resources/none.res"
		seedMaterialRes(t, session, path)

		text := requireToolText(t, session, "read_binary_resource", map[string]any{
			"resourcePath":   path,
			"propertyFilter": []any{"nonexistent_*"},
		})
		requireContains(t, text, `"properties": {}`)
	})

	t.Run("modifies then re-reads a resource", func(t *testing.T) {
		const path = "resources/modify.res"
		seedMaterialRes(t, session, path)

		requireScriptSave(t, session, `extends RefCounted

func run():
	var material: StandardMaterial3D = load("res://`+path+`")
	material.metallic = 0.75
	return ResourceSaver.save(material, "res://`+path+`")
`)

		readText := requireToolText(t, session, "read_binary_resource", map[string]any{
			"resourcePath": path,
		})
		requireContains(t, readText, `"metallic": 0.75`)
	})

	t.Run("creates then reads a resource", func(t *testing.T) {
		requireScriptSave(t, session, `extends RefCounted

func run():
	var material := StandardMaterial3D.new()
	material.albedo_color = Color(1, 0, 0, 1)
	DirAccess.make_dir_recursive_absolute("res://materials")
	return ResourceSaver.save(material, "res://materials/red.res")
`)

		readText := requireToolText(t, session, "read_binary_resource", map[string]any{
			"resourcePath": "materials/red.res",
		})
		requireContains(t, readText, `"type": "StandardMaterial3D"`, `"path": "materials/red.res"`)
	})

	t.Run("rejects a text .tres", func(t *testing.T) {
		writeE2EFile(t, filepath.Join(root, "resources", "mat.tres"), resourceTres)

		res := callTool(t, session, "read_binary_resource", map[string]any{
			"resourcePath": "resources/mat.tres",
		})
		if !res.IsError {
			t.Fatalf("expected an error for a text .tres: %s", firstText(t, res))
		}
		requireContains(t, firstText(t, res), "read_binary_resource reads binary .res resources")
	})
}

func TestResourceTheme(t *testing.T) {
	t.Parallel()
	_, session := sharedResourceSession(t)

	requireScriptSave(t, session, `extends RefCounted

func run():
	var theme := Theme.new()
	theme.default_font_size = 16
	DirAccess.make_dir_recursive_absolute("res://themes")
	return ResourceSaver.save(theme, "res://themes/main.res")
`)

	readText := requireToolText(t, session, "read_binary_resource", map[string]any{
		"resourcePath": "themes/main.res",
	})
	requireContains(t, readText, `"type": "Theme"`, `"properties"`, `"default_font_size": 16`)
	requireNotContains(t, readText, "THEME_JSON_START")

	requireScriptSave(t, session, `extends RefCounted

func run():
	var theme: Theme = load("res://themes/main.res")
	theme.default_font_size = 20
	return ResourceSaver.save(theme, "res://themes/main.res")
`)

	reReadText := requireToolText(t, session, "read_binary_resource", map[string]any{
		"resourcePath": "themes/main.res",
	})
	requireContains(t, reReadText, `"type": "Theme"`, `"default_font_size": 20`)
}

func TestResourceThemeConvertsTypes(t *testing.T) {
	t.Parallel()
	_, session := sharedResourceSession(t)

	requireScriptSave(t, session, `extends RefCounted

func run():
	DirAccess.make_dir_recursive_absolute("res://themes")
	return ResourceSaver.save(StyleBoxFlat.new(), "res://themes/panel_box.res")
`)
	// run_script does not reconcile the filesystem, so the referenced resource
	// needs a scan to register its UID before the theme can load it.
	requireToolText(t, session, "rescan_project", nil)

	requireScriptSave(t, session, `extends RefCounted

func run():
	var theme := Theme.new()
	theme.set_theme_item(Theme.DATA_TYPE_COLOR, "font_color", "Label", Color(1, 0, 0, 1))
	theme.set_theme_item(Theme.DATA_TYPE_FONT_SIZE, "font_size", "Label", 24)
	theme.set_theme_item(Theme.DATA_TYPE_STYLEBOX, "panel", "Panel", load("res://themes/panel_box.res"))
	return ResourceSaver.save(theme, "res://themes/main.res")
`)

	readText := requireToolText(t, session, "read_binary_resource", map[string]any{
		"resourcePath": "themes/main.res",
	})
	var readPayload struct {
		Properties map[string]any `json:"properties"`
	}
	if err := json.Unmarshal([]byte(readText), &readPayload); err != nil {
		t.Fatalf("unmarshal theme payload: %v\n%s", err, readText)
	}
	fontColor, ok := readPayload.Properties["Label/colors/font_color"].(map[string]any)
	if !ok || fontColor["$type"] != "Color" || fontColor["r"] != float64(1) {
		t.Errorf("font_color = %#v, want a tagged red Color", readPayload.Properties["Label/colors/font_color"])
	}
	if got := readPayload.Properties["Label/font_sizes/font_size"]; got != float64(24) {
		t.Errorf("font_size = %#v, want 24", got)
	}
	panel, ok := readPayload.Properties["Panel/styles/panel"].(map[string]any)
	panelPath, _ := panel["path"].(string)
	if !ok || !strings.Contains(panelPath, "panel_box.res") {
		t.Errorf("panel = %#v, want a reference to panel_box.res", readPayload.Properties["Panel/styles/panel"])
	}

	requireScriptSave(t, session, `extends RefCounted

func run():
	var theme: Theme = load("res://themes/main.res")
	theme.set_theme_item(Theme.DATA_TYPE_COLOR, "font_color", "Label", Color(0, 0, 1, 1))
	return ResourceSaver.save(theme, "res://themes/main.res")
`)

	reReadText := requireToolText(t, session, "read_binary_resource", map[string]any{
		"resourcePath": "themes/main.res",
	})
	var reReadPayload struct {
		Properties map[string]any `json:"properties"`
	}
	if err := json.Unmarshal([]byte(reReadText), &reReadPayload); err != nil {
		t.Fatalf("unmarshal theme payload: %v\n%s", err, reReadText)
	}
	fontColor, ok = reReadPayload.Properties["Label/colors/font_color"].(map[string]any)
	if !ok || fontColor["b"] != float64(1) {
		t.Errorf("font_color = %#v, want a tagged blue Color", reReadPayload.Properties["Label/colors/font_color"])
	}
}

func TestResourceThemeMoreCategories(t *testing.T) {
	t.Parallel()
	_, session := sharedResourceSession(t)

	requireScriptSave(t, session, `extends RefCounted

func run():
	DirAccess.make_dir_recursive_absolute("res://themes")
	return ResourceSaver.save(SystemFont.new(), "res://themes/font.res")
`)
	requireToolText(t, session, "rescan_project", nil)

	requireScriptSave(t, session, `extends RefCounted

func run():
	var theme := Theme.new()
	theme.set_theme_item(Theme.DATA_TYPE_CONSTANT, "outline_size", "Label", 4)
	theme.set_theme_item(Theme.DATA_TYPE_FONT, "font", "Label", load("res://themes/font.res"))
	return ResourceSaver.save(theme, "res://themes/categories.res")
`)

	readText := requireToolText(t, session, "read_binary_resource", map[string]any{
		"resourcePath": "themes/categories.res",
	})
	var payload struct {
		Properties map[string]any `json:"properties"`
	}
	if err := json.Unmarshal([]byte(readText), &payload); err != nil {
		t.Fatalf("unmarshal theme payload: %v\n%s", err, readText)
	}
	if got := payload.Properties["Label/constants/outline_size"]; got != float64(4) {
		t.Errorf("outline_size = %#v, want 4", got)
	}
	font, ok := payload.Properties["Label/fonts/font"].(map[string]any)
	fontPath, _ := font["path"].(string)
	if !ok || !strings.Contains(fontPath, "font.res") {
		t.Errorf("font = %#v, want a reference to font.res", payload.Properties["Label/fonts/font"])
	}
}

func TestResourceErrors(t *testing.T) {
	t.Parallel()
	root, session := sharedResourceSession(t)

	t.Run("not a project", func(t *testing.T) {
		notProject := t.TempDir()
		notProjectSession := connectProject(t, notProject)
		requireToolError(t, notProjectSession, "read_binary_resource", map[string]any{
			"resourcePath": "mat.tres",
		}, "Not a valid Godot project: "+notProject)
	})

	t.Run("resource not found", func(t *testing.T) {
		res := callTool(t, session, "read_binary_resource", map[string]any{
			"resourcePath": "resources/missing.res",
		})
		if !res.IsError {
			t.Fatal("expected an error result")
		}
		requireContains(t, firstText(t, res), "read_resource failed", "Resource not found: res://resources/missing.res")
	})

	t.Run("unloadable resource", func(t *testing.T) {
		writeE2EFile(t, filepath.Join(root, "resources", "bad.res"), "this is not a valid resource\n")

		res := callTool(t, session, "read_binary_resource", map[string]any{
			"resourcePath": "resources/bad.res",
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
