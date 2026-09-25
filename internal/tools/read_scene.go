package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"

	"github.com/vilppuvuorinen/go-dot-mcp/internal/mcp"
)

func readScene() mcp.Tool {
	return mcp.Tool{
		Name:        "read_scene",
		Title:       "Read Scene",
		Description: "Read scene file as JSON node tree (headless)",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: mcp.Bool(true), OpenWorldHint: mcp.Bool(false)},
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"scenePath": map[string]any{
					"type":        "string",
					"description": "Scene file path (relative to project)",
				},
			},
			"required": []string{"scenePath"},
		},
		OutputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"name":   map[string]any{"type": "string"},
				"type":   map[string]any{"type": "string"},
				"script": map[string]any{"type": "string", "description": "Script path, relative to the bound project"},
				"properties": map[string]any{
					"type":                 "object",
					"description":          "Node properties in the tool value contract",
					"additionalProperties": godotValueSchema("JSON-native values are nested; engine-only types are tagged objects such as {\"$type\":\"Vector2\",\"x\":1,\"y\":2}"),
				},
				"groups": map[string]any{
					"type":  "array",
					"items": map[string]any{"type": "string"},
				},
				"children": map[string]any{
					"type":  "array",
					"items": map[string]any{"type": "object"},
				},
				"error": map[string]any{"type": "string"},
				"raw":   map[string]any{"type": "string"},
			},
			// A readable scene yields a node tree, while a scene the engine
			// cannot instantiate yields its raw text so the caller can still
			// inspect it. Advertise both so neither shape is misreported.
			"anyOf": []any{
				map[string]any{"required": []string{"name", "type"}},
				map[string]any{"required": []string{"error", "raw"}},
			},
		},
		Handler: handleReadScene,
	}
}

type readSceneArgs struct {
	ScenePath string `json:"scenePath"`
}

func handleReadScene(ctx context.Context, raw json.RawMessage) (any, error) {
	var args readSceneArgs
	if len(raw) > 0 && string(raw) != "null" {
		if err := json.Unmarshal(raw, &args); err != nil {
			return nil, fmt.Errorf("invalid arguments: %w", err)
		}
	}
	project := projectRoot(ctx)
	if args.ScenePath == "" {
		return mcp.ErrorResult(
			"scenePath is required.",
			"Provide a scene path relative to the bound project",
		), nil
	}
	sceneFull, pathOK := confinePath(project, args.ScenePath)
	if !validPath(project) || !pathOK {
		return mcp.ErrorResult(
			"Invalid path.",
			`Provide a scene path without ".." or other potentially unsafe characters`,
		), nil
	}
	if !pathExists(filepath.Join(project, "project.godot")) {
		return notAProjectResult(project), nil
	}
	if !pathExists(sceneFull) {
		return sceneMissingResult(args.ScenePath), nil
	}

	params := map[string]any{"scenePath": args.ScenePath}
	stdout, stderr, exitCode, err := runOperation(ctx, project, "read_scene", params)
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

	if payload, ok := markerJSON(stdout); ok {
		return mcp.JSONResult(payload), nil
	}

	text := "Scene read output:\n" + stdout
	if stderr != "" {
		text += "\nErrors:\n" + stderr
	}
	return mcp.TextResult(text), nil
}
