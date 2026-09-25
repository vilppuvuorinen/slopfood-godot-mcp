package e2e

import (
	"encoding/json"
	"path/filepath"
	"testing"
)

// resourceTres is a minimal StandardMaterial3D resource so the read/modify
// manage_resource e2e tests start from an existing file.
const resourceTres = `[gd_resource type="StandardMaterial3D" format=3]

[resource]
metallic = 0.5
`

// TestManageResourceSchema ports the reference's tool-definition expectations
// for manage_resource over the real stdio surface.
func TestManageResourceSchema(t *testing.T) {
	schema := requireSchema(t, "manage_resource", "Create, read, or modify .tres/.res resource files")
	schema.requireRequired(t, "resourcePath", "action")
	schema.requireStringProps(t, "resourcePath", "action", "resourceType")
	schema.requireType(t, "properties", "object")
	schema.requireType(t, "propertyFilter", "array")
	schema.requirePropCount(t, 5)
}

// TestManageResourceTool drives the real manage_resource tool against Godot.
func TestManageResourceTool(t *testing.T) {
	t.Run("reads a resource", func(t *testing.T) {
		root := newProject(t)
		session := connectProject(t, root)
		requireGodot(t, session)
		writeE2EFile(t, filepath.Join(root, "resources", "mat.tres"), resourceTres)

		text := requireToolText(t, session, "manage_resource", map[string]any{
			"resourcePath": "resources/mat.tres",
			"action":       "read",
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
		writeE2EFile(t, filepath.Join(root, "resources", "mat.tres"), resourceTres)

		text := requireToolText(t, session, "manage_resource", map[string]any{
			"resourcePath":   "resources/mat.tres",
			"action":         "read",
			"propertyFilter": []any{"metallic"},
		})
		requireContains(t, text, `"metallic": 0.5`)
		requireNotContains(t, text, `"albedo_color"`)

		text = requireToolText(t, session, "manage_resource", map[string]any{
			"resourcePath":   "resources/mat.tres",
			"action":         "read",
			"propertyFilter": []any{"albedo_*"},
		})
		requireContains(t, text, `"albedo_color"`)
		requireNotContains(t, text, `"metallic"`)
	})

	t.Run("modifies then re-reads a resource", func(t *testing.T) {
		root := newProject(t)
		session := connectProject(t, root)
		requireGodot(t, session)
		writeE2EFile(t, filepath.Join(root, "resources", "mat.tres"), resourceTres)

		modifyText := requireToolText(t, session, "manage_resource", map[string]any{
			"resourcePath": "resources/mat.tres",
			"action":       "modify",
			"properties":   map[string]any{"metallic": 0.75},
		})
		requirePrefix(t, modifyText, "manage_resource succeeded.")

		readText := requireToolText(t, session, "manage_resource", map[string]any{
			"resourcePath": "resources/mat.tres",
			"action":       "read",
		})
		requireContains(t, readText, `"metallic": 0.75`)
	})

	t.Run("creates then reads a resource", func(t *testing.T) {
		root := newProject(t)
		session := connectProject(t, root)
		requireGodot(t, session)

		createText := requireToolText(t, session, "manage_resource", map[string]any{
			"resourcePath": "materials/red.tres",
			"action":       "create",
			"resourceType": "StandardMaterial3D",
			"properties": map[string]any{
				"albedo_color": map[string]any{"$type": "Color", "r": 1, "g": 0, "b": 0, "a": 1},
			},
		})
		requirePrefix(t, createText, "manage_resource succeeded.")

		content := readFile(t, filepath.Join(root, "materials", "red.tres"))
		requireContains(t, content, `[gd_resource type="StandardMaterial3D"`)

		readText := requireToolText(t, session, "manage_resource", map[string]any{
			"resourcePath": "materials/red.tres",
			"action":       "read",
		})
		requireContains(t, readText, `"type": "StandardMaterial3D"`, `"path": "materials/red.tres"`)
	})
}

// TestManageResourceTheme covers the Theme codec folded into manage_resource:
// create/read/modify with the theme item type inference intact.
func TestManageResourceTheme(t *testing.T) {
	root := newProject(t)
	session := connectProject(t, root)
	requireGodot(t, session)

	createText := requireToolText(t, session, "manage_resource", map[string]any{
		"resourcePath": "themes/main.tres",
		"action":       "create",
		"resourceType": "Theme",
		"properties":   map[string]any{"default_font_size": 16},
	})
	requirePrefix(t, createText, "manage_resource succeeded.")

	themeFile := filepath.Join(root, "themes", "main.tres")
	requireContains(t, readFile(t, themeFile), `[gd_resource type="Theme"`)

	readText := requireToolText(t, session, "manage_resource", map[string]any{
		"resourcePath": "themes/main.tres",
		"action":       "read",
	})
	requireContains(t, readText, `"type": "Theme"`, `"properties"`, `"default_font_size": 16`)
	requireNotContains(t, readText, "THEME_JSON_START")

	modifyText := requireToolText(t, session, "manage_resource", map[string]any{
		"resourcePath": "themes/main.tres",
		"action":       "modify",
		"properties":   map[string]any{"default_font_size": 20},
	})
	requirePrefix(t, modifyText, "manage_resource succeeded.")

	reReadText := requireToolText(t, session, "manage_resource", map[string]any{
		"resourcePath": "themes/main.tres",
		"action":       "read",
	})
	requireContains(t, reReadText, `"type": "Theme"`, `"default_font_size": 20`)
}

// TestManageResourceThemeConvertsTypes covers theme properties whose JSON
// values need conversion: a color sent as a component dictionary, a font size,
// and a Theme item that references another resource by path. The saved .tres is
// inspected so a silently-ignored or mis-typed value fails the test.
func TestManageResourceThemeConvertsTypes(t *testing.T) {
	root := newProject(t)
	session := connectProject(t, root)
	requireGodot(t, session)

	requireToolText(t, session, "manage_resource", map[string]any{
		"resourcePath": "themes/panel_box.tres",
		"action":       "create",
		"resourceType": "StyleBoxFlat",
	})

	createText := requireToolText(t, session, "manage_resource", map[string]any{
		"resourcePath": "themes/main.tres",
		"action":       "create",
		"resourceType": "Theme",
		"properties": map[string]any{
			"Label/colors/font_color":    map[string]any{"$type": "Color", "r": 1, "g": 0, "b": 0, "a": 1},
			"Label/font_sizes/font_size": 24,
			"Panel/styles/panel":         "res://themes/panel_box.tres",
		},
	})
	requirePrefix(t, createText, "manage_resource succeeded.")

	themeFile := filepath.Join(root, "themes", "main.tres")
	content := readFile(t, themeFile)
	requireContains(t, content,
		"Color(1, 0, 0, 1)",
		"Label/font_sizes/font_size = 24",
		"panel_box.tres",
	)
	requireNotContains(t, content, "Panel/styles/panel = null")

	readText := requireToolText(t, session, "manage_resource", map[string]any{
		"resourcePath": "themes/main.tres",
		"action":       "read",
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

	modifyText := requireToolText(t, session, "manage_resource", map[string]any{
		"resourcePath": "themes/main.tres",
		"action":       "modify",
		"properties": map[string]any{
			"Label/colors/font_color": map[string]any{"$type": "Color", "r": 0, "g": 0, "b": 1, "a": 1},
		},
	})
	requirePrefix(t, modifyText, "manage_resource succeeded.")

	requireContains(t, readFile(t, themeFile), "Color(0, 0, 1, 1)")
}

// TestManageResourceErrors covers the reference's validation and failure
// messages.
func TestManageResourceErrors(t *testing.T) {
	t.Run("missing params", func(t *testing.T) {
		session := connect(t)
		requireToolError(t, session, "manage_resource", map[string]any{}, "resourcePath and action are required.")
	})

	t.Run("create without resource type", func(t *testing.T) {
		root := newProject(t)
		session := connectProject(t, root)
		requireToolError(t, session, "manage_resource", map[string]any{
			"resourcePath": "materials/red.tres",
			"action":       "create",
		}, "resourceType is required for the create action.")
	})

	t.Run("not a project", func(t *testing.T) {
		root := t.TempDir()
		session := connectProject(t, root)
		requireToolError(t, session, "manage_resource", map[string]any{
			"resourcePath": "mat.tres",
			"action":       "read",
		}, "Not a valid Godot project: "+root)
	})

	t.Run("unknown action", func(t *testing.T) {
		root := newProject(t)
		session := connectProject(t, root)
		requireGodot(t, session)

		res := callTool(t, session, "manage_resource", map[string]any{
			"resourcePath": "resources/mat.tres",
			"action":       "bogus",
		})
		if !res.IsError {
			t.Fatal("expected an error result")
		}
		requireContains(t, firstText(t, res), "Unknown manage_resource action: bogus")
	})

	t.Run("unknown resource type", func(t *testing.T) {
		root := newProject(t)
		session := connectProject(t, root)
		requireGodot(t, session)

		res := callTool(t, session, "manage_resource", map[string]any{
			"resourcePath": "materials/red.tres",
			"action":       "create",
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
		res := callTool(t, session, "manage_resource", map[string]any{
			"resourcePath": "materials/red.tres",
			"action":       "create",
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
		res := callTool(t, session, "manage_resource", map[string]any{
			"resourcePath": "materials/red.tres",
			"action":       "create",
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

		res := callTool(t, session, "manage_resource", map[string]any{
			"resourcePath": "resources/missing.tres",
			"action":       "read",
		})
		if !res.IsError {
			t.Fatal("expected an error result")
		}
		requireContains(t, firstText(t, res), "manage_resource failed", "Resource not found: res://resources/missing.tres")
	})

	t.Run("unloadable resource", func(t *testing.T) {
		root := newProject(t)
		session := connectProject(t, root)
		requireGodot(t, session)
		writeE2EFile(t, filepath.Join(root, "resources", "bad.tres"), "this is not a valid resource\n")

		res := callTool(t, session, "manage_resource", map[string]any{
			"resourcePath": "resources/bad.tres",
			"action":       "read",
		})
		if !res.IsError {
			t.Fatal("expected an error result")
		}
		requireContains(t, firstText(t, res), "manage_resource failed", "Failed to load resource: res://resources/bad.tres")
	})

	t.Run("invalid property value", func(t *testing.T) {
		root := newProject(t)
		session := connectProject(t, root)
		requireGodot(t, session)

		res := callTool(t, session, "manage_resource", map[string]any{
			"resourcePath": "materials/red.tres",
			"action":       "create",
			"resourceType": "StandardMaterial3D",
			"properties":   map[string]any{"metallic": "not-a-number"},
		})
		if !res.IsError {
			t.Fatal("expected an error result")
		}
		requireContains(t, firstText(t, res), "manage_resource failed", "Invalid property value")
	})
}
