package e2e

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type testThemePayload struct {
	Path             string           `json:"path"`
	DefaultFont      map[string]any   `json:"defaultFont"`
	DefaultFontSize  *float64         `json:"defaultFontSize"`
	DefaultBaseScale *float64         `json:"defaultBaseScale"`
	Types            []map[string]any `json:"types"`
}

func (p testThemePayload) themeType(name string) map[string]any {
	for _, themeType := range p.Types {
		if themeType["name"] == name {
			return themeType
		}
	}
	return nil
}

func readThemePayload(t *testing.T, session *mcp.ClientSession, resourcePath string) testThemePayload {
	t.Helper()
	text := requireToolText(t, session, "read_theme", map[string]any{"resourcePath": resourcePath})
	var payload testThemePayload
	if err := json.Unmarshal([]byte(text), &payload); err != nil {
		t.Fatalf("unmarshal theme payload: %v\n%s", err, text)
	}
	return payload
}

// SystemFont, GradientTexture2D, and StyleBoxFlat need no import step, unlike a raw font or PNG.
func seedThemeResources(t *testing.T, session *mcp.ClientSession, prefix string) {
	t.Helper()
	for name, resourceType := range map[string]string{
		"font.tres": "SystemFont",
		"icon.tres": "GradientTexture2D",
		"box.tres":  "StyleBoxFlat",
	} {
		requireToolText(t, session, "create_resource", map[string]any{
			"resourcePath": prefix + name,
			"resourceType": resourceType,
		})
	}
}

func TestThemeTools(t *testing.T) {
	t.Parallel()
	root := newProject(t)
	session := connectProject(t, root)
	seedThemeResources(t, session, "themes/shared_")

	t.Run("creates a theme with defaults", func(t *testing.T) {
		const path = "themes/defaults.tres"
		text := requireToolText(t, session, "create_theme", map[string]any{
			"resourcePath":     path,
			"defaultFont":      "themes/shared_font.tres",
			"defaultFontSize":  16,
			"defaultBaseScale": 1.5,
		})
		requirePrefix(t, text, "manage_theme succeeded.")
		requireContains(t, readFile(t, filepath.Join(root, "themes", "defaults.tres")), `[gd_resource type="Theme"`)

		payload := readThemePayload(t, session, path)
		if payload.Path != path {
			t.Errorf("path = %q", payload.Path)
		}
		if payload.DefaultFontSize == nil || *payload.DefaultFontSize != 16 {
			t.Errorf("defaultFontSize = %v, want 16", payload.DefaultFontSize)
		}
		if payload.DefaultBaseScale == nil || *payload.DefaultBaseScale != 1.5 {
			t.Errorf("defaultBaseScale = %v, want 1.5", payload.DefaultBaseScale)
		}
		if payload.DefaultFont["$type"] != "Resource" || payload.DefaultFont["path"] != "res://themes/shared_font.tres" {
			t.Errorf("defaultFont = %v", payload.DefaultFont)
		}
		if len(payload.Types) != 0 {
			t.Errorf("types = %v, want none", payload.Types)
		}
	})

	t.Run("sets and clears items", func(t *testing.T) {
		const path = "themes/items.tres"
		requireToolText(t, session, "create_theme", map[string]any{
			"resourcePath": path,
		})

		for _, args := range []map[string]any{
			{"themeType": "Label", "itemType": "color", "itemName": "font_color", "value": map[string]any{"$type": "Color", "r": 1, "g": 0, "b": 0, "a": 1}},
			{"themeType": "Label", "itemType": "constant", "itemName": "outline_size", "value": 4},
			{"themeType": "Label", "itemType": "font", "itemName": "font", "value": "themes/shared_font.tres"},
			{"themeType": "Panel", "itemType": "stylebox", "itemName": "panel", "value": "themes/shared_box.tres"},
			{"themeType": "Panel", "itemType": "icon", "itemName": "arrow", "value": "themes/shared_icon.tres"},
		} {
			args["resourcePath"] = path
			requireToolText(t, session, "set_theme_item", args)
		}

		payload := readThemePayload(t, session, path)
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
		if font["path"] != "res://themes/shared_font.tres" {
			t.Errorf("font = %v", fonts["font"])
		}
		panel := payload.themeType("Panel")
		if panel == nil {
			t.Fatalf("types = %v, want a Panel type", payload.Types)
		}

		requireToolText(t, session, "clear_theme_item", map[string]any{
			"resourcePath": path,
			"themeType":    "Label",
			"itemType":     "constant",
			"itemName":     "outline_size",
		})
		payload = readThemePayload(t, session, path)
		constants, _ = payload.themeType("Label")["constants"].(map[string]any)
		if len(constants) != 0 {
			t.Errorf("constants = %v, want none after clearing", constants)
		}
	})

	t.Run("manages type variations", func(t *testing.T) {
		const path = "themes/variations.tres"
		requireToolText(t, session, "create_theme", map[string]any{
			"resourcePath": path,
		})

		requireToolText(t, session, "set_theme_type_variation", map[string]any{
			"resourcePath": path,
			"themeType":    "CustomButton",
			"baseType":     "Button",
		})
		payload := readThemePayload(t, session, path)
		custom := payload.themeType("CustomButton")
		if custom == nil || custom["variationBase"] != "Button" {
			t.Fatalf("types = %v, want CustomButton based on Button", payload.Types)
		}

		requireToolText(t, session, "clear_theme_type_variation", map[string]any{
			"resourcePath": path,
			"themeType":    "CustomButton",
		})
		payload = readThemePayload(t, session, path)
		if payload.themeType("CustomButton") != nil {
			t.Errorf("types = %v, want CustomButton gone after clearing its variation", payload.Types)
		}
	})

	t.Run("removes a type with its items", func(t *testing.T) {
		const path = "themes/removals.tres"
		requireToolText(t, session, "create_theme", map[string]any{
			"resourcePath": path,
		})
		requireToolText(t, session, "set_theme_item", map[string]any{
			"resourcePath": path,
			"themeType":    "Label",
			"itemType":     "constant",
			"itemName":     "outline_size",
			"value":        4,
		})

		requireToolText(t, session, "remove_theme_type", map[string]any{
			"resourcePath": path,
			"themeType":    "Label",
		})
		payload := readThemePayload(t, session, path)
		if payload.themeType("Label") != nil {
			t.Errorf("types = %v, want Label gone after removal", payload.Types)
		}
	})

	t.Run("modifies theme defaults", func(t *testing.T) {
		const path = "themes/modify.tres"
		requireToolText(t, session, "create_theme", map[string]any{
			"resourcePath":    path,
			"defaultFontSize": 12,
		})
		requireToolText(t, session, "modify_theme", map[string]any{
			"resourcePath":    path,
			"defaultFontSize": 20,
		})

		payload := readThemePayload(t, session, path)
		if payload.DefaultFontSize == nil || *payload.DefaultFontSize != 20 {
			t.Errorf("defaultFontSize = %v, want 20", payload.DefaultFontSize)
		}
	})
}

func TestThemeErrors(t *testing.T) {
	t.Parallel()
	validationSession := connect(t)
	t.Run("missing params", func(t *testing.T) {
		requireToolError(t, validationSession, "create_theme", map[string]any{}, "resourcePath is required.")
	})

	t.Run("setItem without a value", func(t *testing.T) {
		requireToolError(t, validationSession, "set_theme_item", map[string]any{
			"resourcePath": "themes/main.tres",
			"themeType":    "Label",
			"itemType":     "color",
			"itemName":     "font_color",
		}, "resourcePath, themeType, itemType, itemName, and value are required.")
	})

	t.Run("setTypeVariation without a base type", func(t *testing.T) {
		requireToolError(t, validationSession, "set_theme_type_variation", map[string]any{
			"resourcePath": "themes/main.tres",
			"themeType":    "CustomButton",
		}, "resourcePath, themeType, and baseType are required.")
	})

	root := newProject(t)
	session := connectProject(t, root)

	t.Run("unknown item type", func(t *testing.T) {
		requireToolText(t, session, "create_theme", map[string]any{
			"resourcePath": "themes/unknown_item.tres",
		})

		res := callTool(t, session, "set_theme_item", map[string]any{
			"resourcePath": "themes/unknown_item.tres",
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
		res := callTool(t, session, "read_theme", map[string]any{"resourcePath": "themes/missing.tres"})
		if !res.IsError {
			t.Fatal("expected an error result")
		}
		requireContains(t, firstText(t, res), "manage_theme failed", "Theme resource not found")
	})

	t.Run("not a theme resource", func(t *testing.T) {
		requireToolText(t, session, "create_resource", map[string]any{
			"resourcePath": "resources/mat_theme.tres",
			"resourceType": "StandardMaterial3D",
		})
		res := callTool(t, session, "read_theme", map[string]any{"resourcePath": "resources/mat_theme.tres"})
		if !res.IsError {
			t.Fatal("expected an error result")
		}
		requireContains(t, firstText(t, res), "manage_theme failed", "Not a Theme resource")
	})
}
