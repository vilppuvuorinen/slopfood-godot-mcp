package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/vilppuvuorinen/slopfood-godot-mcp/internal/mcp"
)

func readResource() mcp.Tool {
	return mcp.Tool{
		Name:        "read_resource",
		Title:       "Read Resource",
		Description: "Read engine-decoded binary .res resource files (text .tres files are read directly)",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: mcp.Bool(true), OpenWorldHint: mcp.Bool(false)},
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"resourcePath": map[string]any{
					"type":        "string",
					"description": "Binary .res resource path (relative to project)",
				},
				"propertyFilter": map[string]any{
					"type":        "array",
					"items":       map[string]any{"type": "string"},
					"description": "Limit the returned properties to these names (wildcards * and ? allowed)",
				},
			},
			"required": []string{"resourcePath"},
		},
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

	// A .tres is plain text, so the engine round-trip this tool performs adds
	// nothing a direct read would not: reject it and point at the file itself.
	if strings.HasSuffix(strings.ToLower(args.ResourcePath), ".tres") {
		return mcp.ErrorResult(
			"read_resource reads binary .res resources; "+args.ResourcePath+" is a text .tres file.",
			"Read .tres files directly",
		), nil
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
