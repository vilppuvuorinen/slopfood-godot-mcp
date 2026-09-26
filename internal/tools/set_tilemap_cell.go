package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"

	"github.com/vilppuvuorinen/go-dot-mcp/internal/mcp"
)

func setTilemapCell() mcp.Tool {
	return mcp.Tool{
		Name:        "set_tilemap_cell",
		Title:       "Set Tilemap Cell",
		Description: "Place a tile on a TileMapLayer node in a scene",
		Annotations: &mcp.ToolAnnotations{DestructiveHint: mcp.Bool(true), IdempotentHint: mcp.Bool(true), OpenWorldHint: mcp.Bool(false)},
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"scenePath": map[string]any{
					"type":        "string",
					"description": "Scene file path (relative to project)",
				},
				"nodePath": map[string]any{
					"type":        "string",
					"description": `Path to the TileMapLayer node within the scene, relative to the scene root (e.g., "root/Ground")`,
				},
				"coords":          godotValueSchema("Cell coordinates as a Vector2i"),
				"sourceId":        map[string]any{"type": "integer", "description": "TileSet source id to place"},
				"atlasCoords":     godotValueSchema("Tile atlas coordinates as a Vector2i"),
				"alternativeTile": map[string]any{"type": "integer", "description": "Alternative tile id to place (defaults to 0)"},
			},
			"required": []string{"scenePath", "nodePath", "coords", "sourceId", "atlasCoords"},
		},
		OutputSchema: statusOutputSchema(map[string]any{
			"scenePath": map[string]any{"type": "string"},
			"nodePath":  map[string]any{"type": "string"},
		}),
		Handler: handleSetTilemapCell,
	}
}

type setTilemapCellArgs struct {
	ScenePath       string `json:"scenePath"`
	NodePath        string `json:"nodePath"`
	Coords          any    `json:"coords"`
	SourceID        *int   `json:"sourceId"`
	AtlasCoords     any    `json:"atlasCoords"`
	AlternativeTile *int   `json:"alternativeTile"`
}

func handleSetTilemapCell(ctx context.Context, raw json.RawMessage) (any, error) {
	var args setTilemapCellArgs
	if len(raw) > 0 && string(raw) != "null" {
		if err := json.Unmarshal(raw, &args); err != nil {
			return nil, fmt.Errorf("invalid arguments: %w", err)
		}
	}
	project := projectRoot(ctx)
	if args.ScenePath == "" || args.NodePath == "" {
		return requiredArgsResult("scenePath", "nodePath"), nil
	}
	if args.Coords == nil || args.SourceID == nil || args.AtlasCoords == nil {
		return requiredArgsResult("coords", "sourceId", "atlasCoords"), nil
	}
	sceneFull, pathOK := confinePath(project, args.ScenePath)
	if !validPath(project) || !pathOK {
		return invalidPathResult(), nil
	}
	if !pathExists(filepath.Join(project, "project.godot")) {
		return notAProjectResult(project), nil
	}
	if !pathExists(sceneFull) {
		return sceneMissingResult(args.ScenePath), nil
	}

	params := map[string]any{
		"scenePath":   args.ScenePath,
		"nodePath":    args.NodePath,
		"action":      "setCell",
		"coords":      args.Coords,
		"sourceId":    *args.SourceID,
		"atlasCoords": args.AtlasCoords,
	}
	if args.AlternativeTile != nil {
		params["alternativeTile"] = *args.AlternativeTile
	}

	return runHeadlessOperation(ctx, project, "manage_tilemap", params, map[string]any{
		"scenePath": args.ScenePath,
		"nodePath":  args.NodePath,
	})
}
