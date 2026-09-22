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
		Description: "Remove a node from a scene file (headless)",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"projectPath": map[string]any{
					"type":        "string",
					"description": "Godot project path",
				},
				"scenePath": map[string]any{
					"type":        "string",
					"description": "Scene file path (relative to project)",
				},
				"nodePath": map[string]any{
					"type":        "string",
					"description": `Path to the node to remove (e.g., "root/Player/OldNode")`,
				},
			},
			"required": []string{"projectPath", "scenePath", "nodePath"},
		},
		Handler: handleRemoveSceneNode,
	}
}

type removeSceneNodeArgs struct {
	ProjectPath string `json:"projectPath"`
	ScenePath   string `json:"scenePath"`
	NodePath    string `json:"nodePath"`
}

func handleRemoveSceneNode(ctx context.Context, raw json.RawMessage) (any, error) {
	raw, err := aliasParams(raw, parameterMappings)
	if err != nil {
		return nil, err
	}
	var args removeSceneNodeArgs
	if len(raw) > 0 && string(raw) != "null" {
		if err = json.Unmarshal(raw, &args); err != nil {
			return nil, fmt.Errorf("invalid arguments: %w", err)
		}
	}
	if args.ProjectPath == "" || args.ScenePath == "" || args.NodePath == "" {
		return mcp.ErrorResult("projectPath, scenePath, and nodePath are required."), nil
	}
	if !validPath(args.ProjectPath) || !validPath(args.ScenePath) {
		return mcp.ErrorResult("Invalid path."), nil
	}
	if !pathExists(filepath.Join(args.ProjectPath, "project.godot")) {
		return notAProjectResult(args.ProjectPath), nil
	}
	if !pathExists(filepath.Join(args.ProjectPath, args.ScenePath)) {
		return sceneMissingResult(args.ScenePath), nil
	}

	params := map[string]any{
		"scenePath": args.ScenePath,
		"nodePath":  args.NodePath,
	}
	return runHeadlessOperation(ctx, args.ProjectPath, "remove_node", params)
}
