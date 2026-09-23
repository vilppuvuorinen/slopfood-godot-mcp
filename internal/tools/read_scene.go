package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/vilppuvuorinen/go-dot-mcp/internal/mcp"
)

func readScene() mcp.Tool {
	return mcp.Tool{
		Name:        "read_scene",
		Description: "Read scene file as JSON node tree (headless)",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"projectPath": map[string]any{
					"type":        "string",
					"description": "Godot project path",
				},
				"scenePath": map[string]any{
					"type":        "string",
					"description": "Scene file path (relative to project)",
				},
			},
			"required": []string{"projectPath", "scenePath"},
		},
		Handler: handleReadScene,
	}
}

type readSceneArgs struct {
	ProjectPath string `json:"projectPath"`
	ScenePath   string `json:"scenePath"`
}

func handleReadScene(ctx context.Context, raw json.RawMessage) (any, error) {
	raw, err := aliasParams(raw, parameterMappings)
	if err != nil {
		return nil, err
	}
	var args readSceneArgs
	if len(raw) > 0 && string(raw) != "null" {
		if err = json.Unmarshal(raw, &args); err != nil {
			return nil, fmt.Errorf("invalid arguments: %w", err)
		}
	}
	if args.ProjectPath == "" || args.ScenePath == "" {
		return mcp.ErrorResult(
			"projectPath and scenePath are required.",
			"Provide valid paths for both the project and the scene",
		), nil
	}
	sceneFull, pathOK := confinePath(args.ProjectPath, args.ScenePath)
	if !validPath(args.ProjectPath) || !pathOK {
		return mcp.ErrorResult(
			"Invalid path.",
			`Provide valid paths without ".." or other potentially unsafe characters`,
		), nil
	}
	if !pathExists(filepath.Join(args.ProjectPath, "project.godot")) {
		return notAProjectResult(args.ProjectPath), nil
	}
	if !pathExists(sceneFull) {
		return sceneMissingResult(args.ScenePath), nil
	}

	params := map[string]any{"scenePath": args.ScenePath}
	stdout, stderr, exitCode, err := runOperation(ctx, args.ProjectPath, "read_scene", params)
	if err != nil {
		return mcp.ErrorResult( //nolint:nilerr // the failure is reported through the tool result.
			"Failed to read scene: "+err.Error(),
			headlessSolutions...,
		), nil
	}
	if scriptFailed(exitCode) {
		return mcp.ErrorResult(
			"Failed to read scene: "+stderr,
			headlessSolutions...,
		), nil
	}

	const startMarker = "SCENE_JSON_START"
	const endMarker = "SCENE_JSON_END"
	startIdx := strings.Index(stdout, startMarker)
	endIdx := strings.Index(stdout, endMarker)
	if startIdx != -1 && endIdx != -1 {
		jsonStr := strings.TrimSpace(stdout[startIdx+len(startMarker) : endIdx])
		var pretty bytes.Buffer
		if indentErr := json.Indent(&pretty, []byte(jsonStr), "", "  "); indentErr == nil {
			return mcp.TextResult(pretty.String()), nil
		}
		return mcp.TextResult("Raw scene data:\n" + jsonStr), nil
	}

	text := "Scene read output:\n" + stdout
	if stderr != "" {
		text += "\nErrors:\n" + stderr
	}
	return mcp.TextResult(text), nil
}
