package tools

import (
	"encoding/json"
	"fmt"
	"path/filepath"

	"github.com/vilppuvuorinen/slopfood-godot-mcp/internal/mcp"
)

func moveSceneNode() mcp.Tool {
	return mcp.Tool{
		Name:        "move_scene_node",
		Title:       "Move Scene Node",
		Description: "Move a node to a new parent in a .tscn scene",
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
					"description": "Source node path, relative to the scene root",
				},
				"newParentPath": map[string]any{
					"type":        "string",
					"description": "New parent path, relative to the scene root",
				},
			},
			"required": []string{"scenePath", "nodePath", "newParentPath"},
		},
		OutputSchema: statusOutputSchema(map[string]any{
			"scenePath":     map[string]any{"type": "string"},
			"nodePath":      map[string]any{"type": "string"},
			"newParentPath": map[string]any{"type": "string"},
		}),
		Handler: withContext(handleMoveSceneNode),
	}
}

type moveSceneNodeArgs struct {
	ScenePath     string `json:"scenePath"`
	NodePath      string `json:"nodePath"`
	NewParentPath string `json:"newParentPath"`
}

func handleMoveSceneNode(ctx Context, raw json.RawMessage) (any, error) {
	var args moveSceneNodeArgs
	if len(raw) > 0 && string(raw) != "null" {
		if err := json.Unmarshal(raw, &args); err != nil {
			return nil, fmt.Errorf("invalid arguments: %w", err)
		}
	}
	project := projectRoot(ctx)
	if args.ScenePath == "" || args.NodePath == "" || args.NewParentPath == "" {
		return requiredArgsResult("scenePath", "nodePath", "newParentPath"), nil
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
		"scenePath":     args.ScenePath,
		"action":        "move",
		"nodePath":      args.NodePath,
		"newParentPath": args.NewParentPath,
	}, map[string]any{
		"scenePath":     args.ScenePath,
		"nodePath":      args.NodePath,
		"newParentPath": args.NewParentPath,
	})
}
