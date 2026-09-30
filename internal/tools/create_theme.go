package tools

import (
	"encoding/json"
	"fmt"
	"path/filepath"

	"github.com/vilppuvuorinen/slopfood-godot-mcp/internal/mcp"
)

func themeDefaultsSchema() map[string]any {
	return map[string]any{
		"defaultFont":      godotValueSchema("Default font as a res:// path or tagged Resource"),
		"defaultFontSize":  map[string]any{"type": "integer", "description": "Default font size in pixels"},
		"defaultBaseScale": map[string]any{"type": "number", "description": "Default base scale for scale-aware themes"},
	}
}

func createTheme() mcp.Tool {
	return mcp.Tool{
		Name:        "create_theme",
		Title:       "Create Theme",
		Description: "Create a Theme resource",
		Annotations: &mcp.ToolAnnotations{DestructiveHint: mcp.Bool(true), IdempotentHint: mcp.Bool(true), OpenWorldHint: mcp.Bool(false)},
		InputSchema: map[string]any{
			"type":       "object",
			"properties": withResourcePath(themeDefaultsSchema()),
			"required":   []string{"resourcePath"},
		},
		OutputSchema: statusOutputSchema(map[string]any{
			"resourcePath": map[string]any{"type": "string"},
		}),
		Handler: withContext(handleCreateTheme),
	}
}

type createThemeArgs struct {
	ResourcePath     string `json:"resourcePath"`
	DefaultFont      any    `json:"defaultFont"`
	DefaultFontSize  *int   `json:"defaultFontSize"`
	DefaultBaseScale any    `json:"defaultBaseScale"`
}

func handleCreateTheme(ctx Context, raw json.RawMessage) (any, error) {
	var args createThemeArgs
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
		"action":       "create",
	}
	addThemeDefaults(params, args.DefaultFont, args.DefaultFontSize, args.DefaultBaseScale)

	return runHeadlessOperation(ctx, project, "manage_theme", params, map[string]any{
		"resourcePath": args.ResourcePath,
	})
}

func addThemeDefaults(params map[string]any, defaultFont any, defaultFontSize *int, defaultBaseScale any) {
	if defaultFont != nil {
		params["defaultFont"] = defaultFont
	}
	if defaultFontSize != nil {
		params["defaultFontSize"] = *defaultFontSize
	}
	if defaultBaseScale != nil {
		params["defaultBaseScale"] = defaultBaseScale
	}
}
