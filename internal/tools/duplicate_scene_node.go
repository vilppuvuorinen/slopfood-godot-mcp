package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"

	"github.com/vilppuvuorinen/go-dot-mcp/internal/mcp"
)

func duplicateSceneNode() mcp.Tool {
	return mcp.Tool{
		Name:        "duplicate_scene_node",
		Title:       "Duplicate Scene Node",
		Description: "Duplicate a node within a .tscn scene",
		Annotations: &mcp.ToolAnnotations{DestructiveHint: mcp.Bool(true), IdempotentHint: mcp.Bool(false), OpenWorldHint: mcp.Bool(false)},
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"scenePath": map[string]any{
					"type":        "string",
					"description": "Scene file path (relative to project)",
				},
				"nodePath": map[string]any{
					"type":        "string",
					"description": "Source node path, relative to the scene root",
				},
			},
			"required": []string{"scenePath", "nodePath"},
		},
		OutputSchema: statusOutputSchema(map[string]any{
			"scenePath": map[string]any{"type": "string"},
			"nodePath":  map[string]any{"type": "string"},
		}),
		Handler: handleDuplicateSceneNode,
	}
}

type duplicateSceneNodeArgs struct {
	ScenePath string `json:"scenePath"`
	NodePath  string `json:"nodePath"`
}

func handleDuplicateSceneNode(ctx context.Context, raw json.RawMessage) (any, error) {
	var args duplicateSceneNodeArgs
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

	return runHeadlessOperation(ctx, project, "manage_scene_structure", map[string]any{
		"scenePath": args.ScenePath,
		"action":    "duplicate",
		"nodePath":  args.NodePath,
	}, map[string]any{
		"scenePath": args.ScenePath,
		"nodePath":  args.NodePath,
	})
}
