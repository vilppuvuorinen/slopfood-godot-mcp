package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"

	"github.com/vilppuvuorinen/go-dot-mcp/internal/mcp"
)

func saveScene() mcp.Tool {
	return mcp.Tool{
		Name:        "save_scene",
		Title:       "Save Scene",
		Description: "Save changes to a scene file",
		Annotations: &mcp.ToolAnnotations{DestructiveHint: mcp.Bool(true), OpenWorldHint: mcp.Bool(false)},
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"projectPath": map[string]any{
					"type":        "string",
					"description": "Path to the Godot project directory",
				},
				"scenePath": map[string]any{
					"type":        "string",
					"description": "Path to the scene file (relative to project)",
				},
				"newPath": map[string]any{
					"type":        "string",
					"description": "Optional: New path to save the scene to (for creating variants)",
				},
			},
			"required": []string{"projectPath", "scenePath"},
		},
		OutputSchema: statusOutputSchema(map[string]any{
			"scenePath": map[string]any{"type": "string"},
			"newPath":   map[string]any{"type": "string"},
			"output":    map[string]any{"type": "string"},
		}),
		Handler: handleSaveScene,
	}
}

type saveSceneArgs struct {
	ProjectPath string `json:"projectPath"`
	ScenePath   string `json:"scenePath"`
	NewPath     string `json:"newPath"`
}

func handleSaveScene(ctx context.Context, raw json.RawMessage) (any, error) {
	var args saveSceneArgs
	if len(raw) > 0 && string(raw) != "null" {
		if err := json.Unmarshal(raw, &args); err != nil {
			return nil, fmt.Errorf("invalid arguments: %w", err)
		}
	}
	if args.ProjectPath == "" || args.ScenePath == "" {
		return mcp.ErrorResult(
			"Missing required parameters",
			"Provide projectPath and scenePath",
		), nil
	}
	sceneFull, pathOK := confinePath(args.ProjectPath, args.ScenePath)
	if !validPath(args.ProjectPath) || !pathOK {
		return mcp.ErrorResult(
			"Invalid path",
			`Provide valid paths without ".." or other potentially unsafe characters`,
		), nil
	}
	if args.NewPath != "" && !validPath(args.NewPath) {
		return mcp.ErrorResult(
			"Invalid new path",
			`Provide a valid new path without ".." or other potentially unsafe characters`,
		), nil
	}

	if !pathExists(filepath.Join(args.ProjectPath, "project.godot")) {
		return notAProjectResult(args.ProjectPath), nil
	}

	if !pathExists(sceneFull) {
		return sceneMissingResult(args.ScenePath), nil
	}

	params := map[string]any{
		"scenePath": args.ScenePath,
	}
	if args.NewPath != "" {
		params["newPath"] = args.NewPath
	}

	stdout, stderr, exitCode, err := runOperation(ctx, args.ProjectPath, "save_scene", params)
	if err != nil {
		return mcp.ErrorResult( //nolint:nilerr // the failure is reported through the tool result.
			"Failed to save scene: "+err.Error(),
			"Ensure Godot is installed correctly",
			"Check if the GODOT_PATH environment variable is set correctly",
			"Verify the project path is accessible",
		), nil
	}
	if scriptFailed(exitCode, stderr) {
		return mcp.ErrorResult(
			"Failed to save scene: "+stderr,
			"Check if the scene file is valid",
			"Ensure you have write permissions to the output path",
			"Verify the scene can be properly packed",
		), nil
	}

	savePath := args.ScenePath
	if args.NewPath != "" {
		savePath = args.NewPath
	}
	message := fmt.Sprintf("Scene saved successfully to: %s\n\nOutput: %s", savePath, stdout)
	fields := map[string]any{
		"scenePath": savePath,
		"output":    stdout,
	}
	if args.NewPath != "" {
		fields["newPath"] = args.NewPath
	}
	return statusResult(message, fields), nil
}
