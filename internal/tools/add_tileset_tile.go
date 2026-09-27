package tools

import (
	"encoding/json"
	"fmt"
	"path/filepath"

	"github.com/vilppuvuorinen/slopfood-godot-mcp/internal/mcp"
)

func addTilesetTile() mcp.Tool {
	return mcp.Tool{
		Name:        "add_tileset_tile",
		Title:       "Add TileSet Tile",
		Description: "Add a tile to a TileSet source",
		Annotations: &mcp.ToolAnnotations{DestructiveHint: mcp.Bool(true), IdempotentHint: mcp.Bool(false), OpenWorldHint: mcp.Bool(false)},
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"resourcePath": map[string]any{
					"type":        "string",
					"description": "TileSet resource file path (relative to project)",
				},
				"sourceId": map[string]any{
					"type":        "integer",
					"description": "Source id to add the tile to",
				},
				"atlasCoords": godotValueSchema("Tile atlas coordinates as a Vector2i (atlas source only)"),
				"sizeInAtlas": godotValueSchema("Tile size in atlas cells as a Vector2i; defaults to 1x1 (atlas only)"),
				"scenePath": map[string]any{
					"type":        "string",
					"description": "PackedScene path to place as a scene tile (scenes source only)",
				},
				"sceneTileId": map[string]any{
					"type":        "integer",
					"description": "Scene tile id to assign; defaults to the engine's choice (scenes source only)",
				},
				"displayPlaceholder": map[string]any{
					"type":        "boolean",
					"description": "Whether a scene tile shows a placeholder in the editor (scenes source only)",
				},
			},
			"required": []string{"resourcePath", "sourceId"},
		},
		OutputSchema: statusOutputSchema(map[string]any{
			"resourcePath": map[string]any{"type": "string"},
			"sourceId":     map[string]any{"type": "integer"},
		}),
		Handler: withContext(handleAddTilesetTile),
	}
}

type addTilesetTileArgs struct {
	ResourcePath       string `json:"resourcePath"`
	SourceID           *int   `json:"sourceId"`
	AtlasCoords        any    `json:"atlasCoords"`
	SizeInAtlas        any    `json:"sizeInAtlas"`
	ScenePath          string `json:"scenePath"`
	SceneTileID        *int   `json:"sceneTileId"`
	DisplayPlaceholder *bool  `json:"displayPlaceholder"`
}

func handleAddTilesetTile(ctx Context, raw json.RawMessage) (any, error) {
	var args addTilesetTileArgs
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
		"action":       "addTile",
		"sourceId":     *args.SourceID,
	}
	if args.AtlasCoords != nil {
		params["atlasCoords"] = args.AtlasCoords
	}
	if args.SizeInAtlas != nil {
		params["sizeInAtlas"] = args.SizeInAtlas
	}
	if args.ScenePath != "" {
		params["scenePath"] = args.ScenePath
	}
	if args.SceneTileID != nil {
		params["sceneTileId"] = *args.SceneTileID
	}
	if args.DisplayPlaceholder != nil {
		params["displayPlaceholder"] = *args.DisplayPlaceholder
	}

	return runHeadlessOperation(ctx, project, "manage_tileset", params, map[string]any{
		"resourcePath": args.ResourcePath,
		"sourceId":     *args.SourceID,
	})
}
