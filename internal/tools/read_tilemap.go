package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"

	"github.com/vilppuvuorinen/slopfood-godot-mcp/internal/mcp"
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
		Handler: handleReadTilemap,
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
