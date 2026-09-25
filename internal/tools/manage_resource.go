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
		Annotations: &mcp.ToolAnnotations{DestructiveHint: mcp.Bool(true), IdempotentHint: mcp.Bool(true), OpenWorldHint: mcp.Bool(false)},
		InputSchema: actionInputSchema(
			map[string]any{
				"resourcePath": map[string]any{
					"type":        "string",
					"description": "Resource file path (relative to project)",
				},
				"action": map[string]any{
					"type":        "string",
					"enum":        []string{"create", "read", "modify"},
					"description": "Action: create, read, or modify",
				},
				"resourceType": map[string]any{
					"type":        "string",
					"description": `Godot class name for create (e.g., "StandardMaterial3D", "Theme", "Environment")`,
				},
				"properties": propertiesSchema("Properties to set, as name to godotValue pairs"),
				"propertyFilter": map[string]any{
					"type":        "array",
					"items":       map[string]any{"type": "string"},
					"description": "Read action only: limit the returned properties to these names (wildcards * and ? allowed)",
				},
			},
			[]string{"resourcePath", "action"},
			[]actionVariant{
				{Action: "create", Required: []string{"resourceType"}},
				{Action: "read"},
				{Action: "modify"},
			},
		),
		OutputSchema: actionOutputSchema(
			map[string]any{
				"resourcePath": map[string]any{"type": "string"},
				"action":       map[string]any{"type": "string"},
				"resourceType": map[string]any{"type": "string"},
			},
			payloadOutputSchema(map[string]any{
				"type": map[string]any{"type": "string"},
				"path": map[string]any{"type": "string", "description": "Resource path, relative to the bound project"},
				"properties": map[string]any{
					"type":                 "object",
					"description":          "Resource properties in the tool value contract",
					"additionalProperties": godotValueSchema("JSON-native values are nested; engine-only types are tagged objects"),
				},
			}),
		),
		Handler: handleManageResource,
	}
}

type manageResourceArgs struct {
	ResourcePath   string         `json:"resourcePath"`
	Action         string         `json:"action"`
	ResourceType   string         `json:"resourceType"`
	Properties     map[string]any `json:"properties"`
	PropertyFilter []string       `json:"propertyFilter"`
}

func handleManageResource(ctx context.Context, raw json.RawMessage) (any, error) {
	var args manageResourceArgs
	if len(raw) > 0 && string(raw) != "null" {
		if err := json.Unmarshal(raw, &args); err != nil {
			return nil, fmt.Errorf("invalid arguments: %w", err)
		}
	}
	project := projectRoot(ctx)
	if args.ResourcePath == "" || args.Action == "" {
		return mcp.ErrorResult("resourcePath and action are required."), nil
	}
	if args.Action == "create" && args.ResourceType == "" {
		return mcp.ErrorResult("resourceType is required for the create action."), nil
	}
	if !validPath(project) || !validPath(args.ResourcePath) {
		return mcp.ErrorResult("Invalid path."), nil
	}
	if !pathExists(filepath.Join(project, "project.godot")) {
		return notAProjectResult(project), nil
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
	if args.Action == "read" && len(args.PropertyFilter) > 0 {
		params["propertyFilter"] = args.PropertyFilter
	}

	return runHeadlessOperation(ctx, project, "manage_resource", params, fields)
}
