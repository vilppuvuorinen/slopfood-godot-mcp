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
		Title:       "Manage Scene Structure",
		Description: "Rename/duplicate/move nodes within .tscn scenes",
		Annotations: &mcp.ToolAnnotations{DestructiveHint: mcp.Bool(true), OpenWorldHint: mcp.Bool(false)},
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"scenePath": map[string]any{
					"type":        "string",
					"description": "Scene file path (relative to project)",
				},
				"action": map[string]any{
					"type":        "string",
					"enum":        []string{"rename", "duplicate", "move"},
					"description": "Action: rename, duplicate, move",
				},
				"nodePath": map[string]any{
					"type":        "string",
					"description": "Source node path, relative to the scene root",
				},
				"newName": map[string]any{
					"type":        "string",
					"description": "New name (for rename)",
				},
				"newParentPath": map[string]any{
					"type":        "string",
					"description": "New parent path, relative to the scene root (for move)",
				},
			},
			"required": []string{"scenePath", "action", "nodePath"},
		},
		OutputSchema: statusOutputSchema(map[string]any{
			"scenePath":     map[string]any{"type": "string"},
			"action":        map[string]any{"type": "string"},
			"nodePath":      map[string]any{"type": "string"},
			"newName":       map[string]any{"type": "string"},
			"newParentPath": map[string]any{"type": "string"},
		}),
		Handler: handleManageSceneStructure,
	}
}

type manageSceneStructureArgs struct {
	ScenePath     string `json:"scenePath"`
	Action        string `json:"action"`
	NodePath      string `json:"nodePath"`
	NewName       string `json:"newName"`
	NewParentPath string `json:"newParentPath"`
}

func handleManageSceneStructure(ctx context.Context, raw json.RawMessage) (any, error) {
	var args manageSceneStructureArgs
	if len(raw) > 0 && string(raw) != "null" {
		if err := json.Unmarshal(raw, &args); err != nil {
			return nil, fmt.Errorf("invalid arguments: %w", err)
		}
	}
	project := projectRoot(ctx)
	if args.ScenePath == "" || args.Action == "" || args.NodePath == "" {
		return mcp.ErrorResult(
			"scenePath, action, and nodePath are required.",
			"Provide scenePath, action, and nodePath",
		), nil
	}
	sceneFull, pathOK := confinePath(project, args.ScenePath)
	if !validPath(project) || !pathOK {
		return mcp.ErrorResult(
			"Invalid path.",
			`Provide a valid scene path without ".." or other potentially unsafe characters`,
		), nil
	}
	if !pathExists(filepath.Join(project, "project.godot")) {
		return notAProjectResult(project), nil
	}
	if !pathExists(sceneFull) {
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

	fields := map[string]any{
		"scenePath": args.ScenePath,
		"action":    args.Action,
		"nodePath":  args.NodePath,
	}
	if args.NewName != "" {
		fields["newName"] = args.NewName
	}
	if args.NewParentPath != "" {
		fields["newParentPath"] = args.NewParentPath
	}
	return runHeadlessOperation(ctx, project, "manage_scene_structure", params, fields)
}
