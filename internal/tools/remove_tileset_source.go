package tools

import (
	"encoding/json"
	"fmt"
	"path/filepath"

	"github.com/vilppuvuorinen/slopfood-godot-mcp/internal/mcp"
)

func removeTilesetSource() mcp.Tool {
	return mcp.Tool{
		Name:        "remove_tileset_source",
		Title:       "Remove TileSet Source",
		Description: "Remove a source from a TileSet",
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
					"description": "Source id to remove",
				},
			},
			"required": []string{"resourcePath", "sourceId"},
		},
		OutputSchema: statusOutputSchema(map[string]any{
			"resourcePath": map[string]any{"type": "string"},
			"sourceId":     map[string]any{"type": "integer"},
		}),
		Handler: withContext(handleRemoveTilesetSource),
	}
}

type removeTilesetSourceArgs struct {
	ResourcePath string `json:"resourcePath"`
	SourceID     *int   `json:"sourceId"`
}

func handleRemoveTilesetSource(ctx Context, raw json.RawMessage) (any, error) {
	var args removeTilesetSourceArgs
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

	return runHeadlessOperation(ctx, project, "manage_tileset", map[string]any{
		"resourcePath": args.ResourcePath,
		"action":       "removeSource",
		"sourceId":     *args.SourceID,
	}, map[string]any{
		"resourcePath": args.ResourcePath,
		"sourceId":     *args.SourceID,
	})
}
