package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"

	"github.com/vilppuvuorinen/slopfood-godot-mcp/internal/mcp"
)

func setAnimationTrack() mcp.Tool {
	return mcp.Tool{
		Name:        "set_animation_track",
		Title:       "Set Animation Track",
		Description: "Set an existing Animation track's path, interpolation, and keys",
		Annotations: &mcp.ToolAnnotations{DestructiveHint: mcp.Bool(true), IdempotentHint: mcp.Bool(true), OpenWorldHint: mcp.Bool(false)},
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"resourcePath": map[string]any{
					"type":        "string",
					"description": "Animation resource file path (relative to project)",
				},
				"trackIndex": map[string]any{
					"type":        "integer",
					"description": "Track index to update",
				},
				"trackPath": map[string]any{
					"type":        "string",
					"description": "Track path, a NodePath relative to the AnimationPlayer",
				},
				"interpolation": map[string]any{
					"type":        "string",
					"enum":        animationInterpolationNames,
					"description": "Track interpolation",
				},
				"keys": map[string]any{
					"type":        "array",
					"description": "Keyframes for the track; replaces the track's keys",
					"items":       animationKeySchema(),
				},
			},
			"required": []string{"resourcePath", "trackIndex"},
		},
		OutputSchema: statusOutputSchema(map[string]any{
			"resourcePath": map[string]any{"type": "string"},
			"trackIndex":   map[string]any{"type": "integer"},
		}),
		Handler: handleSetAnimationTrack,
	}
}

type setAnimationTrackArgs struct {
	ResourcePath  string           `json:"resourcePath"`
	TrackIndex    *int             `json:"trackIndex"`
	TrackPath     string           `json:"trackPath"`
	Interpolation string           `json:"interpolation"`
	Keys          []map[string]any `json:"keys"`
}

func handleSetAnimationTrack(ctx context.Context, raw json.RawMessage) (any, error) {
	var args setAnimationTrackArgs
	if len(raw) > 0 && string(raw) != "null" {
		if err := json.Unmarshal(raw, &args); err != nil {
			return nil, fmt.Errorf("invalid arguments: %w", err)
		}
	}
	project := projectRoot(ctx)
	if args.ResourcePath == "" || args.TrackIndex == nil {
		return requiredArgsResult("resourcePath", "trackIndex"), nil
	}
	if !validPath(project) || !validPath(args.ResourcePath) {
		return invalidPathResult(), nil
	}
	if !pathExists(filepath.Join(project, "project.godot")) {
		return notAProjectResult(project), nil
	}

	params := map[string]any{
		"resourcePath": args.ResourcePath,
		"action":       "set",
		"trackIndex":   *args.TrackIndex,
	}
	addAnimationTrackFields(params, args.TrackPath, args.Interpolation, args.Keys)

	return runHeadlessOperation(ctx, project, "manage_animation", params, map[string]any{
		"resourcePath": args.ResourcePath,
		"trackIndex":   *args.TrackIndex,
	})
}
