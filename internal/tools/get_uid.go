package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/vilppuvuorinen/go-dot-mcp/internal/godot"
	"github.com/vilppuvuorinen/go-dot-mcp/internal/mcp"
)

func getUID() mcp.Tool {
	return mcp.Tool{
		Name:        "get_uid",
		Title:       "Get UID",
		Description: "Get the UID for a specific file in a Godot project (for Godot 4.4+)",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: mcp.Bool(true), OpenWorldHint: mcp.Bool(false)},
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"filePath": map[string]any{
					"type":        "string",
					"description": "Path to the file (relative to project) for which to get the UID",
				},
			},
			"required": []string{"filePath"},
		},
		OutputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"file":         map[string]any{"type": "string"},
				"absolutePath": map[string]any{"type": "string"},
				"uid":          map[string]any{"type": "string"},
				"exists":       map[string]any{"type": "boolean"},
				"message":      map[string]any{"type": "string"},
			},
			"required": []string{"file", "absolutePath", "exists"},
		},
		Handler: handleGetUID,
	}
}

type getUIDArgs struct {
	FilePath string `json:"filePath"`
}

func handleGetUID(ctx context.Context, raw json.RawMessage) (any, error) {
	var args getUIDArgs
	if len(raw) > 0 && string(raw) != "null" {
		if err := json.Unmarshal(raw, &args); err != nil {
			return nil, fmt.Errorf("invalid arguments: %w", err)
		}
	}
	project := projectRoot(ctx)
	if args.FilePath == "" {
		return mcp.ErrorResult(
			"Missing required parameters",
			"Provide filePath",
		), nil
	}
	fileFull, pathOK := confinePath(project, args.FilePath)
	if !validPath(project) || !pathOK {
		return mcp.ErrorResult(
			"Invalid path",
			`Provide a valid file path without ".." or other potentially unsafe characters`,
		), nil
	}

	if !pathExists(filepath.Join(project, "project.godot")) {
		return notAProjectResult(project), nil
	}

	if !pathExists(fileFull) {
		return mcp.ErrorResult(
			"File does not exist: "+args.FilePath,
			"Ensure the file path is correct",
		), nil
	}

	version, err := godot.Version(ctx, godot.Path())
	if err != nil {
		return mcp.ErrorResult( //nolint:nilerr // the failure is reported through the tool result.
			"Failed to get UID: "+err.Error(),
			"Ensure Godot is installed correctly",
			"Check if the GODOT_PATH environment variable is set correctly",
			"Verify the project path is accessible",
		), nil
	}
	if !godot.UIDCapable(version) {
		return mcp.ErrorResult(
			"UIDs are only supported in Godot 4.4 or later. Current version: "+version,
			"Upgrade to Godot 4.4 or later to use UIDs",
			"Use resource paths instead of UIDs for this version of Godot",
		), nil
	}

	params := map[string]any{"filePath": args.FilePath}
	stdout, stderr, exitCode, err := runOperation(ctx, project, "get_uid", params)
	if err != nil {
		return mcp.ErrorResult( //nolint:nilerr // the failure is reported through the tool result.
			"Failed to get UID: "+err.Error(),
			"Ensure Godot is installed correctly",
			"Check if the GODOT_PATH environment variable is set correctly",
			"Verify the project path is accessible",
		), nil
	}
	if scriptFailed(exitCode, stderr) {
		return mcp.ErrorResult(
			"Failed to get UID: "+stderr,
			"Check if the file is a valid Godot resource",
			"Ensure the file path is correct",
		), nil
	}

	if payload, ok := markerJSON(stdout); ok {
		return mcp.JSONResult(payload), nil
	}

	message := fmt.Sprintf("UID for %s: %s", args.FilePath, strings.TrimSpace(stdout))
	return mcp.TextResult(message), nil
}
