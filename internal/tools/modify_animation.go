package tools

import (
	"encoding/json"
	"fmt"
	"path/filepath"

	"github.com/vilppuvuorinen/slopfood-godot-mcp/internal/mcp"
)

func modifyAnimation() mcp.Tool {
	return mcp.Tool{
		Name:        "modify_animation",
		Title:       "Modify Animation",
		Description: "Set an Animation resource's length, loop mode, or step",
		Annotations: &mcp.ToolAnnotations{DestructiveHint: mcp.Bool(true), IdempotentHint: mcp.Bool(true), OpenWorldHint: mcp.Bool(false)},
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"resourcePath": map[string]any{
					"type":        "string",
					"description": "Animation resource file path (relative to project)",
				},
				"length": map[string]any{
					"type":        "number",
					"description": "Animation length in seconds",
				},
				"loopMode": map[string]any{
					"type":        "string",
					"enum":        animationLoopModeNames,
					"description": "Loop mode",
				},
				"step": map[string]any{
					"type":        "number",
					"description": "Step granularity in seconds",
				},
			},
			"required": []string{"resourcePath"},
		},
		OutputSchema: statusOutputSchema(map[string]any{
			"resourcePath": map[string]any{"type": "string"},
		}),
		Handler: withContext(handleModifyAnimation),
	}
}

type modifyAnimationArgs struct {
	ResourcePath string   `json:"resourcePath"`
	Length       *float64 `json:"length"`
	LoopMode     string   `json:"loopMode"`
	Step         *float64 `json:"step"`
}

func handleModifyAnimation(ctx Context, raw json.RawMessage) (any, error) {
	var args modifyAnimationArgs
	if len(raw) > 0 && string(raw) != "null" {
		if err := json.Unmarshal(raw, &args); err != nil {
			return nil, fmt.Errorf("invalid arguments: %w", err)
		}
	}
	project := projectRoot(ctx)
	if args.ResourcePath == "" {
		return requiredArgsResult("resourcePath"), nil
	}
	if args.Length == nil && args.LoopMode == "" && args.Step == nil {
		return requiredOneOfResult("modify_animation", "length", "loopMode", "step"), nil
	}
	if !validPath(project) || !validPath(args.ResourcePath) {
		return invalidPathResult(), nil
	}
	if !pathExists(filepath.Join(project, "project.godot")) {
		return notAProjectResult(project), nil
	}

	params := map[string]any{
		"resourcePath": args.ResourcePath,
		"action":       "modify",
	}
	addAnimationProperties(params, args.Length, args.LoopMode, args.Step)

	return runHeadlessOperation(ctx, project, "manage_animation", params, map[string]any{
		"resourcePath": args.ResourcePath,
	})
}
