package tools

import (
	"encoding/json"
	"fmt"

	"github.com/vilppuvuorinen/slopfood-godot-mcp/internal/mcp"
)

func readScene() mcp.Tool {
	return mcp.Tool{
		Name:        "read_scene",
		Title:       "Read Scene",
		Description: "Read scene file as JSON node tree, including signal connections (headless)",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: mcp.Bool(true), OpenWorldHint: mcp.Bool(false)},
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"scenePath": map[string]any{
					"type":        "string",
					"description": "Scene file path (relative to project)",
				},
				"nodePath": map[string]any{
					"type":        "string",
					"description": "Optional node path relative to the scene root; only this node's subtree is returned",
				},
				"depth": map[string]any{
					"type":        "integer",
					"minimum":     0,
					"description": "Optional maximum number of child levels below the node; 0 returns the node without children, omitted means no limit",
				},
			},
			"required": []string{"scenePath"},
		},
		Handler: withContext(handleReadScene),
	}
}

type readSceneArgs struct {
	ScenePath string `json:"scenePath"`
	NodePath  string `json:"nodePath"`
	Depth     *int   `json:"depth"`
}

func handleReadScene(ctx Context, raw json.RawMessage) (any, error) {
	var args readSceneArgs
	if len(raw) > 0 && string(raw) != "null" {
		if err := json.Unmarshal(raw, &args); err != nil {
			return nil, fmt.Errorf("invalid arguments: %w", err)
		}
	}
	project := projectRoot(ctx)
	if args.ScenePath == "" {
		return requiredArgsResult("scenePath"), nil
	}
	if args.Depth != nil && *args.Depth < 0 {
		return mcp.ErrorResult(
			"depth must be zero or greater.",
			"Omit depth to return the whole subtree",
		), nil
	}
	return readSceneThrough(ctx, project, args.ScenePath, args.NodePath, args.Depth)
}
