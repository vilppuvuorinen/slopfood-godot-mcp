package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"

	"github.com/vilppuvuorinen/go-dot-mcp/internal/mcp"
)

// themeItemTypeNames are the tool contract's names for the Theme item
// categories, shared with the engine bridge in godot_operations.gd.
var themeItemTypeNames = []string{"color", "constant", "font", "fontSize", "icon", "stylebox"}

func manageTheme() mcp.Tool {
	return mcp.Tool{
		Name:        "manage_theme",
		Title:       "Manage Theme",
		Description: "Create or modify Theme resources, their types, variations, and items",
		Annotations: &mcp.ToolAnnotations{DestructiveHint: mcp.Bool(true), IdempotentHint: mcp.Bool(true), OpenWorldHint: mcp.Bool(false)},
		InputSchema: actionInputSchema(
			map[string]any{
				"resourcePath": map[string]any{
					"type":        "string",
					"description": "Theme resource file path (relative to project)",
				},
				"action": map[string]any{
					"type":        "string",
					"enum":        []string{"create", "modify", "removeType", "setTypeVariation", "clearTypeVariation", "setItem", "clearItem"},
					"description": "Action: create/modify a theme, remove a type, manage type variations, or set/clear an item",
				},
				"defaultFont":      godotValueSchema("Default font as a res:// path or tagged Resource (create, modify)"),
				"defaultFontSize":  map[string]any{"type": "integer", "description": "Default font size in pixels (create, modify)"},
				"defaultBaseScale": map[string]any{"type": "number", "description": "Default base scale for scale-aware themes (create, modify)"},
				"themeType": map[string]any{
					"type":        "string",
					"description": "Theme type such as Button (removeType, setTypeVariation, clearTypeVariation, setItem, clearItem)",
				},
				"baseType": map[string]any{
					"type":        "string",
					"description": "Base type a variation derives from (setTypeVariation)",
				},
				"itemType": map[string]any{
					"type":        "string",
					"enum":        themeItemTypeNames,
					"description": "Item category: color, constant, font, fontSize, icon, or stylebox (setItem, clearItem)",
				},
				"itemName": map[string]any{
					"type":        "string",
					"description": "Item name such as font_color (setItem, clearItem)",
				},
				"value": godotValueSchema("Item value: a tagged Color, an integer for constant/fontSize, or a res:// path or tagged Resource for font/icon/stylebox (setItem)"),
			},
			[]string{"resourcePath", "action"},
			[]actionVariant{
				{Action: "create"},
				{Action: "modify", AnyOf: []string{"defaultFont", "defaultFontSize", "defaultBaseScale"}},
				{Action: "removeType", Required: []string{"themeType"}},
				{Action: "setTypeVariation", Required: []string{"themeType", "baseType"}},
				{Action: "clearTypeVariation", Required: []string{"themeType"}},
				{Action: "setItem", Required: []string{"themeType", "itemType", "itemName", "value"}},
				{Action: "clearItem", Required: []string{"themeType", "itemType", "itemName"}},
			},
		),
		OutputSchema: statusOutputSchema(map[string]any{
			"resourcePath": map[string]any{"type": "string"},
			"action":       map[string]any{"type": "string"},
		}),
		Handler: handleManageTheme,
	}
}

type manageThemeArgs struct {
	ResourcePath     string `json:"resourcePath"`
	Action           string `json:"action"`
	DefaultFont      any    `json:"defaultFont"`
	DefaultFontSize  *int   `json:"defaultFontSize"`
	DefaultBaseScale any    `json:"defaultBaseScale"`
	ThemeType        string `json:"themeType"`
	BaseType         string `json:"baseType"`
	ItemType         string `json:"itemType"`
	ItemName         string `json:"itemName"`
	Value            any    `json:"value"`
}

func handleManageTheme(ctx context.Context, raw json.RawMessage) (any, error) {
	var args manageThemeArgs
	if len(raw) > 0 && string(raw) != "null" {
		if err := json.Unmarshal(raw, &args); err != nil {
			return nil, fmt.Errorf("invalid arguments: %w", err)
		}
	}
	project := projectRoot(ctx)
	if args.ResourcePath == "" || args.Action == "" {
		return requiredArgsResult("resourcePath", "action"), nil
	}
	if result, ok := validateThemeAction(args); !ok {
		return result, nil
	}
	if !validPath(project) || !validPath(args.ResourcePath) {
		return invalidPathResult(), nil
	}
	if !pathExists(filepath.Join(project, "project.godot")) {
		return notAProjectResult(project), nil
	}

	params := map[string]any{
		"resourcePath": args.ResourcePath,
		"action":       args.Action,
	}
	fields := map[string]any{
		"resourcePath": args.ResourcePath,
		"action":       args.Action,
	}
	addThemeProperties(params, args)
	addThemeFields(params, args)

	return runHeadlessOperation(ctx, project, "manage_theme", params, fields)
}

// validateThemeAction enforces each action's own required fields. The input
// schema's oneOf advertises them; this is the server-side guarantee.
func validateThemeAction(args manageThemeArgs) (mcp.ToolResult, bool) {
	missing := func(field string) (mcp.ToolResult, bool) {
		return mcp.ErrorResult(field + " is required for the " + args.Action + " action."), false
	}
	switch args.Action {
	case "create":
		return mcp.ToolResult{}, true
	case "modify":
		if args.DefaultFont == nil && args.DefaultFontSize == nil && args.DefaultBaseScale == nil {
			return effectfulFieldsResult("modify", "defaultFont", "defaultFontSize", "defaultBaseScale"), false
		}
	case "removeType", "clearTypeVariation":
		if args.ThemeType == "" {
			return missing("themeType")
		}
	case "setTypeVariation":
		if args.ThemeType == "" {
			return missing("themeType")
		}
		if args.BaseType == "" {
			return missing("baseType")
		}
	case "setItem":
		if args.ThemeType == "" {
			return missing("themeType")
		}
		if args.ItemType == "" {
			return missing("itemType")
		}
		if args.ItemName == "" {
			return missing("itemName")
		}
		if args.Value == nil {
			return missing("value")
		}
	case "clearItem":
		if args.ThemeType == "" {
			return missing("themeType")
		}
		if args.ItemType == "" {
			return missing("itemType")
		}
		if args.ItemName == "" {
			return missing("itemName")
		}
	default:
		return mcp.ErrorResult("Unknown action: " + args.Action + ". Use create, modify, removeType, setTypeVariation, clearTypeVariation, setItem, or clearItem."), false
	}
	return mcp.ToolResult{}, true
}

// addThemeProperties copies the theme-level default fields onto params when
// present.
func addThemeProperties(params map[string]any, args manageThemeArgs) {
	if args.DefaultFont != nil {
		params["defaultFont"] = args.DefaultFont
	}
	if args.DefaultFontSize != nil {
		params["defaultFontSize"] = *args.DefaultFontSize
	}
	if args.DefaultBaseScale != nil {
		params["defaultBaseScale"] = args.DefaultBaseScale
	}
}

// addThemeFields copies the type, variation, and item fields onto params when
// present.
func addThemeFields(params map[string]any, args manageThemeArgs) {
	if args.ThemeType != "" {
		params["themeType"] = args.ThemeType
	}
	if args.BaseType != "" {
		params["baseType"] = args.BaseType
	}
	if args.ItemType != "" {
		params["itemType"] = args.ItemType
	}
	if args.ItemName != "" {
		params["itemName"] = args.ItemName
	}
	if args.Value != nil {
		params["value"] = args.Value
	}
}
