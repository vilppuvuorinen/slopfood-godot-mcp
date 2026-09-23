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
		Description: "Create a new Godot scene file",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"projectPath": map[string]any{
					"type":        "string",
					"description": "Path to the Godot project directory",
				},
				"scenePath": map[string]any{
					"type":        "string",
					"description": "Path where the scene file will be saved (relative to project)",
				},
				"rootNodeType": map[string]any{
					"type":        "string",
					"description": "Type of the root node (e.g., Node2D, Node3D)",
				},
			},
			"required": []string{"projectPath", "scenePath"},
		},
		Handler: handleCreateScene,
	}
}

type createSceneArgs struct {
	ProjectPath  string `json:"projectPath"`
	ScenePath    string `json:"scenePath"`
	RootNodeType string `json:"rootNodeType"`
}

func handleCreateScene(ctx context.Context, raw json.RawMessage) (any, error) {
	raw, err := aliasParams(raw, parameterMappings)
	if err != nil {
		return nil, err
	}
	var args createSceneArgs
	if len(raw) > 0 && string(raw) != "null" {
		if err = json.Unmarshal(raw, &args); err != nil {
			return nil, fmt.Errorf("invalid arguments: %w", err)
		}
	}
	if args.ProjectPath == "" || args.ScenePath == "" {
		return mcp.ErrorResult(
			"Project path and scene path are required",
			"Provide valid paths for both the project and the scene",
		), nil
	}
	if !validPath(args.ProjectPath) || !validPath(args.ScenePath) {
		return mcp.ErrorResult(
			"Invalid path",
			`Provide valid paths without ".." or other potentially unsafe characters`,
		), nil
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

	if !pathExists(filepath.Join(args.ProjectPath, "project.godot")) {
		return notAProjectResult(args.ProjectPath), nil
	}

	params := map[string]any{
		"scenePath":    args.ScenePath,
		"rootNodeType": rootNodeType,
	}
	stdout, stderr, exitCode, err := runOperation(ctx, args.ProjectPath, "create_scene", params)
	if err != nil {
		return mcp.ErrorResult( //nolint:nilerr // the failure is reported through the tool result.
			"Failed to create scene: "+err.Error(),
			"Ensure Godot is installed correctly",
			"Check if the GODOT_PATH environment variable is set correctly",
			"Verify the project path is accessible",
		), nil
	}
	if scriptFailed(stderr, exitCode) {
		return mcp.ErrorResult(
			"Failed to create scene: "+stderr,
			"Check if the root node type is valid",
			"Ensure you have write permissions to the scene path",
			"Verify the scene path is valid",
		), nil
	}

	message := fmt.Sprintf("Scene created successfully at: %s\n\nOutput: %s", args.ScenePath, stdout)
	return mcp.TextResult(message), nil
}
