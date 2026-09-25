package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"

	"github.com/vilppuvuorinen/go-dot-mcp/internal/mcp"
)

func removeSceneNode() mcp.Tool {
	return mcp.Tool{
		Name:        "remove_scene_node",
		Title:       "Remove Scene Node",
		Description: "Remove a node from a scene file (headless)",
		Annotations: &mcp.ToolAnnotations{DestructiveHint: mcp.Bool(true), OpenWorldHint: mcp.Bool(false)},
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"scenePath": map[string]any{
					"type":        "string",
					"description": "Scene file path (relative to project)",
				},
				"nodePath": map[string]any{
					"type":        "string",
					"description": `Path to the node to remove, relative to the scene root (e.g., "root/Player/OldNode")`,
				},
			},
			"required": []string{"scenePath", "nodePath"},
		},
		OutputSchema: statusOutputSchema(map[string]any{
			"scenePath": map[string]any{"type": "string"},
			"nodePath":  map[string]any{"type": "string"},
		}),
		Handler: handleRemoveSceneNode,
	}
}

type removeSceneNodeArgs struct {
	ScenePath string `json:"scenePath"`
	NodePath  string `json:"nodePath"`
}

func handleRemoveSceneNode(ctx context.Context, raw json.RawMessage) (any, error) {
	var args removeSceneNodeArgs
	if len(raw) > 0 && string(raw) != "null" {
		if err := json.Unmarshal(raw, &args); err != nil {
			return nil, fmt.Errorf("invalid arguments: %w", err)
		}
	}
	project := projectRoot(ctx)
	if args.ScenePath == "" || args.NodePath == "" {
		return mcp.ErrorResult("scenePath and nodePath are required."), nil
	}
	sceneFull, pathOK := confinePath(project, args.ScenePath)
	if !validPath(project) || !pathOK {
		return mcp.ErrorResult("Invalid path."), nil
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
	}
	return runHeadlessOperation(ctx, project, "remove_node", params, map[string]any{
		"scenePath": args.ScenePath,
		"nodePath":  args.NodePath,
	})
}
