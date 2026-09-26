package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"

	"github.com/vilppuvuorinen/slopfood-godot-mcp/internal/mcp"
)

func eraseTilemapCell() mcp.Tool {
	return mcp.Tool{
		Name:        "erase_tilemap_cell",
		Title:       "Erase Tilemap Cell",
		Description: "Erase a tile from a TileMapLayer node in a scene",
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
				"coords": godotValueSchema("Cell coordinates as a Vector2i"),
			},
			"required": []string{"scenePath", "nodePath", "coords"},
		},
		OutputSchema: statusOutputSchema(map[string]any{
			"scenePath": map[string]any{"type": "string"},
			"nodePath":  map[string]any{"type": "string"},
		}),
		Handler: handleEraseTilemapCell,
	}
}

type eraseTilemapCellArgs struct {
	ScenePath string `json:"scenePath"`
	NodePath  string `json:"nodePath"`
	Coords    any    `json:"coords"`
}

func handleEraseTilemapCell(ctx context.Context, raw json.RawMessage) (any, error) {
	var args eraseTilemapCellArgs
	if len(raw) > 0 && string(raw) != "null" {
		if err := json.Unmarshal(raw, &args); err != nil {
			return nil, fmt.Errorf("invalid arguments: %w", err)
		}
	}
	project := projectRoot(ctx)
	if args.ScenePath == "" || args.NodePath == "" {
		return requiredArgsResult("scenePath", "nodePath"), nil
	}
	if args.Coords == nil {
		return requiredArgsResult("coords"), nil
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

	return runHeadlessOperation(ctx, project, "manage_tilemap", map[string]any{
		"scenePath": args.ScenePath,
		"nodePath":  args.NodePath,
		"action":    "eraseCell",
		"coords":    args.Coords,
	}, map[string]any{
		"scenePath": args.ScenePath,
		"nodePath":  args.NodePath,
	})
}
