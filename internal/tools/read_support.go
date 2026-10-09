package tools

import (
	"context"
	"encoding/json"

	"github.com/vilppuvuorinen/slopfood-godot-mcp/internal/mcp"
)

// ensureReadableScene normalizes scenePath and checks the project and scene
// exist. It is the single pre-flight for every scene/tilemap read.
func ensureReadableScene(project, scenePath string) (mcp.ToolResult, bool) {
	full, err := normalizeReadPath(project, scenePath)
	if err != nil {
		return invalidPathResultFor(err), false
	}
	if result, ok := requireProject(project); !ok {
		return result, false
	}
	if !pathExists(full) {
		return sceneMissingResult(scenePath), false
	}
	return mcp.ToolResult{}, true
}

// readSceneThrough runs ensureReadableScene and then the bespoke read_scene
// payload handling.
func (t *toolset) readSceneThrough(ctx context.Context, scenePath, nodePath string, depth *int) (any, error) {
	if result, ok := ensureReadableScene(t.project, scenePath); !ok {
		return result, nil
	}

	params := map[string]any{"scenePath": scenePath}
	if nodePath != "" {
		params["nodePath"] = nodePath
	}
	if depth != nil {
		params["depth"] = *depth
	}
	stdout, payload, hasPayload, failure, failed := runEngineCall(ctx, func(ctx context.Context) (string, string, int, error) {
		return t.runEngine(ctx, "read_scene", params)
	}, "Failed to read scene", headlessSolutions)
	if failed {
		return failure, nil //nolint:nilerr // the failure is reported through the tool result.
	}

	// A non-error result always carries the declared node tree, so markerless or
	// invalid output is a failure; the raw output stays in the message.
	if hasPayload {
		if json.Valid([]byte(payload)) {
			return mcp.JSONResult(payload), nil
		}
		return mcp.ErrorResult(
			"Failed to read scene: read_scene returned invalid JSON: "+payload,
			headlessSolutions...,
		), nil
	}
	return mcp.ErrorResult(
		"Failed to read scene: read_scene returned no structured output: "+stdout,
		headlessSolutions...,
	), nil
}
