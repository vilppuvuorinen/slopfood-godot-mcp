package tools

import (
	"encoding/json"
	"fmt"
	"path/filepath"

	"github.com/vilppuvuorinen/slopfood-godot-mcp/internal/mcp"
)

func addTilesetAlternative() mcp.Tool {
	return mcp.Tool{
		Name:        "add_tileset_alternative",
		Title:       "Add TileSet Alternative",
		Description: "Add an alternative tile to a TileSet atlas tile",
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
					"description": "Atlas source id",
				},
				"atlasCoords": godotValueSchema("Tile atlas coordinates as a Vector2i"),
				"alternativeId": map[string]any{
					"type":        "integer",
					"description": "Alternative tile id to assign; defaults to the engine's choice",
				},
			},
			"required": []string{"resourcePath", "sourceId", "atlasCoords"},
		},
		OutputSchema: statusOutputSchema(map[string]any{
			"resourcePath": map[string]any{"type": "string"},
			"sourceId":     map[string]any{"type": "integer"},
		}),
		Handler: withContext(handleAddTilesetAlternative),
	}
}

type addTilesetAlternativeArgs struct {
	ResourcePath  string `json:"resourcePath"`
	SourceID      *int   `json:"sourceId"`
	AtlasCoords   any    `json:"atlasCoords"`
	AlternativeID *int   `json:"alternativeId"`
}

func handleAddTilesetAlternative(ctx Context, raw json.RawMessage) (any, error) {
	var args addTilesetAlternativeArgs
	if len(raw) > 0 && string(raw) != "null" {
		if err := json.Unmarshal(raw, &args); err != nil {
			return nil, fmt.Errorf("invalid arguments: %w", err)
		}
	}
	project := projectRoot(ctx)
	if args.ResourcePath == "" || args.SourceID == nil || args.AtlasCoords == nil {
		return requiredArgsResult("resourcePath", "sourceId", "atlasCoords"), nil
	}
	if !validPath(project) || !validPath(args.ResourcePath) {
		return invalidPathResult(), nil
	}
	if !pathExists(filepath.Join(project, "project.godot")) {
		return notAProjectResult(project), nil
	}

	params := map[string]any{
		"resourcePath": args.ResourcePath,
		"action":       "addAlternative",
		"sourceId":     *args.SourceID,
		"atlasCoords":  args.AtlasCoords,
	}
	if args.AlternativeID != nil {
		params["alternativeId"] = *args.AlternativeID
	}

	return runHeadlessOperation(ctx, project, "manage_tileset", params, map[string]any{
		"resourcePath": args.ResourcePath,
		"sourceId":     *args.SourceID,
	})
}
