package tools

import (
	"encoding/json"
	"fmt"
	"path/filepath"

	"github.com/vilppuvuorinen/slopfood-godot-mcp/internal/mcp"
)

func clearThemeItem() mcp.Tool {
	return mcp.Tool{
		Name:        "clear_theme_item",
		Title:       "Clear Theme Item",
		Description: "Clear a Theme item from a type",
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
			},
			"required": []string{"resourcePath", "themeType", "itemType", "itemName"},
		},
		OutputSchema: statusOutputSchema(map[string]any{
			"resourcePath": map[string]any{"type": "string"},
			"themeType":    map[string]any{"type": "string"},
			"itemType":     map[string]any{"type": "string"},
			"itemName":     map[string]any{"type": "string"},
		}),
		Handler: withContext(handleClearThemeItem),
	}
}

type clearThemeItemArgs struct {
	ResourcePath string `json:"resourcePath"`
	ThemeType    string `json:"themeType"`
	ItemType     string `json:"itemType"`
	ItemName     string `json:"itemName"`
}

func handleClearThemeItem(ctx Context, raw json.RawMessage) (any, error) {
	var args clearThemeItemArgs
	if len(raw) > 0 && string(raw) != "null" {
		if err := json.Unmarshal(raw, &args); err != nil {
			return nil, fmt.Errorf("invalid arguments: %w", err)
		}
	}
	project := projectRoot(ctx)
	if args.ResourcePath == "" || args.ThemeType == "" || args.ItemType == "" || args.ItemName == "" {
		return requiredArgsResult("resourcePath", "themeType", "itemType", "itemName"), nil
	}
	if !validPath(project) || !validPath(args.ResourcePath) {
		return invalidPathResult(), nil
	}
	if !pathExists(filepath.Join(project, "project.godot")) {
		return notAProjectResult(project), nil
	}

	return runHeadlessOperation(ctx, project, "manage_theme", map[string]any{
		"resourcePath": args.ResourcePath,
		"action":       "clearItem",
		"themeType":    args.ThemeType,
		"itemType":     args.ItemType,
		"itemName":     args.ItemName,
	}, map[string]any{
		"resourcePath": args.ResourcePath,
		"themeType":    args.ThemeType,
		"itemType":     args.ItemType,
		"itemName":     args.ItemName,
	})
}
