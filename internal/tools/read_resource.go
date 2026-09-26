package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"

	"github.com/vilppuvuorinen/go-dot-mcp/internal/mcp"
)

func readResource() mcp.Tool {
	return mcp.Tool{
		Name:        "read_resource",
		Title:       "Read Resource",
		Description: "Read .tres/.res resource files",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: mcp.Bool(true), OpenWorldHint: mcp.Bool(false)},
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"resourcePath": map[string]any{
					"type":        "string",
					"description": "Resource file path (relative to project)",
				},
				"propertyFilter": map[string]any{
					"type":        "array",
					"items":       map[string]any{"type": "string"},
					"description": "Limit the returned properties to these names (wildcards * and ? allowed)",
				},
			},
			"required": []string{"resourcePath"},
		},
		OutputSchema: payloadOutputSchema(map[string]any{
			"type": map[string]any{"type": "string"},
			"path": map[string]any{"type": "string", "description": "Resource path, relative to the bound project"},
			"properties": map[string]any{
				"type":                 "object",
				"description":          "Resource properties in the tool value contract",
				"additionalProperties": godotValueSchema("A resource property value in the tool value contract"),
			},
		}),
		Handler: handleReadResource,
	}
}

type readResourceArgs struct {
	ResourcePath   string   `json:"resourcePath"`
	PropertyFilter []string `json:"propertyFilter"`
}

func handleReadResource(ctx context.Context, raw json.RawMessage) (any, error) {
	var args readResourceArgs
	if len(raw) > 0 && string(raw) != "null" {
		if err := json.Unmarshal(raw, &args); err != nil {
			return nil, fmt.Errorf("invalid arguments: %w", err)
		}
	}
	project := projectRoot(ctx)
	if args.ResourcePath == "" {
		return requiredArgsResult("resourcePath"), nil
	}
	if !validPath(project) || !validPath(args.ResourcePath) {
		return invalidPathResult(), nil
	}
	if !pathExists(filepath.Join(project, "project.godot")) {
		return notAProjectResult(project), nil
	}

	params := map[string]any{
		"resourcePath": args.ResourcePath,
		"action":       "read",
	}
	if len(args.PropertyFilter) > 0 {
		params["propertyFilter"] = args.PropertyFilter
	}
	fields := map[string]any{
		"resourcePath": args.ResourcePath,
		"action":       "read",
	}
	return runHeadlessOperation(ctx, project, "manage_resource", params, fields)
}
