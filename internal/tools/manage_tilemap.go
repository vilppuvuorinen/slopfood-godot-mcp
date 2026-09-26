package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"

	"github.com/vilppuvuorinen/go-dot-mcp/internal/mcp"
)

func manageTilemap() mcp.Tool {
	return mcp.Tool{
		Name:        "manage_tilemap",
		Title:       "Manage TileMap",
		Description: "Place, erase, or clear cells on a TileMapLayer node in a scene",
		Annotations: &mcp.ToolAnnotations{DestructiveHint: mcp.Bool(true), OpenWorldHint: mcp.Bool(false)},
		InputSchema: actionInputSchema(
			map[string]any{
				"scenePath": map[string]any{
					"type":        "string",
					"description": "Scene file path (relative to project)",
				},
				"nodePath": map[string]any{
					"type":        "string",
					"description": `Path to the TileMapLayer node within the scene, relative to the scene root (e.g., "root/Ground")`,
				},
				"action": map[string]any{
					"type":        "string",
					"enum":        []string{"setCell", "eraseCell", "clear"},
					"description": "Action: setCell places a tile, eraseCell removes one, clear removes every cell",
				},
				"coords":          godotValueSchema("Cell coordinates as a Vector2i (setCell, eraseCell)"),
				"sourceId":        map[string]any{"type": "integer", "description": "TileSet source id to place (setCell)"},
				"atlasCoords":     godotValueSchema("Tile atlas coordinates as a Vector2i (setCell)"),
				"alternativeTile": map[string]any{"type": "integer", "description": "Alternative tile id to place (setCell; defaults to 0)"},
			},
			[]string{"scenePath", "nodePath", "action"},
			[]actionVariant{
				{Action: "setCell", Required: []string{"coords", "sourceId", "atlasCoords"}},
				{Action: "eraseCell", Required: []string{"coords"}},
				{Action: "clear"},
			},
		),
		OutputSchema: statusOutputSchema(map[string]any{
			"scenePath": map[string]any{"type": "string"},
			"nodePath":  map[string]any{"type": "string"},
			"action":    map[string]any{"type": "string"},
		}),
		Handler: handleManageTilemap,
	}
}

type manageTilemapArgs struct {
	ScenePath       string `json:"scenePath"`
	NodePath        string `json:"nodePath"`
	Action          string `json:"action"`
	Coords          any    `json:"coords"`
	SourceID        *int   `json:"sourceId"`
	AtlasCoords     any    `json:"atlasCoords"`
	AlternativeTile *int   `json:"alternativeTile"`
}

func handleManageTilemap(ctx context.Context, raw json.RawMessage) (any, error) {
	var args manageTilemapArgs
	if len(raw) > 0 && string(raw) != "null" {
		if err := json.Unmarshal(raw, &args); err != nil {
			return nil, fmt.Errorf("invalid arguments: %w", err)
		}
	}
	project := projectRoot(ctx)
	if args.ScenePath == "" || args.NodePath == "" || args.Action == "" {
		return requiredArgsResult("scenePath", "nodePath", "action"), nil
	}
	if result, ok := validateTilemapAction(args); !ok {
		return result, nil
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
		"scenePath": args.ScenePath,
		"nodePath":  args.NodePath,
		"action":    args.Action,
	}
	if args.Coords != nil {
		params["coords"] = args.Coords
	}
	if args.SourceID != nil {
		params["sourceId"] = *args.SourceID
	}
	if args.AtlasCoords != nil {
		params["atlasCoords"] = args.AtlasCoords
	}
	if args.AlternativeTile != nil {
		params["alternativeTile"] = *args.AlternativeTile
	}

	return runHeadlessOperation(ctx, project, "manage_tilemap", params, map[string]any{
		"scenePath": args.ScenePath,
		"nodePath":  args.NodePath,
		"action":    args.Action,
	})
}

// validateTilemapAction enforces each action's own required fields. The input
// schema's oneOf advertises them; this is the server-side guarantee.
func validateTilemapAction(args manageTilemapArgs) (mcp.ToolResult, bool) {
	missing := func(field string) (mcp.ToolResult, bool) {
		return mcp.ErrorResult(field + " is required for the " + args.Action + " action."), false
	}
	switch args.Action {
	case "clear":
		return mcp.ToolResult{}, true
	case "eraseCell":
		if args.Coords == nil {
			return missing("coords")
		}
	case "setCell":
		if args.Coords == nil {
			return missing("coords")
		}
		if args.SourceID == nil {
			return missing("sourceId")
		}
		if args.AtlasCoords == nil {
			return missing("atlasCoords")
		}
	default:
		return mcp.ErrorResult("Unknown action: " + args.Action + ". Use setCell, eraseCell, or clear."), false
	}
	return mcp.ToolResult{}, true
}
