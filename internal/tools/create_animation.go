package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"

	"github.com/vilppuvuorinen/slopfood-godot-mcp/internal/mcp"
)

// animationLoopModeNames is the tool contract's names for Animation loop modes,
// shared with the engine bridge in godot_operations.gd.
var animationLoopModeNames = []string{"none", "linear", "pingpong"}

func createAnimation() mcp.Tool {
	return mcp.Tool{
		Name:        "create_animation",
		Title:       "Create Animation",
		Description: "Create an Animation resource",
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
		Handler: handleCreateAnimation,
	}
}

type createAnimationArgs struct {
	ResourcePath string   `json:"resourcePath"`
	Length       *float64 `json:"length"`
	LoopMode     string   `json:"loopMode"`
	Step         *float64 `json:"step"`
}

func handleCreateAnimation(ctx context.Context, raw json.RawMessage) (any, error) {
	var args createAnimationArgs
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
		"action":       "create",
	}
	addAnimationProperties(params, args.Length, args.LoopMode, args.Step)

	return runHeadlessOperation(ctx, project, "manage_animation", params, map[string]any{
		"resourcePath": args.ResourcePath,
	})
}

// addAnimationProperties copies the animation-level fields onto params when the
// caller supplied them.
func addAnimationProperties(params map[string]any, length *float64, loopMode string, step *float64) {
	if length != nil {
		params["length"] = *length
	}
	if loopMode != "" {
		params["loopMode"] = loopMode
	}
	if step != nil {
		params["step"] = *step
	}
}
