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
		Description: "Create or modify .tres/.res resource files",
		Annotations: &mcp.ToolAnnotations{DestructiveHint: mcp.Bool(true), IdempotentHint: mcp.Bool(true), OpenWorldHint: mcp.Bool(false)},
		InputSchema: actionInputSchema(
			map[string]any{
				"resourcePath": map[string]any{
					"type":        "string",
					"description": "Resource file path (relative to project)",
				},
				"action": map[string]any{
					"type":        "string",
					"enum":        []string{"create", "modify"},
					"description": "Action: create or modify",
				},
				"resourceType": map[string]any{
					"type":        "string",
					"description": `Godot class name for create (e.g., "StandardMaterial3D", "Theme", "Environment")`,
				},
				"properties": propertiesSchema("Properties to set, as name to godotValue pairs"),
			},
			[]string{"resourcePath", "action"},
			[]actionVariant{
				{Action: "create", Required: []string{"resourceType"}},
				{Action: "modify", Required: []string{"properties"}},
			},
		),
		OutputSchema: statusOutputSchema(map[string]any{
			"resourcePath": map[string]any{"type": "string"},
			"action":       map[string]any{"type": "string"},
			"resourceType": map[string]any{"type": "string"},
		}),
		Handler: handleManageResource,
	}
}

type manageResourceArgs struct {
	ResourcePath string         `json:"resourcePath"`
	Action       string         `json:"action"`
	ResourceType string         `json:"resourceType"`
	Properties   map[string]any `json:"properties"`
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
		return requiredArgsResult("resourcePath", "action"), nil
	}
	if args.Action == "create" && args.ResourceType == "" {
		return mcp.ErrorResult("resourceType is required for the create action."), nil
	}
	if args.Action == "modify" && len(args.Properties) == 0 {
		return mcp.ErrorResult("properties is required for the modify action.", "Provide at least one property to set"), nil
	}
	if !validPath(project) || !validPath(args.ResourcePath) {
		return invalidPathResult(), nil
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

	return runHeadlessOperation(ctx, project, "manage_resource", params, fields)
}
