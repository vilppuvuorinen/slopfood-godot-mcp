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
		Title:       "Manage Scene Signals",
		Description: "List/add/remove signal connections in .tscn files",
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
					"enum":        []string{"list", "add", "remove"},
					"description": "Action: list, add, remove",
				},
				"signalName": map[string]any{
					"type":        "string",
					"description": "Signal name (required for add and remove)",
				},
				"sourcePath": map[string]any{
					"type":        "string",
					"description": "Source node path (required for add and remove)",
				},
				"targetPath": map[string]any{
					"type":        "string",
					"description": "Target node path (required for add and remove)",
				},
				"method": map[string]any{
					"type":        "string",
					"description": "Target method name (required for add; optional filter for remove)",
				},
			},
			"required": []string{"scenePath", "action"},
		},
		OutputSchema: actionOutputSchema(
			map[string]any{
				"scenePath":  map[string]any{"type": "string"},
				"action":     map[string]any{"type": "string"},
				"signalName": map[string]any{"type": "string"},
				"sourcePath": map[string]any{"type": "string"},
				"targetPath": map[string]any{"type": "string"},
				"method":     map[string]any{"type": "string"},
			},
			payloadOutputSchema(map[string]any{
				"connections": map[string]any{
					"type": "array",
					"items": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"signal": map[string]any{"type": "string"},
							"from":   map[string]any{"type": "string"},
							"to":     map[string]any{"type": "string"},
							"method": map[string]any{"type": "string"},
						},
					},
				},
			}),
		),
		Handler: handleManageSceneSignals,
	}
}

type manageSceneSignalsArgs struct {
	ScenePath  string `json:"scenePath"`
	Action     string `json:"action"`
	SignalName string `json:"signalName"`
	SourcePath string `json:"sourcePath"`
	TargetPath string `json:"targetPath"`
	Method     string `json:"method"`
}

func handleManageSceneSignals(ctx context.Context, raw json.RawMessage) (any, error) {
	var args manageSceneSignalsArgs
	if len(raw) > 0 && string(raw) != "null" {
		if err := json.Unmarshal(raw, &args); err != nil {
			return nil, fmt.Errorf("invalid arguments: %w", err)
		}
	}
	project := projectRoot(ctx)
	if args.ScenePath == "" || args.Action == "" {
		return mcp.ErrorResult("scenePath and action are required."), nil
	}
	switch args.Action {
	case "list":
	case "add":
		if args.SignalName == "" || args.SourcePath == "" || args.TargetPath == "" || args.Method == "" {
			return mcp.ErrorResult("signalName, sourcePath, targetPath, and method are required for add."), nil
		}
	case "remove":
		if args.SignalName == "" || args.SourcePath == "" || args.TargetPath == "" {
			return mcp.ErrorResult("signalName, sourcePath, and targetPath are required for remove."), nil
		}
	default:
		return mcp.ErrorResult("Unknown action: " + args.Action + ". Use list, add, or remove."), nil
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

	fields := map[string]any{
		"scenePath": args.ScenePath,
		"action":    args.Action,
	}
	if args.SignalName != "" {
		fields["signalName"] = args.SignalName
	}
	if args.SourcePath != "" {
		fields["sourcePath"] = args.SourcePath
	}
	if args.TargetPath != "" {
		fields["targetPath"] = args.TargetPath
	}
	if args.Method != "" {
		fields["method"] = args.Method
	}
	return runHeadlessOperation(ctx, project, "manage_scene_signals", params, fields)
}
