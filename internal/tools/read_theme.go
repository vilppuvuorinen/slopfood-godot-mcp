package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"

	"github.com/vilppuvuorinen/slopfood-godot-mcp/internal/mcp"
)

func readTheme() mcp.Tool {
	return mcp.Tool{
		Name:        "read_theme",
		Title:       "Read Theme",
		Description: "Read Theme resources, their types, variations, and items",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: mcp.Bool(true), OpenWorldHint: mcp.Bool(false)},
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"resourcePath": map[string]any{
					"type":        "string",
					"description": "Theme resource file path (relative to project)",
				},
			},
			"required": []string{"resourcePath"},
		},
		OutputSchema: themePayloadSchema(),
		Handler:      handleReadTheme,
	}
}

// themePayloadSchema describes read_theme's payload: the theme-level defaults
// plus one object per theme type.
func themePayloadSchema() map[string]any {
	return payloadOutputSchema(map[string]any{
		"path":             map[string]any{"type": "string", "description": "Theme resource path, relative to the bound project"},
		"defaultFont":      godotValueSchema("Default font as a tagged Resource, or null when unset"),
		"defaultFontSize":  themeNullableSchema("integer", "Default font size in pixels, or null when unset"),
		"defaultBaseScale": themeNullableSchema("number", "Default base scale, or null when unset"),
		"types":            map[string]any{"type": "array", "description": "Theme types in name order", "items": themeTypeSchema()},
	})
}

// themeNullableSchema is a payload field that is either the given JSON type or
// null, so an unset theme default is advertised rather than described as a bare
// zero.
func themeNullableSchema(typ, description string) map[string]any {
	return map[string]any{
		"type":        []any{typ, "null"},
		"description": description,
	}
}

// themeTypeSchema describes one theme type and its items grouped by category.
func themeTypeSchema() map[string]any {
	return map[string]any{
		"type":        "object",
		"description": "One theme type and its items",
		"properties": map[string]any{
			"name":          map[string]any{"type": "string", "description": "Theme type name"},
			"variationBase": map[string]any{"type": "string", "description": "Base type this type is a variation of, or an empty string"},
			"colors":        themeItemMapSchema("Color items in the tool value contract"),
			"constants":     themeItemMapSchema("Constant items as integers"),
			"fonts":         themeItemMapSchema("Font items as tagged Resources"),
			"fontSizes":     themeItemMapSchema("Font size items as integers"),
			"icons":         themeItemMapSchema("Icon items as tagged Resources"),
			"styleboxes":    themeItemMapSchema("StyleBox items as tagged Resources"),
		},
		"required": []string{"name", "variationBase", "colors", "constants", "fonts", "fontSizes", "icons", "styleboxes"},
	}
}

// themeItemMapSchema is one item category's name-to-value map.
func themeItemMapSchema(description string) map[string]any {
	return map[string]any{
		"type":                 "object",
		"description":          description,
		"additionalProperties": godotValueSchema("An item value"),
	}
}

type readThemeArgs struct {
	ResourcePath string `json:"resourcePath"`
}

func handleReadTheme(ctx context.Context, raw json.RawMessage) (any, error) {
	var args readThemeArgs
	if len(raw) > 0 && string(raw) != "null" {
		if err := json.Unmarshal(raw, &args); err != nil {
			return nil, fmt.Errorf("invalid arguments: %w", err)
		}
	}
	project := projectRoot(ctx)
	if args.ResourcePath == "" {
		return requiredArgsResult("resourcePath"), nil
	}
	if !validPath(project) || !validPath(args.ResourcePath) {
		return invalidPathResult(), nil
	}
	if !pathExists(filepath.Join(project, "project.godot")) {
		return notAProjectResult(project), nil
	}

	params := map[string]any{
		"resourcePath": args.ResourcePath,
		"action":       "read",
	}
	fields := map[string]any{
		"resourcePath": args.ResourcePath,
		"action":       "read",
	}
	return runHeadlessOperation(ctx, project, "manage_theme", params, fields)
}
