package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"

	"github.com/vilppuvuorinen/go-dot-mcp/internal/mcp"
)

func manageSceneStructure() mcp.Tool {
	return mcp.Tool{
		Name:        "manage_scene_structure",
		Description: "Rename/duplicate/move nodes within .tscn scenes",
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
				"action": map[string]any{
					"type":        "string",
					"description": "Action: rename, duplicate, move",
				},
				"nodePath": map[string]any{
					"type":        "string",
					"description": "Source node path in scene",
				},
				"newName": map[string]any{
					"type":        "string",
					"description": "New name (for rename)",
				},
				"newParentPath": map[string]any{
					"type":        "string",
					"description": "New parent path (for move)",
				},
			},
			"required": []string{"projectPath", "scenePath", "action", "nodePath"},
		},
		Handler: handleManageSceneStructure,
	}
}

type manageSceneStructureArgs struct {
	ProjectPath   string `json:"projectPath"`
	ScenePath     string `json:"scenePath"`
	Action        string `json:"action"`
	NodePath      string `json:"nodePath"`
	NewName       string `json:"newName"`
	NewParentPath string `json:"newParentPath"`
}

func handleManageSceneStructure(ctx context.Context, raw json.RawMessage) (any, error) {
	raw, err := aliasParams(raw, parameterMappings)
	if err != nil {
		return nil, err
	}
	var args manageSceneStructureArgs
	if len(raw) > 0 && string(raw) != "null" {
		if err = json.Unmarshal(raw, &args); err != nil {
			return nil, fmt.Errorf("invalid arguments: %w", err)
		}
	}
	if args.ProjectPath == "" || args.ScenePath == "" || args.Action == "" || args.NodePath == "" {
		return mcp.ErrorResult(
			"projectPath, scenePath, action, and nodePath are required.",
			"Provide projectPath, scenePath, action, and nodePath",
		), nil
	}
	if !validPath(args.ProjectPath) || !validPath(args.ScenePath) {
		return mcp.ErrorResult(
			"Invalid path.",
			`Provide valid paths without ".." or other potentially unsafe characters`,
		), nil
	}
	if !pathExists(filepath.Join(args.ProjectPath, "project.godot")) {
		return notAProjectResult(args.ProjectPath), nil
	}
	if !pathExists(filepath.Join(args.ProjectPath, args.ScenePath)) {
		return sceneMissingResult(args.ScenePath), nil
	}

	params := map[string]any{
		"scenePath": args.ScenePath,
		"action":    args.Action,
		"nodePath":  args.NodePath,
	}
	if args.NewName != "" {
		params["newName"] = args.NewName
	}
	if args.NewParentPath != "" {
		params["newParentPath"] = args.NewParentPath
	}

	return runHeadlessOperation(ctx, args.ProjectPath, "manage_scene_structure", params)
}
