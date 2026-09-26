package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"

	"github.com/vilppuvuorinen/slopfood-godot-mcp/internal/mcp"
)

func modifyResource() mcp.Tool {
	return mcp.Tool{
		Name:        "modify_resource",
		Title:       "Modify Resource",
		Description: "Set properties on a .tres/.res resource file",
		Annotations: &mcp.ToolAnnotations{DestructiveHint: mcp.Bool(true), IdempotentHint: mcp.Bool(true), OpenWorldHint: mcp.Bool(false)},
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"resourcePath": map[string]any{
					"type":        "string",
					"description": "Resource file path (relative to project)",
				},
				"properties": propertiesSchema("Properties to set, as name to godotValue pairs"),
			},
			"required": []string{"resourcePath", "properties"},
		},
		OutputSchema: statusOutputSchema(map[string]any{
			"resourcePath": map[string]any{"type": "string"},
		}),
		Handler: handleModifyResource,
	}
}

type modifyResourceArgs struct {
	ResourcePath string         `json:"resourcePath"`
	Properties   map[string]any `json:"properties"`
}

func handleModifyResource(ctx context.Context, raw json.RawMessage) (any, error) {
	var args modifyResourceArgs
	if len(raw) > 0 && string(raw) != "null" {
		if err := json.Unmarshal(raw, &args); err != nil {
			return nil, fmt.Errorf("invalid arguments: %w", err)
		}
	}
	project := projectRoot(ctx)
	if args.ResourcePath == "" || len(args.Properties) == 0 {
		return requiredArgsResult("resourcePath", "properties"), nil
	}
	if !validPath(project) || !validPath(args.ResourcePath) {
		return invalidPathResult(), nil
	}
	if !pathExists(filepath.Join(project, "project.godot")) {
		return notAProjectResult(project), nil
	}

	return runHeadlessOperation(ctx, project, "manage_resource", map[string]any{
		"resourcePath": args.ResourcePath,
		"action":       "modify",
		"properties":   args.Properties,
	}, map[string]any{
		"resourcePath": args.ResourcePath,
	})
}
