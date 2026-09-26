package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"

	"github.com/vilppuvuorinen/slopfood-godot-mcp/internal/mcp"
)

func createResource() mcp.Tool {
	return mcp.Tool{
		Name:        "create_resource",
		Title:       "Create Resource",
		Description: "Create a .tres/.res resource file",
		Annotations: &mcp.ToolAnnotations{DestructiveHint: mcp.Bool(true), IdempotentHint: mcp.Bool(true), OpenWorldHint: mcp.Bool(false)},
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"resourcePath": map[string]any{
					"type":        "string",
					"description": "Resource file path (relative to project)",
				},
				"resourceType": map[string]any{
					"type":        "string",
					"description": `Godot class name (e.g., "StandardMaterial3D", "Theme", "Environment")`,
				},
				"properties": propertiesSchema("Properties to set, as name to godotValue pairs"),
			},
			"required": []string{"resourcePath", "resourceType"},
		},
		OutputSchema: statusOutputSchema(map[string]any{
			"resourcePath": map[string]any{"type": "string"},
			"resourceType": map[string]any{"type": "string"},
		}),
		Handler: handleCreateResource,
	}
}

type createResourceArgs struct {
	ResourcePath string         `json:"resourcePath"`
	ResourceType string         `json:"resourceType"`
	Properties   map[string]any `json:"properties"`
}

func handleCreateResource(ctx context.Context, raw json.RawMessage) (any, error) {
	var args createResourceArgs
	if len(raw) > 0 && string(raw) != "null" {
		if err := json.Unmarshal(raw, &args); err != nil {
			return nil, fmt.Errorf("invalid arguments: %w", err)
		}
	}
	project := projectRoot(ctx)
	if args.ResourcePath == "" || args.ResourceType == "" {
		return requiredArgsResult("resourcePath", "resourceType"), nil
	}
	if !validPath(project) || !validPath(args.ResourcePath) {
		return invalidPathResult(), nil
	}
	if !pathExists(filepath.Join(project, "project.godot")) {
		return notAProjectResult(project), nil
	}

	params := map[string]any{
		"resourcePath": args.ResourcePath,
		"action":       "create",
		"resourceType": args.ResourceType,
	}
	if args.Properties != nil {
		params["properties"] = args.Properties
	}

	return runHeadlessOperation(ctx, project, "manage_resource", params, map[string]any{
		"resourcePath": args.ResourcePath,
		"resourceType": args.ResourceType,
	})
}
