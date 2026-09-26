package e2e

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// resourceTres is a minimal StandardMaterial3D resource so tests that inspect a
// hand-written text resource have one.
const resourceTres = `[gd_resource type="StandardMaterial3D" format=3]

[resource]
metallic = 0.5
`

// seedMaterialRes creates a binary resources/mat.res StandardMaterial3D through
// the engine. read_resource rejects the text .tres form, so a readable fixture
// must be a binary file the engine wrote.
func seedMaterialRes(t *testing.T, session *mcp.ClientSession) {
	t.Helper()
	requireToolText(t, session, "create_resource", map[string]any{
		"resourcePath": "resources/mat.res",
		"resourceType": "StandardMaterial3D",
		"properties": map[string]any{
			"metallic":     0.5,
			"albedo_color": map[string]any{"$type": "Color", "r": 1, "g": 0, "b": 0, "a": 1},
		},
	})
}

// TestCreateResourceSchema ports the tool-definition expectations for
// create_resource over the real stdio surface.
func TestCreateResourceSchema(t *testing.T) {
	schema := requireSchema(t, "create_resource", "Create a .tres/.res resource file")
	schema.requireRequired(t, "resourcePath", "resourceType")
	schema.requireStringProps(t, "resourcePath", "resourceType")
	schema.requireType(t, "properties", "object")
	schema.requirePropCount(t, 3)
}

// TestModifyResourceSchema ports the tool-definition expectations for
// modify_resource over the real stdio surface.
func TestModifyResourceSchema(t *testing.T) {
	schema := requireSchema(t, "modify_resource", "Set properties on a .tres/.res resource file")
	schema.requireRequired(t, "resourcePath", "properties")
	schema.requireStringProps(t, "resourcePath")
	schema.requireType(t, "properties", "object")
	schema.requirePropCount(t, 2)
}

// TestReadResourceSchema ports the read-only resource tool's expectations.
func TestReadResourceSchema(t *testing.T) {
	schema := requireSchema(t, "read_resource", "Read engine-decoded binary .res resource files (text .tres files are read directly)")
	schema.requireRequired(t, "resourcePath")
	schema.requireStringProps(t, "resourcePath")
	schema.requireType(t, "propertyFilter", "array")
	schema.requirePropCount(t, 2)
}

// TestResourceTools drives the real resource tools against Godot.
func TestResourceTools(t *testing.T) {
	t.Run("reads a resource", func(t *testing.T) {
		root := newProject(t)
		session := connectProject(t, root)
		requireGodot(t, session)
		seedMaterialRes(t, session)

		text := requireToolText(t, session, "read_resource", map[string]any{
			"resourcePath": "resources/mat.res",
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
		root := newProject(t)
		session := connectProject(t, root)
		requireGodot(t, session)
		seedMaterialRes(t, session)

		text := requireToolText(t, session, "read_resource", map[string]any{
			"resourcePath":   "resources/mat.res",
			"propertyFilter": []any{"metallic"},
		})
		requireContains(t, text, `"metallic": 0.5`)
		requireNotContains(t, text, `"albedo_color"`)

		text = requireToolText(t, session, "read_resource", map[string]any{
			"resourcePath":   "resources/mat.res",
			"propertyFilter": []any{"albedo_*"},
		})
		requireContains(t, text, `"albedo_color"`)
		requireNotContains(t, text, `"metallic"`)
	})

	t.Run("filters with a single-character wildcard", func(t *testing.T) {
		root := newProject(t)
		session := connectProject(t, root)
		requireGodot(t, session)
		seedMaterialRes(t, session)

		text := requireToolText(t, session, "read_resource", map[string]any{
			"resourcePath":   "resources/mat.res",
			"propertyFilter": []any{"albed?_color"},
		})
		requireContains(t, text, `"albedo_color"`)
		requireNotContains(t, text, `"metallic"`)
	})

	t.Run("filter matching nothing yields no properties", func(t *testing.T) {
		root := newProject(t)
		session := connectProject(t, root)
		requireGodot(t, session)
		seedMaterialRes(t, session)

		text := requireToolText(t, session, "read_resource", map[string]any{
			"resourcePath":   "resources/mat.res",
			"propertyFilter": []any{"nonexistent_*"},
		})
		requireContains(t, text, `"properties": {}`)
	})

	t.Run("modifies then re-reads a resource", func(t *testing.T) {
		root := newProject(t)
		session := connectProject(t, root)
		requireGodot(t, session)
		seedMaterialRes(t, session)

		modifyText := requireToolText(t, session, "modify_resource", map[string]any{
			"resourcePath": "resources/mat.res",
			"properties":   map[string]any{"metallic": 0.75},
		})
		requirePrefix(t, modifyText, "manage_resource succeeded.")

		readText := requireToolText(t, session, "read_resource", map[string]any{
			"resourcePath": "resources/mat.res",
		})
		requireContains(t, readText, `"metallic": 0.75`)
	})

	t.Run("creates then reads a resource", func(t *testing.T) {
		root := newProject(t)
		session := connectProject(t, root)
		requireGodot(t, session)

		createText := requireToolText(t, session, "create_resource", map[string]any{
			"resourcePath": "materials/red.res",
			"resourceType": "StandardMaterial3D",
			"properties": map[string]any{
				"albedo_color": map[string]any{"$type": "Color", "r": 1, "g": 0, "b": 0, "a": 1},
			},
		})
		requirePrefix(t, createText, "manage_resource succeeded.")

		readText := requireToolText(t, session, "read_resource", map[string]any{
			"resourcePath": "materials/red.res",
		})
		requireContains(t, readText, `"type": "StandardMaterial3D"`, `"path": "materials/red.res"`)
	})

	t.Run("rejects a text .tres", func(t *testing.T) {
		root := newProject(t)
		session := connectProject(t, root)
		writeE2EFile(t, filepath.Join(root, "resources", "mat.tres"), resourceTres)

		res := callTool(t, session, "read_resource", map[string]any{
			"resourcePath": "resources/mat.tres",
		})
		if !res.IsError {
			t.Fatalf("expected an error for a text .tres: %s", firstText(t, res))
		}
		requireContains(t, firstText(t, res), "read_resource reads binary .res resources")
	})
}

// TestResourceTheme covers the Theme codec handled by the resource tools:
// create/read/modify with the theme item type inference intact.
func TestResourceTheme(t *testing.T) {
	root := newProject(t)
	session := connectProject(t, root)
	requireGodot(t, session)

	createText := requireToolText(t, session, "create_resource", map[string]any{
		"resourcePath": "themes/main.res",
		"resourceType": "Theme",
		"properties":   map[string]any{"default_font_size": 16},
	})
	requirePrefix(t, createText, "manage_resource succeeded.")

	readText := requireToolText(t, session, "read_resource", map[string]any{
		"resourcePath": "themes/main.res",
	})
	requireContains(t, readText, `"type": "Theme"`, `"properties"`, `"default_font_size": 16`)
	requireNotContains(t, readText, "THEME_JSON_START")

	modifyText := requireToolText(t, session, "modify_resource", map[string]any{
		"resourcePath": "themes/main.res",
		"properties":   map[string]any{"default_font_size": 20},
	})
	requirePrefix(t, modifyText, "manage_resource succeeded.")

	reReadText := requireToolText(t, session, "read_resource", map[string]any{
		"resourcePath": "themes/main.res",
	})
	requireContains(t, reReadText, `"type": "Theme"`, `"default_font_size": 20`)
}

// TestResourceThemeConvertsTypes covers theme properties whose JSON values need
// conversion: a color sent as a component dictionary, a font size, and a Theme
// item that references another resource by path. The saved .tres is inspected so
// a silently-ignored or mis-typed value fails the test.
func TestResourceThemeConvertsTypes(t *testing.T) {
	root := newProject(t)
	session := connectProject(t, root)
	requireGodot(t, session)

	requireToolText(t, session, "create_resource", map[string]any{
		"resourcePath": "themes/panel_box.res",
		"resourceType": "StyleBoxFlat",
	})

	createText := requireToolText(t, session, "create_resource", map[string]any{
		"resourcePath": "themes/main.res",
		"resourceType": "Theme",
		"properties": map[string]any{
			"Label/colors/font_color":    map[string]any{"$type": "Color", "r": 1, "g": 0, "b": 0, "a": 1},
			"Label/font_sizes/font_size": 24,
			"Panel/styles/panel":         "res://themes/panel_box.res",
		},
	})
	requirePrefix(t, createText, "manage_resource succeeded.")

	readText := requireToolText(t, session, "read_resource", map[string]any{
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

	modifyText := requireToolText(t, session, "modify_resource", map[string]any{
		"resourcePath": "themes/main.res",
		"properties": map[string]any{
			"Label/colors/font_color": map[string]any{"$type": "Color", "r": 0, "g": 0, "b": 1, "a": 1},
		},
	})
	requirePrefix(t, modifyText, "manage_resource succeeded.")

	reReadText := requireToolText(t, session, "read_resource", map[string]any{
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

// TestResourceThemeMoreCategories covers the theme item categories whose value
// type is inferred from the item path rather than an existing property:
// constants are integers and fonts are object references.
func TestResourceThemeMoreCategories(t *testing.T) {
	root := newProject(t)
	session := connectProject(t, root)
	requireGodot(t, session)

	requireToolText(t, session, "create_resource", map[string]any{
		"resourcePath": "themes/font.res",
		"resourceType": "SystemFont",
	})

	requireToolText(t, session, "create_resource", map[string]any{
		"resourcePath": "themes/categories.res",
		"resourceType": "Theme",
		"properties": map[string]any{
			"Label/constants/outline_size": 4,
			"Label/fonts/font":             "res://themes/font.res",
		},
	})

	readText := requireToolText(t, session, "read_resource", map[string]any{
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

// TestResourceErrors covers the reference's validation and failure messages.
func TestResourceErrors(t *testing.T) {
	t.Run("missing params", func(t *testing.T) {
		session := connect(t)
		requireToolError(t, session, "create_resource", map[string]any{}, "resourcePath and resourceType are required.")
	})

	t.Run("create without resource type", func(t *testing.T) {
		session := connect(t)
		requireToolError(t, session, "create_resource", map[string]any{
			"resourcePath": "materials/red.tres",
		}, "resourcePath and resourceType are required.")
	})

	t.Run("modify without properties", func(t *testing.T) {
		session := connect(t)
		requireToolError(t, session, "modify_resource", map[string]any{
			"resourcePath": "materials/red.tres",
		}, "resourcePath and properties are required.")
	})

	t.Run("not a project", func(t *testing.T) {
		root := t.TempDir()
		session := connectProject(t, root)
		requireToolError(t, session, "read_resource", map[string]any{
			"resourcePath": "mat.tres",
		}, "Not a valid Godot project: "+root)
	})

	t.Run("unknown resource type", func(t *testing.T) {
		root := newProject(t)
		session := connectProject(t, root)
		requireGodot(t, session)

		res := callTool(t, session, "create_resource", map[string]any{
			"resourcePath": "materials/red.tres",
			"resourceType": "NotARealResource",
		})
		if !res.IsError {
			t.Fatal("expected an error result")
		}
		requireContains(t, firstText(t, res), "Unknown resource type: NotARealResource")
	})

	t.Run("abstract resource type", func(t *testing.T) {
		root := newProject(t)
		session := connectProject(t, root)
		requireGodot(t, session)

		// BaseMaterial3D is a registered Resource subclass that Godot
		// refuses to instantiate directly.
		res := callTool(t, session, "create_resource", map[string]any{
			"resourcePath": "materials/red.tres",
			"resourceType": "BaseMaterial3D",
		})
		if !res.IsError {
			t.Fatal("expected an error result")
		}
		requireContains(t, firstText(t, res), "manage_resource failed", "Cannot instantiate resource type: BaseMaterial3D")
	})

	t.Run("non-resource type", func(t *testing.T) {
		root := newProject(t)
		session := connectProject(t, root)
		requireGodot(t, session)

		// Node is instantiable, so the failure is the Resource-subclass guard.
		res := callTool(t, session, "create_resource", map[string]any{
			"resourcePath": "materials/red.tres",
			"resourceType": "Node",
		})
		if !res.IsError {
			t.Fatal("expected an error result")
		}
		requireContains(t, firstText(t, res), "manage_resource failed", "Type Node is not a Resource subclass")
	})

	t.Run("resource not found", func(t *testing.T) {
		root := newProject(t)
		session := connectProject(t, root)
		requireGodot(t, session)

		res := callTool(t, session, "read_resource", map[string]any{
			"resourcePath": "resources/missing.res",
		})
		if !res.IsError {
			t.Fatal("expected an error result")
		}
		requireContains(t, firstText(t, res), "manage_resource failed", "Resource not found: res://resources/missing.res")
	})

	t.Run("unloadable resource", func(t *testing.T) {
		root := newProject(t)
		session := connectProject(t, root)
		requireGodot(t, session)
		writeE2EFile(t, filepath.Join(root, "resources", "bad.res"), "this is not a valid resource\n")

		res := callTool(t, session, "read_resource", map[string]any{
			"resourcePath": "resources/bad.res",
		})
		if !res.IsError {
			t.Fatal("expected an error result")
		}
		requireContains(t, firstText(t, res), "manage_resource failed", "Failed to load resource: res://resources/bad.res")
	})

	t.Run("invalid property value", func(t *testing.T) {
		root := newProject(t)
		session := connectProject(t, root)
		requireGodot(t, session)

		res := callTool(t, session, "create_resource", map[string]any{
			"resourcePath": "materials/red.tres",
			"resourceType": "StandardMaterial3D",
			"properties":   map[string]any{"metallic": "not-a-number"},
		})
		if !res.IsError {
			t.Fatal("expected an error result")
		}
		requireContains(t, firstText(t, res), "manage_resource failed", "Invalid property value")
	})
}
