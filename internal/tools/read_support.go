package tools

import (
	"encoding/json"
	"path/filepath"

	"github.com/vilppuvuorinen/slopfood-godot-mcp/internal/mcp"
)

// ensureReadableScene normalizes scenePath, checks the project and scene
// exist, and returns the confined absolute path. It is the single pre-flight
// for every scene/tilemap read.
//
//nolint:unparam // the confined path is part of the shared pre-flight contract.
func ensureReadableScene(project, scenePath string) (string, mcp.ToolResult, bool) {
	full, err := normalizeReadPath(project, scenePath)
	if err != nil {
		return "", invalidPathResultFor(err), false
	}
	if !pathExists(filepath.Join(project, "project.godot")) {
		return "", notAProjectResult(project), false
	}
	if !pathExists(full) {
		return "", sceneMissingResult(scenePath), false
	}
	return full, mcp.ToolResult{}, true
}

// readSceneThrough runs ensureReadableScene and then the bespoke read_scene
// payload handling.
func readSceneThrough(ctx Context, project, scenePath, nodePath string, depth *int) (any, error) {
	if _, result, ok := ensureReadableScene(project, scenePath); !ok {
		return result, nil
	}

	params := map[string]any{"scenePath": scenePath}
	if nodePath != "" {
		params["nodePath"] = nodePath
	}
	if depth != nil {
		params["depth"] = *depth
	}
	stdout, stderr, exitCode, err := ctx.Run(project, "read_scene", params)
	if err != nil {
		return mcp.ErrorResult( //nolint:nilerr // the failure is reported through the tool result.
			"Failed to read scene: "+err.Error(),
			headlessSolutions...,
		), nil
	}
	if scriptFailed(exitCode, stderr) {
		return mcp.ErrorResult(
			"Failed to read scene: "+stderr,
			headlessSolutions...,
		), nil
	}

	// A non-error result always carries the declared node tree, so markerless or
	// invalid output is a failure; the raw output stays in the message.
	if payload, ok := markerJSON(stdout); ok {
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
