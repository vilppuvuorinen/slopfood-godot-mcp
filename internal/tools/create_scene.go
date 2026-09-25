package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"

	"github.com/vilppuvuorinen/go-dot-mcp/internal/mcp"
)

func createScene() mcp.Tool {
	return mcp.Tool{
		Name:        "create_scene",
		Title:       "Create Scene",
		Description: "Create a new Godot scene file",
		Annotations: &mcp.ToolAnnotations{DestructiveHint: mcp.Bool(true), OpenWorldHint: mcp.Bool(false)},
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"scenePath": map[string]any{
					"type":        "string",
					"description": "Path where the scene file will be saved (relative to project)",
				},
				"rootNodeType": map[string]any{
					"type":        "string",
					"description": "Type of the root node (e.g., Node2D, Node3D)",
				},
			},
			"required": []string{"scenePath"},
		},
		OutputSchema: statusOutputSchema(map[string]any{
			"scenePath":    map[string]any{"type": "string"},
			"rootNodeType": map[string]any{"type": "string"},
		}),
		Handler: handleCreateScene,
	}
}

type createSceneArgs struct {
	ScenePath    string `json:"scenePath"`
	RootNodeType string `json:"rootNodeType"`
}

func handleCreateScene(ctx context.Context, raw json.RawMessage) (any, error) {
	var args createSceneArgs
	if len(raw) > 0 && string(raw) != "null" {
		if err := json.Unmarshal(raw, &args); err != nil {
			return nil, fmt.Errorf("invalid arguments: %w", err)
		}
	}
	project := projectRoot(ctx)
	if args.ScenePath == "" {
		return requiredArgsResult("scenePath"), nil
	}
	if !validPath(project) || !validPath(args.ScenePath) {
		return invalidPathResult(), nil
	}

	rootNodeType := args.RootNodeType
	if rootNodeType == "" {
		rootNodeType = "Node2D"
	}
	if !validClassName(rootNodeType) {
		return mcp.ErrorResult(
			"Invalid rootNodeType",
			"rootNodeType must be a built-in Godot class name (no paths, no file extensions)",
		), nil
	}

	if !pathExists(filepath.Join(project, "project.godot")) {
		return notAProjectResult(project), nil
	}

	params := map[string]any{
		"scenePath":    args.ScenePath,
		"rootNodeType": rootNodeType,
	}
	stdout, stderr, exitCode, err := runOperation(ctx, project, "create_scene", params)
	if err != nil {
		return mcp.ErrorResult( //nolint:nilerr // the failure is reported through the tool result.
			"Failed to create scene: "+err.Error(),
			"Ensure Godot is installed correctly",
			"Check if the GODOT_PATH environment variable is set correctly",
			"Verify the project path is accessible",
		), nil
	}
	if scriptFailed(exitCode, stderr) {
		return mcp.ErrorResult(
			"Failed to create scene: "+stderr,
			"Check if the root node type is valid",
			"Ensure you have write permissions to the scene path",
			"Verify the scene path is valid",
		), nil
	}

	message := fmt.Sprintf("Scene created successfully at: %s\n\nOutput: %s", args.ScenePath, stdout)
	return statusResult(project, message, map[string]any{
		"scenePath":    args.ScenePath,
		"rootNodeType": rootNodeType,
	}), nil
}
