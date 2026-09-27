package tools

import (
	"encoding/json"
	"fmt"
	"path/filepath"

	"github.com/vilppuvuorinen/slopfood-godot-mcp/internal/mcp"
)

func renameSceneNode() mcp.Tool {
	return mcp.Tool{
		Name:        "rename_scene_node",
		Title:       "Rename Scene Node",
		Description: "Rename a node in a .tscn scene",
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
				"newName": map[string]any{
					"type":        "string",
					"description": "New name for the node",
				},
			},
			"required": []string{"scenePath", "nodePath", "newName"},
		},
		OutputSchema: statusOutputSchema(map[string]any{
			"scenePath": map[string]any{"type": "string"},
			"nodePath":  map[string]any{"type": "string"},
			"newName":   map[string]any{"type": "string"},
		}),
		Handler: withContext(handleRenameSceneNode),
	}
}

type renameSceneNodeArgs struct {
	ScenePath string `json:"scenePath"`
	NodePath  string `json:"nodePath"`
	NewName   string `json:"newName"`
}

func handleRenameSceneNode(ctx Context, raw json.RawMessage) (any, error) {
	var args renameSceneNodeArgs
	if len(raw) > 0 && string(raw) != "null" {
		if err := json.Unmarshal(raw, &args); err != nil {
			return nil, fmt.Errorf("invalid arguments: %w", err)
		}
	}
	project := projectRoot(ctx)
	if args.ScenePath == "" || args.NodePath == "" || args.NewName == "" {
		return requiredArgsResult("scenePath", "nodePath", "newName"), nil
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
		"action":    "rename",
		"nodePath":  args.NodePath,
		"newName":   args.NewName,
	}, map[string]any{
		"scenePath": args.ScenePath,
		"nodePath":  args.NodePath,
		"newName":   args.NewName,
	})
}
