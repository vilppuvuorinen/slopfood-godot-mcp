package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"

	"github.com/vilppuvuorinen/go-dot-mcp/internal/mcp"
)

func modifySceneNode() mcp.Tool {
	return mcp.Tool{
		Name:        "modify_scene_node",
		Title:       "Modify Scene Node",
		Description: "Modify node properties in a scene file (headless)",
		Annotations: &mcp.ToolAnnotations{DestructiveHint: mcp.Bool(true), IdempotentHint: mcp.Bool(true), OpenWorldHint: mcp.Bool(false)},
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
					"description": `Path to the node within the scene (e.g., "root/Player/Sprite2D")`,
				},
				"properties": map[string]any{
					"type":        "object",
					"description": "Properties to set on the node as key-value pairs",
				},
			},
			"required": []string{"projectPath", "scenePath", "nodePath", "properties"},
		},
		OutputSchema: statusOutputSchema(map[string]any{
			"scenePath": map[string]any{"type": "string"},
			"nodePath":  map[string]any{"type": "string"},
		}),
		Handler: handleModifySceneNode,
	}
}

type modifySceneNodeArgs struct {
	ProjectPath string         `json:"projectPath"`
	ScenePath   string         `json:"scenePath"`
	NodePath    string         `json:"nodePath"`
	Properties  map[string]any `json:"properties"`
}

func handleModifySceneNode(ctx context.Context, raw json.RawMessage) (any, error) {
	raw, err := aliasParams(raw, parameterMappings)
	if err != nil {
		return nil, err
	}
	var args modifySceneNodeArgs
	if len(raw) > 0 && string(raw) != "null" {
		if err = json.Unmarshal(raw, &args); err != nil {
			return nil, fmt.Errorf("invalid arguments: %w", err)
		}
	}
	if args.ProjectPath == "" || args.ScenePath == "" || args.NodePath == "" || args.Properties == nil {
		return mcp.ErrorResult("projectPath, scenePath, nodePath, and properties are required."), nil
	}
	sceneFull, pathOK := confinePath(args.ProjectPath, args.ScenePath)
	if !validPath(args.ProjectPath) || !pathOK {
		return mcp.ErrorResult("Invalid path."), nil
	}
	if !pathExists(filepath.Join(args.ProjectPath, "project.godot")) {
		return notAProjectResult(args.ProjectPath), nil
	}
	if !pathExists(sceneFull) {
		return sceneMissingResult(args.ScenePath), nil
	}
	return runHeadlessOperation(ctx, args.ProjectPath, "modify_node", map[string]any{
		"scenePath":  args.ScenePath,
		"nodePath":   args.NodePath,
		"properties": args.Properties,
	}, map[string]any{
		"scenePath": args.ScenePath,
		"nodePath":  args.NodePath,
	})
}
