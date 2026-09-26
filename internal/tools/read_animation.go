package tools

import (
	"context"
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
		OutputSchema: animationPayloadSchema(),
		Handler:      handleReadAnimation,
	}
}

// animationPayloadSchema describes read_animation's payload: the animation-level
// settings plus one object per track.
func animationPayloadSchema() map[string]any {
	return payloadOutputSchema(map[string]any{
		"path":     map[string]any{"type": "string", "description": "Animation resource path, relative to the bound project"},
		"length":   map[string]any{"type": "number", "description": "Animation length in seconds"},
		"loopMode": map[string]any{"type": "string", "description": "Loop mode: none, linear, or pingpong"},
		"step":     map[string]any{"type": "number", "description": "Step granularity in seconds"},
		"tracks":   map[string]any{"type": "array", "description": "Animation tracks in index order", "items": animationTrackSchema()},
	})
}

func animationTrackSchema() map[string]any {
	return map[string]any{
		"type":        "object",
		"description": "One animation track",
		"properties": map[string]any{
			"index":         map[string]any{"type": "integer", "description": "Track index"},
			"type":          map[string]any{"type": "string", "description": "Track type name from the tool contract"},
			"path":          map[string]any{"type": "string", "description": "Track path, relative to the AnimationPlayer"},
			"interpolation": map[string]any{"type": "string", "description": "Interpolation: nearest, linear, or cubic"},
			"loopWrap":      map[string]any{"type": "boolean", "description": "Whether interpolation wraps across the loop boundary"},
			"enabled":       map[string]any{"type": "boolean", "description": "Whether the track contributes to playback"},
			"keys":          map[string]any{"type": "array", "description": "Keyframes in track order", "items": animationReadKeySchema()},
		},
		"required": []string{"index", "type", "path", "interpolation", "loopWrap", "enabled", "keys"},
	}
}

func animationReadKeySchema() map[string]any {
	return map[string]any{
		"type":        "object",
		"description": "One keyframe; a method value is {method, args} and a bezier key adds inHandle and outHandle",
		"properties": map[string]any{
			"time":       map[string]any{"type": "number", "description": "Key time in seconds"},
			"value":      godotValueSchema("Key value in the tool value contract"),
			"transition": map[string]any{"type": "number", "description": "Interpolation transition"},
			"inHandle":   godotValueSchema("Bezier in-handle offset"),
			"outHandle":  godotValueSchema("Bezier out-handle offset"),
		},
		"required": []string{"time", "value"},
	}
}

type readAnimationArgs struct {
	ResourcePath string `json:"resourcePath"`
}

func handleReadAnimation(ctx context.Context, raw json.RawMessage) (any, error) {
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
	fields := map[string]any{
		"resourcePath": args.ResourcePath,
		"action":       "read",
	}
	return runHeadlessOperation(ctx, project, "manage_animation", params, fields)
}
