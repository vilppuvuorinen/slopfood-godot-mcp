package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"

	"github.com/vilppuvuorinen/slopfood-godot-mcp/internal/mcp"
)

// tilesetShapeNames, tilesetLayoutNames, and tilesetOffsetAxisNames are the
// tool contract's names for the TileSet engine enums, shared with the bridge in
// godot_operations.gd.
var (
	tilesetShapeNames      = []string{"square", "isometric", "half_offset_square", "hexagon"}
	tilesetLayoutNames     = []string{"stacked", "stacked_offset", "stairs_right", "stairs_down", "diamond_right", "diamond_down"}
	tilesetOffsetAxisNames = []string{"horizontal", "vertical"}
)

// tilesetPropertiesSchema is the input schema shared by create_tileset and
// modify_tileset: the tileset-level settings both tools accept.
func tilesetPropertiesSchema() map[string]any {
	return map[string]any{
		"tileSize": godotValueSchema("Tile size in pixels as a Vector2i"),
		"tileShape": map[string]any{
			"type":        "string",
			"enum":        tilesetShapeNames,
			"description": "Tile shape",
		},
		"tileLayout": map[string]any{
			"type":        "string",
			"enum":        tilesetLayoutNames,
			"description": "Tile layout",
		},
		"tileOffsetAxis": map[string]any{
			"type":        "string",
			"enum":        tilesetOffsetAxisNames,
			"description": "Hexagon offset axis",
		},
		"uvClipping": map[string]any{
			"type":        "boolean",
			"description": "Whether UVs are clipped to the tile region",
		},
	}
}

func createTileset() mcp.Tool {
	return mcp.Tool{
		Name:        "create_tileset",
		Title:       "Create TileSet",
		Description: "Create a TileSet resource",
		Annotations: &mcp.ToolAnnotations{DestructiveHint: mcp.Bool(true), IdempotentHint: mcp.Bool(true), OpenWorldHint: mcp.Bool(false)},
		InputSchema: map[string]any{
			"type":       "object",
			"properties": withResourcePath(tilesetPropertiesSchema()),
			"required":   []string{"resourcePath"},
		},
		OutputSchema: statusOutputSchema(map[string]any{
			"resourcePath": map[string]any{"type": "string"},
		}),
		Handler: handleCreateTileset,
	}
}

type createTilesetArgs struct {
	ResourcePath   string `json:"resourcePath"`
	TileSize       any    `json:"tileSize"`
	TileShape      string `json:"tileShape"`
	TileLayout     string `json:"tileLayout"`
	TileOffsetAxis string `json:"tileOffsetAxis"`
	UVClipping     *bool  `json:"uvClipping"`
}

func handleCreateTileset(ctx context.Context, raw json.RawMessage) (any, error) {
	var args createTilesetArgs
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
	addTilesetProperties(params, args.TileSize, args.TileShape, args.TileLayout, args.TileOffsetAxis, args.UVClipping)

	return runHeadlessOperation(ctx, project, "manage_tileset", params, map[string]any{
		"resourcePath": args.ResourcePath,
	})
}

// addTilesetProperties copies the tileset-level fields onto params when present.
func addTilesetProperties(params map[string]any, tileSize any, tileShape, tileLayout, tileOffsetAxis string, uvClipping *bool) {
	if tileSize != nil {
		params["tileSize"] = tileSize
	}
	if tileShape != "" {
		params["tileShape"] = tileShape
	}
	if tileLayout != "" {
		params["tileLayout"] = tileLayout
	}
	if tileOffsetAxis != "" {
		params["tileOffsetAxis"] = tileOffsetAxis
	}
	if uvClipping != nil {
		params["uvClipping"] = *uvClipping
	}
}
