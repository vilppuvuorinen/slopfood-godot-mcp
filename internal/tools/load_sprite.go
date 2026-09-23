package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"

	"github.com/vilppuvuorinen/go-dot-mcp/internal/mcp"
)

func loadSprite() mcp.Tool {
	return mcp.Tool{
		Name:        "load_sprite",
		Description: "Load a sprite into a Sprite2D node",
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
				"nodePath": map[string]any{
					"type":        "string",
					"description": `Path to the Sprite2D node (e.g., "root/Player/Sprite2D")`,
				},
				"texturePath": map[string]any{
					"type":        "string",
					"description": "Path to the texture file (relative to project)",
				},
			},
			"required": []string{"projectPath", "scenePath", "nodePath", "texturePath"},
		},
		Handler: handleLoadSprite,
	}
}

type loadSpriteArgs struct {
	ProjectPath string `json:"projectPath"`
	ScenePath   string `json:"scenePath"`
	NodePath    string `json:"nodePath"`
	TexturePath string `json:"texturePath"`
}

func handleLoadSprite(ctx context.Context, raw json.RawMessage) (any, error) {
	raw, err := aliasParams(raw, parameterMappings)
	if err != nil {
		return nil, err
	}
	var args loadSpriteArgs
	if len(raw) > 0 && string(raw) != "null" {
		if err = json.Unmarshal(raw, &args); err != nil {
			return nil, fmt.Errorf("invalid arguments: %w", err)
		}
	}
	if args.ProjectPath == "" || args.ScenePath == "" || args.NodePath == "" || args.TexturePath == "" {
		return mcp.ErrorResult(
			"Missing required parameters",
			"Provide projectPath, scenePath, nodePath, and texturePath",
		), nil
	}
	if !validPath(args.ProjectPath) || !validPath(args.ScenePath) || !validPath(args.NodePath) || !validPath(args.TexturePath) {
		return mcp.ErrorResult(
			"Invalid path",
			`Provide valid paths without ".." or other potentially unsafe characters`,
		), nil
	}

	if !pathExists(filepath.Join(args.ProjectPath, "project.godot")) {
		return notAProjectResult(args.ProjectPath), nil
	}

	if !pathExists(filepath.Join(args.ProjectPath, args.ScenePath)) {
		return sceneMissingResult(args.ScenePath), nil
	}

	if !pathExists(filepath.Join(args.ProjectPath, args.TexturePath)) {
		return mcp.ErrorResult(
			"Texture file does not exist: "+args.TexturePath,
			"Ensure the texture path is correct",
			"Upload or create the texture file first",
		), nil
	}

	params := map[string]any{
		"scenePath":   args.ScenePath,
		"nodePath":    args.NodePath,
		"texturePath": args.TexturePath,
	}
	stdout, stderr, exitCode, err := runOperation(ctx, args.ProjectPath, "load_sprite", params)
	if err != nil {
		return mcp.ErrorResult( //nolint:nilerr // the failure is reported through the tool result.
			"Failed to load sprite: "+err.Error(),
			"Ensure Godot is installed correctly",
			"Check if the GODOT_PATH environment variable is set correctly",
			"Verify the project path is accessible",
		), nil
	}
	if scriptFailed(stderr, exitCode) {
		return mcp.ErrorResult(
			"Failed to load sprite: "+stderr,
			"Check if the node path is correct",
			"Ensure the node is a Sprite2D, Sprite3D, or TextureRect",
			"Verify the texture file is a valid image format",
		), nil
	}

	message := fmt.Sprintf("Sprite loaded successfully with texture: %s\n\nOutput: %s", args.TexturePath, stdout)
	return mcp.TextResult(message), nil
}
