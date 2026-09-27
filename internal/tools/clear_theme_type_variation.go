package tools

import (
	"encoding/json"
	"fmt"
	"path/filepath"

	"github.com/vilppuvuorinen/slopfood-godot-mcp/internal/mcp"
)

func clearThemeTypeVariation() mcp.Tool {
	return mcp.Tool{
		Name:        "clear_theme_type_variation",
		Title:       "Clear Theme Type Variation",
		Description: "Clear a Theme type variation",
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
					"description": "Theme type variation to clear",
				},
			},
			"required": []string{"resourcePath", "themeType"},
		},
		OutputSchema: statusOutputSchema(map[string]any{
			"resourcePath": map[string]any{"type": "string"},
			"themeType":    map[string]any{"type": "string"},
		}),
		Handler: withContext(handleClearThemeTypeVariation),
	}
}

type clearThemeTypeVariationArgs struct {
	ResourcePath string `json:"resourcePath"`
	ThemeType    string `json:"themeType"`
}

func handleClearThemeTypeVariation(ctx Context, raw json.RawMessage) (any, error) {
	var args clearThemeTypeVariationArgs
	if len(raw) > 0 && string(raw) != "null" {
		if err := json.Unmarshal(raw, &args); err != nil {
			return nil, fmt.Errorf("invalid arguments: %w", err)
		}
	}
	project := projectRoot(ctx)
	if args.ResourcePath == "" || args.ThemeType == "" {
		return requiredArgsResult("resourcePath", "themeType"), nil
	}
	if !validPath(project) || !validPath(args.ResourcePath) {
		return invalidPathResult(), nil
	}
	if !pathExists(filepath.Join(project, "project.godot")) {
		return notAProjectResult(project), nil
	}

	return runHeadlessOperation(ctx, project, "manage_theme", map[string]any{
		"resourcePath": args.ResourcePath,
		"action":       "clearTypeVariation",
		"themeType":    args.ThemeType,
	}, map[string]any{
		"resourcePath": args.ResourcePath,
		"themeType":    args.ThemeType,
	})
}
