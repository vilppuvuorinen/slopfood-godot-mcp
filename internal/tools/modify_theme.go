package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"

	"github.com/vilppuvuorinen/slopfood-godot-mcp/internal/mcp"
)

func modifyTheme() mcp.Tool {
	return mcp.Tool{
		Name:        "modify_theme",
		Title:       "Modify Theme",
		Description: "Set a Theme's default font, font size, or base scale",
		Annotations: &mcp.ToolAnnotations{DestructiveHint: mcp.Bool(true), IdempotentHint: mcp.Bool(true), OpenWorldHint: mcp.Bool(false)},
		InputSchema: map[string]any{
			"type":       "object",
			"properties": withResourcePath(themeDefaultsSchema()),
			"required":   []string{"resourcePath"},
		},
		OutputSchema: statusOutputSchema(map[string]any{
			"resourcePath": map[string]any{"type": "string"},
		}),
		Handler: handleModifyTheme,
	}
}

type modifyThemeArgs struct {
	ResourcePath     string `json:"resourcePath"`
	DefaultFont      any    `json:"defaultFont"`
	DefaultFontSize  *int   `json:"defaultFontSize"`
	DefaultBaseScale any    `json:"defaultBaseScale"`
}

func handleModifyTheme(ctx context.Context, raw json.RawMessage) (any, error) {
	var args modifyThemeArgs
	if len(raw) > 0 && string(raw) != "null" {
		if err := json.Unmarshal(raw, &args); err != nil {
			return nil, fmt.Errorf("invalid arguments: %w", err)
		}
	}
	project := projectRoot(ctx)
	if args.ResourcePath == "" {
		return requiredArgsResult("resourcePath"), nil
	}
	if args.DefaultFont == nil && args.DefaultFontSize == nil && args.DefaultBaseScale == nil {
		return requiredOneOfResult("modify_theme", "defaultFont", "defaultFontSize", "defaultBaseScale"), nil
	}
	if !validPath(project) || !validPath(args.ResourcePath) {
		return invalidPathResult(), nil
	}
	if !pathExists(filepath.Join(project, "project.godot")) {
		return notAProjectResult(project), nil
	}

	params := map[string]any{
		"resourcePath": args.ResourcePath,
		"action":       "modify",
	}
	addThemeDefaults(params, args.DefaultFont, args.DefaultFontSize, args.DefaultBaseScale)

	return runHeadlessOperation(ctx, project, "manage_theme", params, map[string]any{
		"resourcePath": args.ResourcePath,
	})
}
