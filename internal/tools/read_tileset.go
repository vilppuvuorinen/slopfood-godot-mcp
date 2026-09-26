package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"

	"github.com/vilppuvuorinen/go-dot-mcp/internal/mcp"
)

func readTileset() mcp.Tool {
	return mcp.Tool{
		Name:        "read_tileset",
		Title:       "Read TileSet",
		Description: "Read TileSet resources and their sources and tiles",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: mcp.Bool(true), OpenWorldHint: mcp.Bool(false)},
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"resourcePath": map[string]any{
					"type":        "string",
					"description": "TileSet resource file path (relative to project)",
				},
			},
			"required": []string{"resourcePath"},
		},
		OutputSchema: tilesetPayloadSchema(),
		Handler:      handleReadTileset,
	}
}

// tilesetPayloadSchema describes read_tileset's payload: the tileset-level
// settings plus one object per source.
func tilesetPayloadSchema() map[string]any {
	return payloadOutputSchema(map[string]any{
		"path":           map[string]any{"type": "string", "description": "TileSet resource path, relative to the bound project"},
		"tileSize":       godotValueSchema("Tile size in pixels as a Vector2i"),
		"tileShape":      map[string]any{"type": "string", "description": "Tile shape: square, isometric, half_offset_square, or hexagon"},
		"tileLayout":     map[string]any{"type": "string", "description": "Tile layout: stacked, stacked_offset, stairs_right, stairs_down, diamond_right, or diamond_down"},
		"tileOffsetAxis": map[string]any{"type": "string", "description": "Hexagon offset axis: horizontal or vertical"},
		"uvClipping":     map[string]any{"type": "boolean", "description": "Whether UVs are clipped to the tile region"},
		"sources":        map[string]any{"type": "array", "description": "TileSet sources in id order", "items": tilesetSourceSchema()},
	})
}

// tilesetSourceSchema describes one source. An atlas source carries its texture
// and atlas settings; a scenes-collection source does not. The oneOf branches
// pin type with const so exactly one shape matches.
func tilesetSourceSchema() map[string]any {
	atlas := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"id":                map[string]any{"type": "integer", "description": "Source id"},
			"type":              map[string]any{"const": "atlas"},
			"texture":           map[string]any{"type": "string", "description": "Atlas texture path, relative to the bound project (empty when the source has no texture)"},
			"margins":           godotValueSchema("Atlas margins as a Vector2i"),
			"separation":        godotValueSchema("Atlas tile separation as a Vector2i"),
			"textureRegionSize": godotValueSchema("Atlas texture region size as a Vector2i"),
			"useTexturePadding": map[string]any{"type": "boolean", "description": "Whether the atlas pads its texture"},
			"tiles":             map[string]any{"type": "array", "description": "Atlas tiles in creation order", "items": tilesetAtlasTileSchema()},
		},
		"required": []string{"id", "type", "texture", "margins", "separation", "textureRegionSize", "useTexturePadding", "tiles"},
	}
	scenes := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"id":    map[string]any{"type": "integer", "description": "Source id"},
			"type":  map[string]any{"const": "scenes"},
			"tiles": map[string]any{"type": "array", "description": "Scene tiles in creation order", "items": tilesetSceneTileSchema()},
		},
		"required": []string{"id", "type", "tiles"},
	}
	return map[string]any{
		"description": "One TileSet source",
		"oneOf":       []any{atlas, scenes},
	}
}

func tilesetAtlasTileSchema() map[string]any {
	return map[string]any{
		"type":        "object",
		"description": "One atlas tile",
		"properties": map[string]any{
			"atlasCoords":  godotValueSchema("Tile atlas coordinates as a Vector2i"),
			"sizeInAtlas":  godotValueSchema("Tile size in atlas cells as a Vector2i"),
			"alternatives": map[string]any{"type": "array", "description": "The tile's alternatives; id 0 is the base data", "items": tilesetAlternativeSchema()},
		},
		"required": []string{"atlasCoords", "sizeInAtlas", "alternatives"},
	}
}

func tilesetAlternativeSchema() map[string]any {
	return map[string]any{
		"type":        "object",
		"description": "One alternative of an atlas tile",
		"properties": map[string]any{
			"id": map[string]any{"type": "integer", "description": "Alternative tile id"},
			"data": map[string]any{
				"type":                 "object",
				"description":          "TileData storage properties in the tool value contract",
				"additionalProperties": godotValueSchema("A TileData property value"),
			},
		},
		"required": []string{"id", "data"},
	}
}

func tilesetSceneTileSchema() map[string]any {
	return map[string]any{
		"type":        "object",
		"description": "One scenes-collection tile",
		"properties": map[string]any{
			"sceneTileId":        map[string]any{"type": "integer", "description": "Scene tile id"},
			"scene":              map[string]any{"type": "string", "description": "PackedScene path, relative to the bound project"},
			"displayPlaceholder": map[string]any{"type": "boolean", "description": "Whether the tile shows a placeholder in the editor"},
		},
		"required": []string{"sceneTileId", "scene", "displayPlaceholder"},
	}
}

type readTilesetArgs struct {
	ResourcePath string `json:"resourcePath"`
}

func handleReadTileset(ctx context.Context, raw json.RawMessage) (any, error) {
	var args readTilesetArgs
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
		"action":       "read",
	}
	fields := map[string]any{
		"resourcePath": args.ResourcePath,
		"action":       "read",
	}
	return runHeadlessOperation(ctx, project, "manage_tileset", params, fields)
}
