package tools

import (
	"encoding/json"
	"fmt"
	"path/filepath"

	"github.com/vilppuvuorinen/slopfood-godot-mcp/internal/mcp"
)

func removeTilesetAlternative() mcp.Tool {
	return mcp.Tool{
		Name:        "remove_tileset_alternative",
		Title:       "Remove TileSet Alternative",
		Description: "Remove an alternative tile from a TileSet atlas tile",
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
					"description": "Alternative tile id to remove",
				},
			},
			"required": []string{"resourcePath", "sourceId", "atlasCoords", "alternativeId"},
		},
		OutputSchema: statusOutputSchema(map[string]any{
			"resourcePath": map[string]any{"type": "string"},
			"sourceId":     map[string]any{"type": "integer"},
		}),
		Handler: withContext(handleRemoveTilesetAlternative),
	}
}

type removeTilesetAlternativeArgs struct {
	ResourcePath  string `json:"resourcePath"`
	SourceID      *int   `json:"sourceId"`
	AtlasCoords   any    `json:"atlasCoords"`
	AlternativeID *int   `json:"alternativeId"`
}

func handleRemoveTilesetAlternative(ctx Context, raw json.RawMessage) (any, error) {
	var args removeTilesetAlternativeArgs
	if len(raw) > 0 && string(raw) != "null" {
		if err := json.Unmarshal(raw, &args); err != nil {
			return nil, fmt.Errorf("invalid arguments: %w", err)
		}
	}
	project := projectRoot(ctx)
	if args.ResourcePath == "" || args.SourceID == nil || args.AtlasCoords == nil || args.AlternativeID == nil {
		return requiredArgsResult("resourcePath", "sourceId", "atlasCoords", "alternativeId"), nil
	}
	if !validPath(project) || !validPath(args.ResourcePath) {
		return invalidPathResult(), nil
	}
	if !pathExists(filepath.Join(project, "project.godot")) {
		return notAProjectResult(project), nil
	}

	return runHeadlessOperation(ctx, project, "manage_tileset", map[string]any{
		"resourcePath":  args.ResourcePath,
		"action":        "removeAlternative",
		"sourceId":      *args.SourceID,
		"atlasCoords":   args.AtlasCoords,
		"alternativeId": *args.AlternativeID,
	}, map[string]any{
		"resourcePath": args.ResourcePath,
		"sourceId":     *args.SourceID,
	})
}
