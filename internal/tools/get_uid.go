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
		Description: "Get the UID for a specific file in a Godot project (for Godot 4.4+)",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"projectPath": map[string]any{
					"type":        "string",
					"description": "Path to the Godot project directory",
				},
				"filePath": map[string]any{
					"type":        "string",
					"description": "Path to the file (relative to project) for which to get the UID",
				},
			},
			"required": []string{"projectPath", "filePath"},
		},
		Handler: handleGetUID,
	}
}

type getUIDArgs struct {
	ProjectPath string `json:"projectPath"`
	FilePath    string `json:"filePath"`
}

func handleGetUID(ctx context.Context, raw json.RawMessage) (any, error) {
	raw, err := aliasParams(raw, parameterMappings)
	if err != nil {
		return nil, err
	}
	var args getUIDArgs
	if len(raw) > 0 && string(raw) != "null" {
		if err = json.Unmarshal(raw, &args); err != nil {
			return nil, fmt.Errorf("invalid arguments: %w", err)
		}
	}
	if args.ProjectPath == "" || args.FilePath == "" {
		return mcp.ErrorResult(
			"Missing required parameters",
			"Provide projectPath and filePath",
		), nil
	}
	fileFull, pathOK := confinePath(args.ProjectPath, args.FilePath)
	if !validPath(args.ProjectPath) || !pathOK {
		return mcp.ErrorResult(
			"Invalid path",
			`Provide valid paths without ".." or other potentially unsafe characters`,
		), nil
	}

	if !pathExists(filepath.Join(args.ProjectPath, "project.godot")) {
		return notAProjectResult(args.ProjectPath), nil
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
	stdout, stderr, exitCode, err := runOperation(ctx, args.ProjectPath, "get_uid", params)
	if err != nil {
		return mcp.ErrorResult( //nolint:nilerr // the failure is reported through the tool result.
			"Failed to get UID: "+err.Error(),
			"Ensure Godot is installed correctly",
			"Check if the GODOT_PATH environment variable is set correctly",
			"Verify the project path is accessible",
		), nil
	}
	if scriptFailed(exitCode) {
		return mcp.ErrorResult(
			"Failed to get UID: "+stderr,
			"Check if the file is a valid Godot resource",
			"Ensure the file path is correct",
		), nil
	}

	if payload, ok := markerJSON(stdout); ok {
		return mcp.TextResult(payload), nil
	}

	message := fmt.Sprintf("UID for %s: %s", args.FilePath, strings.TrimSpace(stdout))
	return mcp.TextResult(message), nil
}
