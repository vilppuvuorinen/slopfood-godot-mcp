package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"

	"github.com/vilppuvuorinen/slopfood-godot-mcp/internal/mcp"
)

func modifyTileset() mcp.Tool {
	return mcp.Tool{
		Name:        "modify_tileset",
		Title:       "Modify TileSet",
		Description: "Set a TileSet resource's tile size, shape, layout, and UV clipping",
		Annotations: &mcp.ToolAnnotations{DestructiveHint: mcp.Bool(true), IdempotentHint: mcp.Bool(true), OpenWorldHint: mcp.Bool(false)},
		InputSchema: map[string]any{
			"type":       "object",
			"properties": withResourcePath(tilesetPropertiesSchema()),
			"required":   []string{"resourcePath"},
		},
		OutputSchema: statusOutputSchema(map[string]any{
			"resourcePath": map[string]any{"type": "string"},
		}),
		Handler: handleModifyTileset,
	}
}

type modifyTilesetArgs struct {
	ResourcePath   string `json:"resourcePath"`
	TileSize       any    `json:"tileSize"`
	TileShape      string `json:"tileShape"`
	TileLayout     string `json:"tileLayout"`
	TileOffsetAxis string `json:"tileOffsetAxis"`
	UVClipping     *bool  `json:"uvClipping"`
}

func handleModifyTileset(ctx context.Context, raw json.RawMessage) (any, error) {
	var args modifyTilesetArgs
	if len(raw) > 0 && string(raw) != "null" {
		if err := json.Unmarshal(raw, &args); err != nil {
			return nil, fmt.Errorf("invalid arguments: %w", err)
		}
	}
	project := projectRoot(ctx)
	if args.ResourcePath == "" {
		return requiredArgsResult("resourcePath"), nil
	}
	if args.TileSize == nil && args.TileShape == "" && args.TileLayout == "" && args.TileOffsetAxis == "" && args.UVClipping == nil {
		return requiredOneOfResult("modify_tileset", "tileSize", "tileShape", "tileLayout", "tileOffsetAxis", "uvClipping"), nil
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
	addTilesetProperties(params, args.TileSize, args.TileShape, args.TileLayout, args.TileOffsetAxis, args.UVClipping)

	return runHeadlessOperation(ctx, project, "manage_tileset", params, map[string]any{
		"resourcePath": args.ResourcePath,
	})
}
