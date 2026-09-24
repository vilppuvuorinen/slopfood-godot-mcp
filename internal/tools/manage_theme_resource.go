package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"

	"github.com/vilppuvuorinen/go-dot-mcp/internal/mcp"
)

func manageThemeResource() mcp.Tool {
	return mcp.Tool{
		Name:        "manage_theme_resource",
		Title:       "Manage Theme Resource",
		Description: "Create/read/modify Theme .tres resources",
		Annotations: &mcp.ToolAnnotations{DestructiveHint: mcp.Bool(true), OpenWorldHint: mcp.Bool(false)},
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"projectPath": map[string]any{
					"type":        "string",
					"description": "Godot project path",
				},
				"resourcePath": map[string]any{
					"type":        "string",
					"description": "Theme file path (relative to project)",
				},
				"action": map[string]any{
					"type":        "string",
					"description": "Action: create, read, modify",
				},
				"properties": map[string]any{
					"type":        "object",
					"description": "Theme properties to set",
				},
			},
			"required": []string{"projectPath", "resourcePath", "action"},
		},
		Handler: handleManageThemeResource,
	}
}

type manageThemeResourceArgs struct {
	ProjectPath  string         `json:"projectPath"`
	ResourcePath string         `json:"resourcePath"`
	Action       string         `json:"action"`
	Properties   map[string]any `json:"properties"`
}

func handleManageThemeResource(ctx context.Context, raw json.RawMessage) (any, error) {
	raw, err := aliasParams(raw, parameterMappings)
	if err != nil {
		return nil, err
	}
	var args manageThemeResourceArgs
	if len(raw) > 0 && string(raw) != "null" {
		if err = json.Unmarshal(raw, &args); err != nil {
			return nil, fmt.Errorf("invalid arguments: %w", err)
		}
	}
	if args.ProjectPath == "" || args.ResourcePath == "" || args.Action == "" {
		return mcp.ErrorResult("projectPath, resourcePath, and action are required."), nil
	}
	if !validPath(args.ProjectPath) || !validPath(args.ResourcePath) {
		return mcp.ErrorResult("Invalid path."), nil
	}
	if !pathExists(filepath.Join(args.ProjectPath, "project.godot")) {
		return notAProjectResult(args.ProjectPath), nil
	}

	params := map[string]any{
		"resourcePath": args.ResourcePath,
		"action":       args.Action,
	}
	if args.Properties != nil {
		params["properties"] = args.Properties
	}

	return runHeadlessOperation(ctx, args.ProjectPath, "manage_theme_resource", params)
}
