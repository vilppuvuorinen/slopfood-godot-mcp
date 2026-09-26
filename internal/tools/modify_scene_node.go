package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"

	"github.com/vilppuvuorinen/slopfood-godot-mcp/internal/mcp"
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
				"scenePath": map[string]any{
					"type":        "string",
					"description": "Scene file path (relative to project)",
				},
				"nodePath": map[string]any{
					"type":        "string",
					"description": `Path to the node within the scene, relative to the scene root (e.g., "root/Player/Sprite2D")`,
				},
				"properties": propertiesSchema("Properties to set on the node, as name to godotValue pairs"),
			},
			"required": []string{"scenePath", "nodePath", "properties"},
		},
		OutputSchema: statusOutputSchema(map[string]any{
			"scenePath": map[string]any{"type": "string"},
			"nodePath":  map[string]any{"type": "string"},
		}),
		Handler: handleModifySceneNode,
	}
}

type modifySceneNodeArgs struct {
	ScenePath  string         `json:"scenePath"`
	NodePath   string         `json:"nodePath"`
	Properties map[string]any `json:"properties"`
}

func handleModifySceneNode(ctx context.Context, raw json.RawMessage) (any, error) {
	var args modifySceneNodeArgs
	if len(raw) > 0 && string(raw) != "null" {
		if err := json.Unmarshal(raw, &args); err != nil {
			return nil, fmt.Errorf("invalid arguments: %w", err)
		}
	}
	project := projectRoot(ctx)
	if args.ScenePath == "" || args.NodePath == "" || args.Properties == nil {
		return requiredArgsResult("scenePath", "nodePath", "properties"), nil
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
	return runHeadlessOperation(ctx, project, "modify_node", map[string]any{
		"scenePath":  args.ScenePath,
		"nodePath":   args.NodePath,
		"properties": args.Properties,
	}, map[string]any{
		"scenePath": args.ScenePath,
		"nodePath":  args.NodePath,
	})
}
