package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"

	"github.com/vilppuvuorinen/slopfood-godot-mcp/internal/mcp"
)

func removeThemeType() mcp.Tool {
	return mcp.Tool{
		Name:        "remove_theme_type",
		Title:       "Remove Theme Type",
		Description: "Remove a type from a Theme",
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
			},
			"required": []string{"resourcePath", "themeType"},
		},
		OutputSchema: statusOutputSchema(map[string]any{
			"resourcePath": map[string]any{"type": "string"},
			"themeType":    map[string]any{"type": "string"},
		}),
		Handler: handleRemoveThemeType,
	}
}

type removeThemeTypeArgs struct {
	ResourcePath string `json:"resourcePath"`
	ThemeType    string `json:"themeType"`
}

func handleRemoveThemeType(ctx context.Context, raw json.RawMessage) (any, error) {
	var args removeThemeTypeArgs
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
		"action":       "removeType",
		"themeType":    args.ThemeType,
	}, map[string]any{
		"resourcePath": args.ResourcePath,
		"themeType":    args.ThemeType,
	})
}
