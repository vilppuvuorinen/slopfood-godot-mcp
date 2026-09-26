package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"

	"github.com/vilppuvuorinen/slopfood-godot-mcp/internal/mcp"
)

// animationTrackTypeNames, animationLoopModeNames, and
// animationInterpolationNames are the tool contract's names for Animation track
// types and settings, shared with the engine bridge in godot_operations.gd.
var (
	animationTrackTypeNames     = []string{"value", "position_3d", "rotation_3d", "scale_3d", "blend_shape", "method", "bezier", "audio", "animation"}
	animationLoopModeNames      = []string{"none", "linear", "pingpong"}
	animationInterpolationNames = []string{"nearest", "linear", "cubic"}
)

// animationKeySchema describes one keyframe in a track's keys array. A value is
// optional in the schema because a key can carry a null value, but every other
// field is decoded per track type by the engine bridge.
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

func manageAnimation() mcp.Tool {
	return mcp.Tool{
		Name:        "manage_animation",
		Title:       "Manage Animation",
		Description: "Create or modify Animation resources and their tracks",
		Annotations: &mcp.ToolAnnotations{DestructiveHint: mcp.Bool(true), IdempotentHint: mcp.Bool(false), OpenWorldHint: mcp.Bool(false)},
		InputSchema: actionInputSchema(
			map[string]any{
				"resourcePath": map[string]any{
					"type":        "string",
					"description": "Animation resource file path (relative to project)",
				},
				"action": map[string]any{
					"type":        "string",
					"enum":        []string{"create", "modify", "add", "set", "remove"},
					"description": "Action: create, modify, add a track, set a track, or remove a track",
				},
				"length": map[string]any{
					"type":        "number",
					"description": "Animation length in seconds (create, modify)",
				},
				"loopMode": map[string]any{
					"type":        "string",
					"enum":        animationLoopModeNames,
					"description": "Loop mode (create, modify)",
				},
				"step": map[string]any{
					"type":        "number",
					"description": "Step granularity in seconds (create, modify)",
				},
				"trackType": map[string]any{
					"type":        "string",
					"enum":        animationTrackTypeNames,
					"description": "Track type (add)",
				},
				"trackPath": map[string]any{
					"type":        "string",
					"description": "Track path, a NodePath relative to the AnimationPlayer (add, set)",
				},
				"trackIndex": map[string]any{
					"type":        "integer",
					"description": "Track index to update or remove (set, remove)",
				},
				"interpolation": map[string]any{
					"type":        "string",
					"enum":        animationInterpolationNames,
					"description": "Track interpolation (add, set)",
				},
				"keys": map[string]any{
					"type":        "array",
					"description": "Keyframes for the track; a set replaces the track's keys (add, set)",
					"items":       animationKeySchema(),
				},
			},
			[]string{"resourcePath", "action"},
			[]actionVariant{
				{Action: "create"},
				{Action: "modify", AnyOf: []string{"length", "loopMode", "step"}},
				{Action: "add", Required: []string{"trackType"}},
				{Action: "set", Required: []string{"trackIndex"}},
				{Action: "remove", Required: []string{"trackIndex"}},
			},
		),
		OutputSchema: statusOutputSchema(map[string]any{
			"resourcePath": map[string]any{"type": "string"},
			"action":       map[string]any{"type": "string"},
		}),
		Handler: handleManageAnimation,
	}
}

type manageAnimationArgs struct {
	ResourcePath  string           `json:"resourcePath"`
	Action        string           `json:"action"`
	Length        *float64         `json:"length"`
	LoopMode      string           `json:"loopMode"`
	Step          *float64         `json:"step"`
	TrackType     string           `json:"trackType"`
	TrackPath     string           `json:"trackPath"`
	TrackIndex    *int             `json:"trackIndex"`
	Interpolation string           `json:"interpolation"`
	Keys          []map[string]any `json:"keys"`
}

func handleManageAnimation(ctx context.Context, raw json.RawMessage) (any, error) {
	var args manageAnimationArgs
	if len(raw) > 0 && string(raw) != "null" {
		if err := json.Unmarshal(raw, &args); err != nil {
			return nil, fmt.Errorf("invalid arguments: %w", err)
		}
	}
	project := projectRoot(ctx)
	if args.ResourcePath == "" || args.Action == "" {
		return requiredArgsResult("resourcePath", "action"), nil
	}
	switch args.Action {
	case "create":
	case "modify":
		if args.Length == nil && args.LoopMode == "" && args.Step == nil {
			return effectfulFieldsResult("modify", "length", "loopMode", "step"), nil
		}
	case "add":
		if args.TrackType == "" {
			return mcp.ErrorResult("trackType is required for the add action."), nil
		}
	case "set", "remove":
		if args.TrackIndex == nil {
			return mcp.ErrorResult("trackIndex is required for the " + args.Action + " action."), nil
		}
	default:
		return mcp.ErrorResult("Unknown action: " + args.Action + ". Use create, modify, add, set, or remove."), nil
	}
	if !validPath(project) || !validPath(args.ResourcePath) {
		return invalidPathResult(), nil
	}
	if !pathExists(filepath.Join(project, "project.godot")) {
		return notAProjectResult(project), nil
	}

	params := map[string]any{
		"resourcePath": args.ResourcePath,
		"action":       args.Action,
	}
	fields := map[string]any{
		"resourcePath": args.ResourcePath,
		"action":       args.Action,
	}
	switch args.Action {
	case "create", "modify":
		addAnimationProperties(params, args)
	case "add", "set":
		if args.TrackType != "" {
			params["trackType"] = args.TrackType
		}
		if args.TrackPath != "" {
			params["trackPath"] = args.TrackPath
		}
		if args.Interpolation != "" {
			params["interpolation"] = args.Interpolation
		}
		if args.Keys != nil {
			params["keys"] = args.Keys
		}
		if args.Action == "set" {
			params["trackIndex"] = *args.TrackIndex
		}
	case "remove":
		params["trackIndex"] = *args.TrackIndex
	}

	return runHeadlessOperation(ctx, project, "manage_animation", params, fields)
}

// addAnimationProperties copies the animation-level fields onto params when the
// caller supplied them.
func addAnimationProperties(params map[string]any, args manageAnimationArgs) {
	if args.Length != nil {
		params["length"] = *args.Length
	}
	if args.LoopMode != "" {
		params["loopMode"] = args.LoopMode
	}
	if args.Step != nil {
		params["step"] = *args.Step
	}
}
