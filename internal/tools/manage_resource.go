package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"

	"github.com/vilppuvuorinen/go-dot-mcp/internal/mcp"
)

func manageResource() mcp.Tool {
	return mcp.Tool{
		Name:        "manage_resource",
		Title:       "Manage Resource",
		Description: "Create, read, or modify .tres/.res resource files",
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
					"description": "Resource file path (relative to project)",
				},
				"action": map[string]any{
					"type":        "string",
					"description": "Action: create, read, or modify",
				},
				"resourceType": map[string]any{
					"type":        "string",
					"description": `Godot class name for create (e.g., "StandardMaterial3D", "Theme", "Environment")`,
				},
				"properties": propertiesSchema("Properties to set, as name to godotValue pairs"),
			},
			"required": []string{"projectPath", "resourcePath", "action"},
		},
		OutputSchema: statusOutputSchema(map[string]any{
			"resourcePath": map[string]any{"type": "string"},
			"action":       map[string]any{"type": "string"},
			"resourceType": map[string]any{"type": "string"},
			"type":         map[string]any{"type": "string"},
			"path":         map[string]any{"type": "string"},
			"properties": map[string]any{
				"type":                 "object",
				"description":          "Resource properties in the tool value contract",
				"additionalProperties": godotValueSchema("JSON-native values are nested; engine-only types are tagged objects"),
			},
		}),
		Handler: handleManageResource,
	}
}

type manageResourceArgs struct {
	ProjectPath  string         `json:"projectPath"`
	ResourcePath string         `json:"resourcePath"`
	Action       string         `json:"action"`
	ResourceType string         `json:"resourceType"`
	Properties   map[string]any `json:"properties"`
}

func handleManageResource(ctx context.Context, raw json.RawMessage) (any, error) {
	raw, err := aliasParams(raw, parameterMappings)
	if err != nil {
		return nil, err
	}
	var args manageResourceArgs
	if len(raw) > 0 && string(raw) != "null" {
		if err = json.Unmarshal(raw, &args); err != nil {
			return nil, fmt.Errorf("invalid arguments: %w", err)
		}
	}
	if args.ProjectPath == "" || args.ResourcePath == "" || args.Action == "" {
		return mcp.ErrorResult("projectPath, resourcePath, and action are required."), nil
	}
	if args.Action == "create" && args.ResourceType == "" {
		return mcp.ErrorResult("resourceType is required for the create action."), nil
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
	fields := map[string]any{
		"resourcePath": args.ResourcePath,
		"action":       args.Action,
	}
	if args.ResourceType != "" {
		params["resourceType"] = args.ResourceType
		fields["resourceType"] = args.ResourceType
	}
	if args.Properties != nil {
		params["properties"] = args.Properties
	}

	return runHeadlessOperation(ctx, args.ProjectPath, "manage_resource", params, fields)
}
