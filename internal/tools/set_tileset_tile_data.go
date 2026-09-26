package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"

	"github.com/vilppuvuorinen/slopfood-godot-mcp/internal/mcp"
)

func setTilesetTileData() mcp.Tool {
	return mcp.Tool{
		Name:        "set_tileset_tile_data",
		Title:       "Set TileSet Tile Data",
		Description: "Set properties on a TileSet atlas tile's data",
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
					"description": "Atlas source id",
				},
				"atlasCoords": godotValueSchema("Tile atlas coordinates as a Vector2i"),
				"alternativeId": map[string]any{
					"type":        "integer",
					"description": "Alternative tile id; defaults to the base tile (0)",
				},
				"properties": propertiesSchema("TileData properties to set, as name to godotValue pairs"),
			},
			"required": []string{"resourcePath", "sourceId", "atlasCoords", "properties"},
		},
		OutputSchema: statusOutputSchema(map[string]any{
			"resourcePath": map[string]any{"type": "string"},
			"sourceId":     map[string]any{"type": "integer"},
		}),
		Handler: handleSetTilesetTileData,
	}
}

type setTilesetTileDataArgs struct {
	ResourcePath  string         `json:"resourcePath"`
	SourceID      *int           `json:"sourceId"`
	AtlasCoords   any            `json:"atlasCoords"`
	AlternativeID *int           `json:"alternativeId"`
	Properties    map[string]any `json:"properties"`
}

func handleSetTilesetTileData(ctx context.Context, raw json.RawMessage) (any, error) {
	var args setTilesetTileDataArgs
	if len(raw) > 0 && string(raw) != "null" {
		if err := json.Unmarshal(raw, &args); err != nil {
			return nil, fmt.Errorf("invalid arguments: %w", err)
		}
	}
	project := projectRoot(ctx)
	if args.ResourcePath == "" || args.SourceID == nil || args.AtlasCoords == nil || len(args.Properties) == 0 {
		return requiredArgsResult("resourcePath", "sourceId", "atlasCoords", "properties"), nil
	}
	if !validPath(project) || !validPath(args.ResourcePath) {
		return invalidPathResult(), nil
	}
	if !pathExists(filepath.Join(project, "project.godot")) {
		return notAProjectResult(project), nil
	}

	params := map[string]any{
		"resourcePath": args.ResourcePath,
		"action":       "setTileData",
		"sourceId":     *args.SourceID,
		"atlasCoords":  args.AtlasCoords,
		"properties":   args.Properties,
	}
	if args.AlternativeID != nil {
		params["alternativeId"] = *args.AlternativeID
	}

	return runHeadlessOperation(ctx, project, "manage_tileset", params, map[string]any{
		"resourcePath": args.ResourcePath,
		"sourceId":     *args.SourceID,
	})
}
