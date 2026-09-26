package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"

	"github.com/vilppuvuorinen/slopfood-godot-mcp/internal/mcp"
)

func readSceneSignals() mcp.Tool {
	return mcp.Tool{
		Name:        "read_scene_signals",
		Title:       "Read Scene Signals",
		Description: "List signal connections in .tscn files",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: mcp.Bool(true), OpenWorldHint: mcp.Bool(false)},
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"scenePath": map[string]any{
					"type":        "string",
					"description": "Scene file path (relative to project)",
				},
			},
			"required": []string{"scenePath"},
		},
		OutputSchema: payloadOutputSchema(map[string]any{
			"connections": map[string]any{
				"type": "array",
				"items": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"signal": map[string]any{"type": "string"},
						"from":   map[string]any{"type": "string"},
						"to":     map[string]any{"type": "string"},
						"method": map[string]any{"type": "string"},
					},
				},
			},
		}),
		Handler: handleReadSceneSignals,
	}
}

type readSceneSignalsArgs struct {
	ScenePath string `json:"scenePath"`
}

func handleReadSceneSignals(ctx context.Context, raw json.RawMessage) (any, error) {
	var args readSceneSignalsArgs
	if len(raw) > 0 && string(raw) != "null" {
		if err := json.Unmarshal(raw, &args); err != nil {
			return nil, fmt.Errorf("invalid arguments: %w", err)
		}
	}
	project := projectRoot(ctx)
	if args.ScenePath == "" {
		return requiredArgsResult("scenePath"), nil
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
		"action":    "list",
	}
	fields := map[string]any{
		"scenePath": args.ScenePath,
		"action":    "list",
	}
	return runHeadlessOperation(ctx, project, "manage_scene_signals", params, fields)
}
