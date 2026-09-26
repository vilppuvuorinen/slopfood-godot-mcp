package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"

	"github.com/vilppuvuorinen/slopfood-godot-mcp/internal/mcp"
)

// themeItemTypeNames are the tool contract's names for the Theme item
// categories, shared with the engine bridge in godot_operations.gd.
var themeItemTypeNames = []string{"color", "constant", "font", "fontSize", "icon", "stylebox"}

// themeItemTypeSchema describes the item category a theme item belongs to.
func themeItemTypeSchema() map[string]any {
	return map[string]any{
		"type":        "string",
		"enum":        themeItemTypeNames,
		"description": "Item category: color, constant, font, fontSize, icon, or stylebox",
	}
}

func setThemeItem() mcp.Tool {
	return mcp.Tool{
		Name:        "set_theme_item",
		Title:       "Set Theme Item",
		Description: "Set a Theme item on a type",
		Annotations: &mcp.ToolAnnotations{DestructiveHint: mcp.Bool(true), IdempotentHint: mcp.Bool(true), OpenWorldHint: mcp.Bool(false)},
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"resourcePath": map[string]any{
					"type":        "string",
					"description": "Theme resource file path (relative to project)",
				},
				"themeType": map[string]any{
					"type":        "string",
					"description": "Theme type such as Button",
				},
				"itemType": themeItemTypeSchema(),
				"itemName": map[string]any{
					"type":        "string",
					"description": "Item name such as font_color",
				},
				"value": godotValueSchema("Item value: a tagged Color, an integer for constant/fontSize, or a res:// path or tagged Resource for font/icon/stylebox"),
			},
			"required": []string{"resourcePath", "themeType", "itemType", "itemName", "value"},
		},
		OutputSchema: statusOutputSchema(map[string]any{
			"resourcePath": map[string]any{"type": "string"},
			"themeType":    map[string]any{"type": "string"},
			"itemType":     map[string]any{"type": "string"},
			"itemName":     map[string]any{"type": "string"},
		}),
		Handler: handleSetThemeItem,
	}
}

type setThemeItemArgs struct {
	ResourcePath string `json:"resourcePath"`
	ThemeType    string `json:"themeType"`
	ItemType     string `json:"itemType"`
	ItemName     string `json:"itemName"`
	Value        any    `json:"value"`
}

func handleSetThemeItem(ctx context.Context, raw json.RawMessage) (any, error) {
	var args setThemeItemArgs
	if len(raw) > 0 && string(raw) != "null" {
		if err := json.Unmarshal(raw, &args); err != nil {
			return nil, fmt.Errorf("invalid arguments: %w", err)
		}
	}
	project := projectRoot(ctx)
	if args.ResourcePath == "" || args.ThemeType == "" || args.ItemType == "" || args.ItemName == "" || args.Value == nil {
		return requiredArgsResult("resourcePath", "themeType", "itemType", "itemName", "value"), nil
	}
	if !validPath(project) || !validPath(args.ResourcePath) {
		return invalidPathResult(), nil
	}
	if !pathExists(filepath.Join(project, "project.godot")) {
		return notAProjectResult(project), nil
	}

	return runHeadlessOperation(ctx, project, "manage_theme", map[string]any{
		"resourcePath": args.ResourcePath,
		"action":       "setItem",
		"themeType":    args.ThemeType,
		"itemType":     args.ItemType,
		"itemName":     args.ItemName,
		"value":        args.Value,
	}, map[string]any{
		"resourcePath": args.ResourcePath,
		"themeType":    args.ThemeType,
		"itemType":     args.ItemType,
		"itemName":     args.ItemName,
	})
}
