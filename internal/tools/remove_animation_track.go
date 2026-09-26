package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"

	"github.com/vilppuvuorinen/slopfood-godot-mcp/internal/mcp"
)

func removeAnimationTrack() mcp.Tool {
	return mcp.Tool{
		Name:        "remove_animation_track",
		Title:       "Remove Animation Track",
		Description: "Remove a track from an Animation resource",
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
					"description": "Track index to remove",
				},
			},
			"required": []string{"resourcePath", "trackIndex"},
		},
		OutputSchema: statusOutputSchema(map[string]any{
			"resourcePath": map[string]any{"type": "string"},
			"trackIndex":   map[string]any{"type": "integer"},
		}),
		Handler: handleRemoveAnimationTrack,
	}
}

type removeAnimationTrackArgs struct {
	ResourcePath string `json:"resourcePath"`
	TrackIndex   *int   `json:"trackIndex"`
}

func handleRemoveAnimationTrack(ctx context.Context, raw json.RawMessage) (any, error) {
	var args removeAnimationTrackArgs
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

	return runHeadlessOperation(ctx, project, "manage_animation", map[string]any{
		"resourcePath": args.ResourcePath,
		"action":       "remove",
		"trackIndex":   *args.TrackIndex,
	}, map[string]any{
		"resourcePath": args.ResourcePath,
		"trackIndex":   *args.TrackIndex,
	})
}
