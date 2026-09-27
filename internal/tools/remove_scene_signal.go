package tools

import (
	"encoding/json"
	"fmt"
	"path/filepath"

	"github.com/vilppuvuorinen/slopfood-godot-mcp/internal/mcp"
)

func removeSceneSignal() mcp.Tool {
	return mcp.Tool{
		Name:        "remove_scene_signal",
		Title:       "Remove Scene Signal",
		Description: "Remove a signal connection from a .tscn scene",
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
					"description": "Optional target method name to narrow the match",
				},
			},
			"required": []string{"scenePath", "signalName", "sourcePath", "targetPath"},
		},
		OutputSchema: statusOutputSchema(map[string]any{
			"scenePath":  map[string]any{"type": "string"},
			"signalName": map[string]any{"type": "string"},
			"sourcePath": map[string]any{"type": "string"},
			"targetPath": map[string]any{"type": "string"},
			"method":     map[string]any{"type": "string"},
		}),
		Handler: withContext(handleRemoveSceneSignal),
	}
}

type removeSceneSignalArgs struct {
	ScenePath  string `json:"scenePath"`
	SignalName string `json:"signalName"`
	SourcePath string `json:"sourcePath"`
	TargetPath string `json:"targetPath"`
	Method     string `json:"method"`
}

func handleRemoveSceneSignal(ctx Context, raw json.RawMessage) (any, error) {
	var args removeSceneSignalArgs
	if len(raw) > 0 && string(raw) != "null" {
		if err := json.Unmarshal(raw, &args); err != nil {
			return nil, fmt.Errorf("invalid arguments: %w", err)
		}
	}
	project := projectRoot(ctx)
	if args.ScenePath == "" || args.SignalName == "" || args.SourcePath == "" || args.TargetPath == "" {
		return requiredArgsResult("scenePath", "signalName", "sourcePath", "targetPath"), nil
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

	params := map[string]any{
		"scenePath":  args.ScenePath,
		"action":     "remove",
		"signalName": args.SignalName,
		"sourcePath": args.SourcePath,
		"targetPath": args.TargetPath,
	}
	fields := map[string]any{
		"scenePath":  args.ScenePath,
		"signalName": args.SignalName,
		"sourcePath": args.SourcePath,
		"targetPath": args.TargetPath,
	}
	if args.Method != "" {
		params["method"] = args.Method
		fields["method"] = args.Method
	}
	return runHeadlessOperation(ctx, project, "manage_scene_signals", params, fields)
}
