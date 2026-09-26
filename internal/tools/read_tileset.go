package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"

	"github.com/vilppuvuorinen/slopfood-godot-mcp/internal/mcp"
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
		Handler: handleReadTileset,
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
