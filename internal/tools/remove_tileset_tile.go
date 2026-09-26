package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"

	"github.com/vilppuvuorinen/slopfood-godot-mcp/internal/mcp"
)

func removeTilesetTile() mcp.Tool {
	return mcp.Tool{
		Name:        "remove_tileset_tile",
		Title:       "Remove TileSet Tile",
		Description: "Remove a tile from a TileSet source",
		Annotations: &mcp.ToolAnnotations{DestructiveHint: mcp.Bool(true), IdempotentHint: mcp.Bool(true), OpenWorldHint: mcp.Bool(false)},
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"resourcePath": map[string]any{
					"type":        "string",
					"description": "TileSet resource file path (relative to project)",
				},
				"sourceId": map[string]any{
					"type":        "integer",
					"description": "Source id to remove the tile from",
				},
				"atlasCoords": godotValueSchema("Tile atlas coordinates as a Vector2i (atlas source only)"),
				"sceneTileId": map[string]any{
					"type":        "integer",
					"description": "Scene tile id to remove (scenes source only)",
				},
			},
			"required": []string{"resourcePath", "sourceId"},
		},
		OutputSchema: statusOutputSchema(map[string]any{
			"resourcePath": map[string]any{"type": "string"},
			"sourceId":     map[string]any{"type": "integer"},
		}),
		Handler: handleRemoveTilesetTile,
	}
}

type removeTilesetTileArgs struct {
	ResourcePath string `json:"resourcePath"`
	SourceID     *int   `json:"sourceId"`
	AtlasCoords  any    `json:"atlasCoords"`
	SceneTileID  *int   `json:"sceneTileId"`
}

func handleRemoveTilesetTile(ctx context.Context, raw json.RawMessage) (any, error) {
	var args removeTilesetTileArgs
	if len(raw) > 0 && string(raw) != "null" {
		if err := json.Unmarshal(raw, &args); err != nil {
			return nil, fmt.Errorf("invalid arguments: %w", err)
		}
	}
	project := projectRoot(ctx)
	if args.ResourcePath == "" || args.SourceID == nil {
		return requiredArgsResult("resourcePath", "sourceId"), nil
	}
	if !validPath(project) || !validPath(args.ResourcePath) {
		return invalidPathResult(), nil
	}
	if !pathExists(filepath.Join(project, "project.godot")) {
		return notAProjectResult(project), nil
	}

	params := map[string]any{
		"resourcePath": args.ResourcePath,
		"action":       "removeTile",
		"sourceId":     *args.SourceID,
	}
	if args.AtlasCoords != nil {
		params["atlasCoords"] = args.AtlasCoords
	}
	if args.SceneTileID != nil {
		params["sceneTileId"] = *args.SceneTileID
	}

	return runHeadlessOperation(ctx, project, "manage_tileset", params, map[string]any{
		"resourcePath": args.ResourcePath,
		"sourceId":     *args.SourceID,
	})
}
