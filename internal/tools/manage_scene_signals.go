package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"

	"github.com/vilppuvuorinen/go-dot-mcp/internal/mcp"
)

func manageSceneSignals() mcp.Tool {
	return mcp.Tool{
		Name:        "manage_scene_signals",
		Description: "List/add/remove signal connections in .tscn files",
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
					"description": "Action: list, add, remove",
				},
				"signalName": map[string]any{
					"type":        "string",
					"description": "Signal name",
				},
				"sourcePath": map[string]any{
					"type":        "string",
					"description": "Source node path",
				},
				"targetPath": map[string]any{
					"type":        "string",
					"description": "Target node path",
				},
				"method": map[string]any{
					"type":        "string",
					"description": "Target method name",
				},
			},
			"required": []string{"projectPath", "scenePath", "action"},
		},
		Handler: handleManageSceneSignals,
	}
}

type manageSceneSignalsArgs struct {
	ProjectPath string `json:"projectPath"`
	ScenePath   string `json:"scenePath"`
	Action      string `json:"action"`
	SignalName  string `json:"signalName"`
	SourcePath  string `json:"sourcePath"`
	TargetPath  string `json:"targetPath"`
	Method      string `json:"method"`
}

func handleManageSceneSignals(ctx context.Context, raw json.RawMessage) (any, error) {
	raw, err := aliasParams(raw, parameterMappings)
	if err != nil {
		return nil, err
	}
	var args manageSceneSignalsArgs
	if len(raw) > 0 && string(raw) != "null" {
		if err = json.Unmarshal(raw, &args); err != nil {
			return nil, fmt.Errorf("invalid arguments: %w", err)
		}
	}
	if args.ProjectPath == "" || args.ScenePath == "" || args.Action == "" {
		return mcp.ErrorResult("projectPath, scenePath, and action are required."), nil
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

	params := map[string]any{
		"scenePath": args.ScenePath,
		"action":    args.Action,
	}
	if args.SignalName != "" {
		params["signalName"] = args.SignalName
	}
	if args.SourcePath != "" {
		params["sourcePath"] = args.SourcePath
	}
	if args.TargetPath != "" {
		params["targetPath"] = args.TargetPath
	}
	if args.Method != "" {
		params["method"] = args.Method
	}

	return runHeadlessOperation(ctx, args.ProjectPath, "manage_scene_signals", params)
}
