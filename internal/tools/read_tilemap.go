package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"

	"github.com/vilppuvuorinen/go-dot-mcp/internal/mcp"
)

func readTilemap() mcp.Tool {
	return mcp.Tool{
		Name:        "read_tilemap",
		Title:       "Read TileMap",
		Description: "Read the cells of a TileMapLayer node in a scene",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: mcp.Bool(true), OpenWorldHint: mcp.Bool(false)},
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
			},
			"required": []string{"scenePath", "nodePath"},
		},
		OutputSchema: tilemapPayloadSchema(),
		Handler:      handleReadTilemap,
	}
}

// tilemapPayloadSchema describes read_tilemap's payload: the node's tileset and
// one object per placed cell.
func tilemapPayloadSchema() map[string]any {
	return payloadOutputSchema(map[string]any{
		"scenePath": map[string]any{"type": "string", "description": "Scene path, relative to the bound project"},
		"nodePath":  map[string]any{"type": "string", "description": "Node path, relative to the scene root"},
		"tileSet":   map[string]any{"type": "string", "description": "TileSet resource path, relative to the bound project (empty when unset)"},
		"usedCells": map[string]any{"type": "array", "description": "Placed cells, sorted by coordinates", "items": tilemapCellSchema()},
	})
}

func tilemapCellSchema() map[string]any {
	return map[string]any{
		"type":        "object",
		"description": "One placed cell",
		"properties": map[string]any{
			"coords":          godotValueSchema("Cell coordinates as a Vector2i"),
			"sourceId":        map[string]any{"type": "integer", "description": "TileSet source id"},
			"atlasCoords":     godotValueSchema("Tile atlas coordinates as a Vector2i"),
			"alternativeTile": map[string]any{"type": "integer", "description": "Alternative tile id"},
			"flippedH":        map[string]any{"type": "boolean", "description": "Whether the cell is flipped horizontally"},
			"flippedV":        map[string]any{"type": "boolean", "description": "Whether the cell is flipped vertically"},
			"transposed":      map[string]any{"type": "boolean", "description": "Whether the cell is transposed"},
		},
		"required": []string{"coords", "sourceId", "atlasCoords", "alternativeTile", "flippedH", "flippedV", "transposed"},
	}
}

type readTilemapArgs struct {
	ScenePath string `json:"scenePath"`
	NodePath  string `json:"nodePath"`
}

func handleReadTilemap(ctx context.Context, raw json.RawMessage) (any, error) {
	var args readTilemapArgs
	if len(raw) > 0 && string(raw) != "null" {
		if err := json.Unmarshal(raw, &args); err != nil {
			return nil, fmt.Errorf("invalid arguments: %w", err)
		}
	}
	project := projectRoot(ctx)
	if args.ScenePath == "" || args.NodePath == "" {
		return requiredArgsResult("scenePath", "nodePath"), nil
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
		"action":    "read",
	}
	fields := map[string]any{
		"scenePath": args.ScenePath,
		"nodePath":  args.NodePath,
		"action":    "read",
	}
	return runHeadlessOperation(ctx, project, "manage_tilemap", params, fields)
}
