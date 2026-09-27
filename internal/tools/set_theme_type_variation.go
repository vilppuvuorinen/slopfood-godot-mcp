package tools

import (
	"encoding/json"
	"fmt"
	"path/filepath"

	"github.com/vilppuvuorinen/slopfood-godot-mcp/internal/mcp"
)

func setThemeTypeVariation() mcp.Tool {
	return mcp.Tool{
		Name:        "set_theme_type_variation",
		Title:       "Set Theme Type Variation",
		Description: "Set a Theme type variation and its base type",
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
					"description": "Theme type variation such as FlatButton",
				},
				"baseType": map[string]any{
					"type":        "string",
					"description": "Base type the variation derives from",
				},
			},
			"required": []string{"resourcePath", "themeType", "baseType"},
		},
		OutputSchema: statusOutputSchema(map[string]any{
			"resourcePath": map[string]any{"type": "string"},
			"themeType":    map[string]any{"type": "string"},
			"baseType":     map[string]any{"type": "string"},
		}),
		Handler: withContext(handleSetThemeTypeVariation),
	}
}

type setThemeTypeVariationArgs struct {
	ResourcePath string `json:"resourcePath"`
	ThemeType    string `json:"themeType"`
	BaseType     string `json:"baseType"`
}

func handleSetThemeTypeVariation(ctx Context, raw json.RawMessage) (any, error) {
	var args setThemeTypeVariationArgs
	if len(raw) > 0 && string(raw) != "null" {
		if err := json.Unmarshal(raw, &args); err != nil {
			return nil, fmt.Errorf("invalid arguments: %w", err)
		}
	}
	project := projectRoot(ctx)
	if args.ResourcePath == "" || args.ThemeType == "" || args.BaseType == "" {
		return requiredArgsResult("resourcePath", "themeType", "baseType"), nil
	}
	if !validPath(project) || !validPath(args.ResourcePath) {
		return invalidPathResult(), nil
	}
	if !pathExists(filepath.Join(project, "project.godot")) {
		return notAProjectResult(project), nil
	}

	return runHeadlessOperation(ctx, project, "manage_theme", map[string]any{
		"resourcePath": args.ResourcePath,
		"action":       "setTypeVariation",
		"themeType":    args.ThemeType,
		"baseType":     args.BaseType,
	}, map[string]any{
		"resourcePath": args.ResourcePath,
		"themeType":    args.ThemeType,
		"baseType":     args.BaseType,
	})
}
