package tools

import (
	"encoding/json"
	"fmt"
	"path/filepath"

	"github.com/vilppuvuorinen/slopfood-godot-mcp/internal/mcp"
)

func readAnimation() mcp.Tool {
	return mcp.Tool{
		Name:        "read_animation",
		Title:       "Read Animation",
		Description: "Read Animation resources and their tracks",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: mcp.Bool(true), OpenWorldHint: mcp.Bool(false)},
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"resourcePath": map[string]any{
					"type":        "string",
					"description": "Animation resource file path (relative to project)",
				},
			},
			"required": []string{"resourcePath"},
		},
		Handler: withContext(handleReadAnimation),
	}
}

type readAnimationArgs struct {
	ResourcePath string `json:"resourcePath"`
}

func handleReadAnimation(ctx Context, raw json.RawMessage) (any, error) {
	var args readAnimationArgs
	if len(raw) > 0 && string(raw) != "null" {
		if err := json.Unmarshal(raw, &args); err != nil {
			return nil, fmt.Errorf("invalid arguments: %w", err)
		}
	}
	project := projectRoot(ctx)
	if args.ResourcePath == "" {
		return requiredArgsResult("resourcePath"), nil
	}
	if _, ok := normalizeReadPath(project, args.ResourcePath); !ok {
		return invalidPathResult(), nil
	}
	if !pathExists(filepath.Join(project, "project.godot")) {
		return notAProjectResult(project), nil
	}

	params := map[string]any{
		"resourcePath": args.ResourcePath,
	}
	fields := map[string]any{
		"resourcePath": args.ResourcePath,
	}
	return runHeadlessOperation(ctx, project, "read_animation", params, fields)
}
