package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"

	"github.com/vilppuvuorinen/go-dot-mcp/internal/mcp"
)

func createResource() mcp.Tool {
	return mcp.Tool{
		Name:        "create_resource",
		Title:       "Create Resource",
		Description: "Create a .tres resource file (headless)",
		Annotations: &mcp.ToolAnnotations{DestructiveHint: mcp.Bool(false), OpenWorldHint: mcp.Bool(false)},
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"projectPath": map[string]any{
					"type":        "string",
					"description": "Godot project path",
				},
				"resourceType": map[string]any{
					"type":        "string",
					"description": `Godot class name (e.g., "StandardMaterial3D", "Theme", "Environment")`,
				},
				"resourcePath": map[string]any{
					"type":        "string",
					"description": "Where to save the .tres file (relative to project)",
				},
				"properties": map[string]any{
					"type":        "object",
					"description": "Optional properties to set on the resource",
				},
			},
			"required": []string{"projectPath", "resourceType", "resourcePath"},
		},
		Handler: handleCreateResource,
	}
}

type createResourceArgs struct {
	ProjectPath  string         `json:"projectPath"`
	ResourceType string         `json:"resourceType"`
	ResourcePath string         `json:"resourcePath"`
	Properties   map[string]any `json:"properties"`
}

func handleCreateResource(ctx context.Context, raw json.RawMessage) (any, error) {
	raw, err := aliasParams(raw, parameterMappings)
	if err != nil {
		return nil, err
	}
	var args createResourceArgs
	if len(raw) > 0 && string(raw) != "null" {
		if err = json.Unmarshal(raw, &args); err != nil {
			return nil, fmt.Errorf("invalid arguments: %w", err)
		}
	}
	if args.ProjectPath == "" || args.ResourceType == "" || args.ResourcePath == "" {
		return mcp.ErrorResult("projectPath, resourceType, and resourcePath are required."), nil
	}
	if !validPath(args.ProjectPath) || !validPath(args.ResourcePath) {
		return mcp.ErrorResult("Invalid path."), nil
	}
	if !pathExists(filepath.Join(args.ProjectPath, "project.godot")) {
		return notAProjectResult(args.ProjectPath), nil
	}

	params := map[string]any{
		"resourceType": args.ResourceType,
		"resourcePath": args.ResourcePath,
	}
	if args.Properties != nil {
		params["properties"] = args.Properties
	}

	return runHeadlessOperation(ctx, args.ProjectPath, "create_resource", params)
}
