package tools

import (
	"encoding/json"
	"fmt"
	"path/filepath"

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
	sceneFull, pathOK := confinePath(project, args.ScenePath)
	if !validPath(project) || !pathOK {
		return invalidPathResult(), nil
	}
	if !pathExists(filepath.Join(project, "project.godot")) {
		return notAProjectResult(project), nil
	}
	if !pathExists(sceneFull) {
		return sceneMissingResult(args.ScenePath), nil
	}

	params := map[string]any{"scenePath": args.ScenePath}
	if args.NodePath != "" {
		params["nodePath"] = args.NodePath
	}
	if args.Depth != nil {
		params["depth"] = *args.Depth
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

	// A run that yielded no valid node tree is a failure, not a second success
	// shape: a non-error result always carries the declared node tree, so a
	// caller branches only on isError. The raw engine output is kept in the
	// message so the cause is still inspectable.
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
