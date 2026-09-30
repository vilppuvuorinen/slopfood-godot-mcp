package tools

import (
	"encoding/json"
	"fmt"
	"path/filepath"

	"github.com/vilppuvuorinen/slopfood-godot-mcp/internal/mcp"
)

// animationTrackTypeNames and animationInterpolationNames are shared with the
// engine bridge in godot_operations.gd.
var (
	animationTrackTypeNames     = []string{"value", "position_3d", "rotation_3d", "scale_3d", "blend_shape", "method", "bezier", "audio", "animation"}
	animationInterpolationNames = []string{"nearest", "linear", "cubic"}
)

// A key's value is optional because it can be null; every other field is decoded
// per track type by the engine bridge.
func animationKeySchema() map[string]any {
	return map[string]any{
		"type":        "object",
		"description": "One keyframe in the tool value contract",
		"properties": map[string]any{
			"time":        map[string]any{"type": "number", "description": "Key time in seconds"},
			"value":       godotValueSchema("Key value; a method key uses {method, args}"),
			"transition":  map[string]any{"type": "number", "description": "Interpolation transition for a value key"},
			"inHandle":    godotValueSchema("Bezier in-handle offset"),
			"outHandle":   godotValueSchema("Bezier out-handle offset"),
			"startOffset": map[string]any{"type": "number", "description": "Audio key start offset in seconds"},
			"endOffset":   map[string]any{"type": "number", "description": "Audio key end offset in seconds"},
		},
		"required": []string{"time"},
	}
}

func addAnimationTrack() mcp.Tool {
	return mcp.Tool{
		Name:        "add_animation_track",
		Title:       "Add Animation Track",
		Description: "Append a track to an Animation resource",
		Annotations: &mcp.ToolAnnotations{DestructiveHint: mcp.Bool(true), IdempotentHint: mcp.Bool(false), OpenWorldHint: mcp.Bool(false)},
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"resourcePath": map[string]any{
					"type":        "string",
					"description": "Animation resource file path (relative to project)",
				},
				"trackType": map[string]any{
					"type":        "string",
					"enum":        animationTrackTypeNames,
					"description": "Track type",
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
					"description": "Keyframes for the track",
					"items":       animationKeySchema(),
				},
			},
			"required": []string{"resourcePath", "trackType"},
		},
		OutputSchema: statusOutputSchema(map[string]any{
			"resourcePath": map[string]any{"type": "string"},
			"trackType":    map[string]any{"type": "string"},
		}),
		Handler: withContext(handleAddAnimationTrack),
	}
}

type addAnimationTrackArgs struct {
	ResourcePath  string           `json:"resourcePath"`
	TrackType     string           `json:"trackType"`
	TrackPath     string           `json:"trackPath"`
	Interpolation string           `json:"interpolation"`
	Keys          []map[string]any `json:"keys"`
}

func handleAddAnimationTrack(ctx Context, raw json.RawMessage) (any, error) {
	var args addAnimationTrackArgs
	if len(raw) > 0 && string(raw) != "null" {
		if err := json.Unmarshal(raw, &args); err != nil {
			return nil, fmt.Errorf("invalid arguments: %w", err)
		}
	}
	project := projectRoot(ctx)
	if args.ResourcePath == "" || args.TrackType == "" {
		return requiredArgsResult("resourcePath", "trackType"), nil
	}
	if !validPath(project) || !validPath(args.ResourcePath) {
		return invalidPathResult(), nil
	}
	if !pathExists(filepath.Join(project, "project.godot")) {
		return notAProjectResult(project), nil
	}

	params := map[string]any{
		"resourcePath": args.ResourcePath,
		"action":       "add",
		"trackType":    args.TrackType,
	}
	addAnimationTrackFields(params, args.TrackPath, args.Interpolation, args.Keys)

	return runHeadlessOperation(ctx, project, "manage_animation", params, map[string]any{
		"resourcePath": args.ResourcePath,
		"trackType":    args.TrackType,
	})
}

func addAnimationTrackFields(params map[string]any, trackPath, interpolation string, keys []map[string]any) {
	if trackPath != "" {
		params["trackPath"] = trackPath
	}
	if interpolation != "" {
		params["interpolation"] = interpolation
	}
	if keys != nil {
		params["keys"] = keys
	}
}
