package e2e

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// TestCreateThemeSchema pins the create tool's input surface.
func TestCreateThemeSchema(t *testing.T) {
	schema := requireSchema(t, "create_theme", "Create a Theme resource")
	schema.requireRequired(t, "resourcePath")
	schema.requireStringProps(t, "resourcePath")
	schema.requireType(t, "defaultFontSize", "integer")
	schema.requireType(t, "defaultBaseScale", "number")
	schema.requirePropCount(t, 4)
}

// TestModifyThemeSchema pins the modify tool's input surface.
func TestModifyThemeSchema(t *testing.T) {
	schema := requireSchema(t, "modify_theme", "Set a Theme's default font, font size, or base scale")
	schema.requireRequired(t, "resourcePath")
	schema.requireStringProps(t, "resourcePath")
	schema.requireType(t, "defaultFontSize", "integer")
	schema.requireType(t, "defaultBaseScale", "number")
	schema.requirePropCount(t, 4)
}

// TestRemoveThemeTypeSchema pins the remove-type tool's input surface.
func TestRemoveThemeTypeSchema(t *testing.T) {
	schema := requireSchema(t, "remove_theme_type", "Remove a type from a Theme")
	schema.requireRequired(t, "resourcePath", "themeType")
	schema.requireStringProps(t, "resourcePath", "themeType")
	schema.requirePropCount(t, 2)
}

// TestSetThemeTypeVariationSchema pins the set-variation tool's input surface.
func TestSetThemeTypeVariationSchema(t *testing.T) {
	schema := requireSchema(t, "set_theme_type_variation", "Set a Theme type variation and its base type")
	schema.requireRequired(t, "resourcePath", "themeType", "baseType")
	schema.requireStringProps(t, "resourcePath", "themeType", "baseType")
	schema.requirePropCount(t, 3)
}

// TestClearThemeTypeVariationSchema pins the clear-variation tool's input surface.
func TestClearThemeTypeVariationSchema(t *testing.T) {
	schema := requireSchema(t, "clear_theme_type_variation", "Clear a Theme type variation")
	schema.requireRequired(t, "resourcePath", "themeType")
	schema.requireStringProps(t, "resourcePath", "themeType")
	schema.requirePropCount(t, 2)
}

// TestSetThemeItemSchema pins the set-item tool's input surface.
func TestSetThemeItemSchema(t *testing.T) {
	schema := requireSchema(t, "set_theme_item", "Set a Theme item on a type")
	schema.requireRequired(t, "resourcePath", "themeType", "itemType", "itemName", "value")
	schema.requireStringProps(t, "resourcePath", "themeType", "itemType", "itemName")
	schema.requirePropCount(t, 5)
}

// TestClearThemeItemSchema pins the clear-item tool's input surface.
func TestClearThemeItemSchema(t *testing.T) {
	schema := requireSchema(t, "clear_theme_item", "Clear a Theme item from a type")
	schema.requireRequired(t, "resourcePath", "themeType", "itemType", "itemName")
	schema.requireStringProps(t, "resourcePath", "themeType", "itemType", "itemName")
	schema.requirePropCount(t, 4)
}

// TestReadThemeSchema pins the read-only theme tool's input surface.
func TestReadThemeSchema(t *testing.T) {
	schema := requireSchema(t, "read_theme", "Read Theme resources, their types, variations, and items")
	schema.requireRequired(t, "resourcePath")
	schema.requireStringProps(t, "resourcePath")
	schema.requirePropCount(t, 1)
}

// testThemePayload is the subset of read_theme's payload the tests assert
// against.
type testThemePayload struct {
	Path             string           `json:"path"`
	DefaultFont      map[string]any   `json:"defaultFont"`
	DefaultFontSize  *float64         `json:"defaultFontSize"`
	DefaultBaseScale *float64         `json:"defaultBaseScale"`
	Types            []map[string]any `json:"types"`
}

// themeType finds a type by name in a read_theme payload.
func (p testThemePayload) themeType(name string) map[string]any {
	for _, themeType := range p.Types {
		if themeType["name"] == name {
			return themeType
		}
	}
	return nil
}

// readThemePayload calls read_theme and decodes its payload.
func readThemePayload(t *testing.T, session *mcp.ClientSession) testThemePayload {
	t.Helper()
	text := requireToolText(t, session, "read_theme", map[string]any{"resourcePath": "themes/main.tres"})
	var payload testThemePayload
	if err := json.Unmarshal([]byte(text), &payload); err != nil {
		t.Fatalf("unmarshal theme payload: %v\n%s", err, text)
	}
	return payload
}

// seedThemeResources writes the font, icon, and stylebox resources a theme item
// can reference. A SystemFont, GradientTexture2D, and StyleBoxFlat need no
// import step, unlike a raw font or PNG.
func seedThemeResources(t *testing.T, session *mcp.ClientSession) {
	t.Helper()
	for path, resourceType := range map[string]string{
		"themes/font.tres": "SystemFont",
		"themes/icon.tres": "GradientTexture2D",
		"themes/box.tres":  "StyleBoxFlat",
	} {
		requireToolText(t, session, "create_resource", map[string]any{
			"resourcePath": path,
			"resourceType": resourceType,
		})
	}
}

// TestThemeTools drives the real theme tools against Godot.
func TestThemeTools(t *testing.T) {
	t.Run("creates a theme with defaults", func(t *testing.T) {
		root := newProject(t)
		session := connectProject(t, root)
		requireGodot(t, session)
		seedThemeResources(t, session)

		text := requireToolText(t, session, "create_theme", map[string]any{
			"resourcePath":     "themes/main.tres",
			"defaultFont":      "themes/font.tres",
			"defaultFontSize":  16,
			"defaultBaseScale": 1.5,
		})
		requirePrefix(t, text, "manage_theme succeeded.")
		requireContains(t, readFile(t, filepath.Join(root, "themes", "main.tres")), `[gd_resource type="Theme"`)

		payload := readThemePayload(t, session)
		if payload.Path != "themes/main.tres" {
			t.Errorf("path = %q", payload.Path)
		}
		if payload.DefaultFontSize == nil || *payload.DefaultFontSize != 16 {
			t.Errorf("defaultFontSize = %v, want 16", payload.DefaultFontSize)
		}
		if payload.DefaultBaseScale == nil || *payload.DefaultBaseScale != 1.5 {
			t.Errorf("defaultBaseScale = %v, want 1.5", payload.DefaultBaseScale)
		}
		if payload.DefaultFont["$type"] != "Resource" || payload.DefaultFont["path"] != "res://themes/font.tres" {
			t.Errorf("defaultFont = %v", payload.DefaultFont)
		}
		if len(payload.Types) != 0 {
			t.Errorf("types = %v, want none", payload.Types)
		}
	})

	t.Run("sets and clears items", func(t *testing.T) {
		root := newProject(t)
		session := connectProject(t, root)
		requireGodot(t, session)
		seedThemeResources(t, session)
		requireToolText(t, session, "create_theme", map[string]any{
			"resourcePath": "themes/main.tres",
		})

		for _, args := range []map[string]any{
			{"themeType": "Label", "itemType": "color", "itemName": "font_color", "value": map[string]any{"$type": "Color", "r": 1, "g": 0, "b": 0, "a": 1}},
			{"themeType": "Label", "itemType": "constant", "itemName": "outline_size", "value": 4},
			{"themeType": "Label", "itemType": "font", "itemName": "font", "value": "themes/font.tres"},
			{"themeType": "Panel", "itemType": "stylebox", "itemName": "panel", "value": "themes/box.tres"},
			{"themeType": "Panel", "itemType": "icon", "itemName": "arrow", "value": "themes/icon.tres"},
		} {
			args["resourcePath"] = "themes/main.tres"
			requireToolText(t, session, "set_theme_item", args)
		}

		payload := readThemePayload(t, session)
		label := payload.themeType("Label")
		if label == nil {
			t.Fatalf("types = %v, want a Label type", payload.Types)
		}
		colors, _ := label["colors"].(map[string]any)
		color, _ := colors["font_color"].(map[string]any)
		if color["$type"] != "Color" || color["r"] != float64(1) {
			t.Errorf("font_color = %v, want a tagged red Color", colors["font_color"])
		}
		constants, _ := label["constants"].(map[string]any)
		if constants["outline_size"] != float64(4) {
			t.Errorf("outline_size = %v, want 4", constants["outline_size"])
		}
		fonts, _ := label["fonts"].(map[string]any)
		font, _ := fonts["font"].(map[string]any)
		if font["path"] != "res://themes/font.tres" {
			t.Errorf("font = %v", fonts["font"])
		}
		panel := payload.themeType("Panel")
		if panel == nil {
			t.Fatalf("types = %v, want a Panel type", payload.Types)
		}

		requireToolText(t, session, "clear_theme_item", map[string]any{
			"resourcePath": "themes/main.tres",
			"themeType":    "Label",
			"itemType":     "constant",
			"itemName":     "outline_size",
		})
		payload = readThemePayload(t, session)
		constants, _ = payload.themeType("Label")["constants"].(map[string]any)
		if len(constants) != 0 {
			t.Errorf("constants = %v, want none after clearing", constants)
		}
	})

	t.Run("manages type variations", func(t *testing.T) {
		root := newProject(t)
		session := connectProject(t, root)
		requireGodot(t, session)
		requireToolText(t, session, "create_theme", map[string]any{
			"resourcePath": "themes/main.tres",
		})

		requireToolText(t, session, "set_theme_type_variation", map[string]any{
			"resourcePath": "themes/main.tres",
			"themeType":    "CustomButton",
			"baseType":     "Button",
		})
		payload := readThemePayload(t, session)
		custom := payload.themeType("CustomButton")
		if custom == nil || custom["variationBase"] != "Button" {
			t.Fatalf("types = %v, want CustomButton based on Button", payload.Types)
		}

		requireToolText(t, session, "clear_theme_type_variation", map[string]any{
			"resourcePath": "themes/main.tres",
			"themeType":    "CustomButton",
		})
		payload = readThemePayload(t, session)
		if payload.themeType("CustomButton") != nil {
			t.Errorf("types = %v, want CustomButton gone after clearing its variation", payload.Types)
		}
	})

	t.Run("removes a type with its items", func(t *testing.T) {
		root := newProject(t)
		session := connectProject(t, root)
		requireGodot(t, session)
		requireToolText(t, session, "create_theme", map[string]any{
			"resourcePath": "themes/main.tres",
		})
		requireToolText(t, session, "set_theme_item", map[string]any{
			"resourcePath": "themes/main.tres",
			"themeType":    "Label",
			"itemType":     "constant",
			"itemName":     "outline_size",
			"value":        4,
		})

		requireToolText(t, session, "remove_theme_type", map[string]any{
			"resourcePath": "themes/main.tres",
			"themeType":    "Label",
		})
		payload := readThemePayload(t, session)
		if payload.themeType("Label") != nil {
			t.Errorf("types = %v, want Label gone after removal", payload.Types)
		}
	})

	t.Run("modifies theme defaults", func(t *testing.T) {
		root := newProject(t)
		session := connectProject(t, root)
		requireGodot(t, session)
		requireToolText(t, session, "create_theme", map[string]any{
			"resourcePath":    "themes/main.tres",
			"defaultFontSize": 12,
		})
		requireToolText(t, session, "modify_theme", map[string]any{
			"resourcePath":    "themes/main.tres",
			"defaultFontSize": 20,
		})

		payload := readThemePayload(t, session)
		if payload.DefaultFontSize == nil || *payload.DefaultFontSize != 20 {
			t.Errorf("defaultFontSize = %v, want 20", payload.DefaultFontSize)
		}
	})
}

// TestThemeErrors covers the validation and engine failure paths.
func TestThemeErrors(t *testing.T) {
	t.Run("missing params", func(t *testing.T) {
		session := connect(t)
		requireToolError(t, session, "create_theme", map[string]any{}, "resourcePath is required.")
	})

	t.Run("setItem without a value", func(t *testing.T) {
		session := connect(t)
		requireToolError(t, session, "set_theme_item", map[string]any{
			"resourcePath": "themes/main.tres",
			"themeType":    "Label",
			"itemType":     "color",
			"itemName":     "font_color",
		}, "resourcePath, themeType, itemType, itemName, and value are required.")
	})

	t.Run("setTypeVariation without a base type", func(t *testing.T) {
		session := connect(t)
		requireToolError(t, session, "set_theme_type_variation", map[string]any{
			"resourcePath": "themes/main.tres",
			"themeType":    "CustomButton",
		}, "resourcePath, themeType, and baseType are required.")
	})

	t.Run("unknown item type", func(t *testing.T) {
		root := newProject(t)
		session := connectProject(t, root)
		requireGodot(t, session)
		requireToolText(t, session, "create_theme", map[string]any{
			"resourcePath": "themes/main.tres",
		})

		res := callTool(t, session, "set_theme_item", map[string]any{
			"resourcePath": "themes/main.tres",
			"themeType":    "Label",
			"itemType":     "bogus",
			"itemName":     "font_color",
			"value":        map[string]any{"$type": "Color", "r": 1, "g": 0, "b": 0, "a": 1},
		})
		if !res.IsError {
			t.Fatal("expected an error result")
		}
		requireContains(t, firstText(t, res), "manage_theme failed", "unknown itemType: bogus")
	})

	t.Run("missing theme", func(t *testing.T) {
		root := newProject(t)
		session := connectProject(t, root)
		requireGodot(t, session)

		res := callTool(t, session, "read_theme", map[string]any{"resourcePath": "themes/missing.tres"})
		if !res.IsError {
			t.Fatal("expected an error result")
		}
		requireContains(t, firstText(t, res), "manage_theme failed", "Theme resource not found")
	})

	t.Run("not a theme resource", func(t *testing.T) {
		root := newProject(t)
		session := connectProject(t, root)
		requireGodot(t, session)

		requireToolText(t, session, "create_resource", map[string]any{
			"resourcePath": "resources/mat.tres",
			"resourceType": "StandardMaterial3D",
		})
		res := callTool(t, session, "read_theme", map[string]any{"resourcePath": "resources/mat.tres"})
		if !res.IsError {
			t.Fatal("expected an error result")
		}
		requireContains(t, firstText(t, res), "manage_theme failed", "Not a Theme resource")
	})
}
