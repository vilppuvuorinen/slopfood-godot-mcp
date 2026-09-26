package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"

	"github.com/vilppuvuorinen/slopfood-godot-mcp/internal/mcp"
)

func addSceneSignal() mcp.Tool {
	return mcp.Tool{
		Name:        "add_scene_signal",
		Title:       "Add Scene Signal",
		Description: "Add a signal connection in a .tscn scene",
		Annotations: &mcp.ToolAnnotations{DestructiveHint: mcp.Bool(true), IdempotentHint: mcp.Bool(true), OpenWorldHint: mcp.Bool(false)},
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"scenePath": map[string]any{
					"type":        "string",
					"description": "Scene file path (relative to project)",
				},
				"signalName": map[string]any{
					"type":        "string",
					"description": "Signal name",
				},
				"sourcePath": map[string]any{
					"type":        "string",
					"description": "Source node path, relative to the scene root",
				},
				"targetPath": map[string]any{
					"type":        "string",
					"description": "Target node path, relative to the scene root",
				},
				"method": map[string]any{
					"type":        "string",
					"description": "Target method name",
				},
			},
			"required": []string{"scenePath", "signalName", "sourcePath", "targetPath", "method"},
		},
		OutputSchema: statusOutputSchema(map[string]any{
			"scenePath":  map[string]any{"type": "string"},
			"signalName": map[string]any{"type": "string"},
			"sourcePath": map[string]any{"type": "string"},
			"targetPath": map[string]any{"type": "string"},
			"method":     map[string]any{"type": "string"},
		}),
		Handler: handleAddSceneSignal,
	}
}

type addSceneSignalArgs struct {
	ScenePath  string `json:"scenePath"`
	SignalName string `json:"signalName"`
	SourcePath string `json:"sourcePath"`
	TargetPath string `json:"targetPath"`
	Method     string `json:"method"`
}

func handleAddSceneSignal(ctx context.Context, raw json.RawMessage) (any, error) {
	var args addSceneSignalArgs
	if len(raw) > 0 && string(raw) != "null" {
		if err := json.Unmarshal(raw, &args); err != nil {
			return nil, fmt.Errorf("invalid arguments: %w", err)
		}
	}
	project := projectRoot(ctx)
	if args.ScenePath == "" || args.SignalName == "" || args.SourcePath == "" || args.TargetPath == "" || args.Method == "" {
		return requiredArgsResult("scenePath", "signalName", "sourcePath", "targetPath", "method"), nil
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

	return runHeadlessOperation(ctx, project, "manage_scene_signals", map[string]any{
		"scenePath":  args.ScenePath,
		"action":     "add",
		"signalName": args.SignalName,
		"sourcePath": args.SourcePath,
		"targetPath": args.TargetPath,
		"method":     args.Method,
	}, map[string]any{
		"scenePath":  args.ScenePath,
		"signalName": args.SignalName,
		"sourcePath": args.SourcePath,
		"targetPath": args.TargetPath,
		"method":     args.Method,
	})
}
