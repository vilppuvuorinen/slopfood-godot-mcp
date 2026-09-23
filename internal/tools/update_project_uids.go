package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"

	"github.com/vilppuvuorinen/go-dot-mcp/internal/godot"
	"github.com/vilppuvuorinen/go-dot-mcp/internal/mcp"
)

func updateProjectUIDs() mcp.Tool {
	return mcp.Tool{
		Name:        "update_project_uids",
		Description: "Update UID references in a Godot project by resaving resources (for Godot 4.4+)",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"projectPath": map[string]any{
					"type":        "string",
					"description": "Path to the Godot project directory",
				},
			},
			"required": []string{"projectPath"},
		},
		Handler: handleUpdateProjectUIDs,
	}
}

type updateProjectUIDsArgs struct {
	ProjectPath string `json:"projectPath"`
}

func handleUpdateProjectUIDs(ctx context.Context, raw json.RawMessage) (any, error) {
	raw, err := aliasParams(raw, parameterMappings)
	if err != nil {
		return nil, err
	}
	var args updateProjectUIDsArgs
	if len(raw) > 0 && string(raw) != "null" {
		if err = json.Unmarshal(raw, &args); err != nil {
			return nil, fmt.Errorf("invalid arguments: %w", err)
		}
	}
	if args.ProjectPath == "" {
		return mcp.ErrorResult(
			"Project path is required",
			"Provide a valid path to a Godot project directory",
		), nil
	}
	if !validPath(args.ProjectPath) {
		return mcp.ErrorResult(
			"Invalid project path",
			`Provide a valid path without ".." or other potentially unsafe characters`,
		), nil
	}

	if !pathExists(filepath.Join(args.ProjectPath, "project.godot")) {
		return notAProjectResult(args.ProjectPath), nil
	}

	version, err := godot.Version(ctx, godot.Path())
	if err != nil {
		return mcp.ErrorResult( //nolint:nilerr // the failure is reported through the tool result.
			"Failed to update project UIDs: "+err.Error(),
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

	params := map[string]any{"projectPath": args.ProjectPath}
	stdout, stderr, exitCode, err := runOperation(ctx, args.ProjectPath, "resave_resources", params)
	if err != nil {
		return mcp.ErrorResult( //nolint:nilerr // the failure is reported through the tool result.
			"Failed to update project UIDs: "+err.Error(),
			"Ensure Godot is installed correctly",
			"Check if the GODOT_PATH environment variable is set correctly",
			"Verify the project path is accessible",
		), nil
	}
	if scriptFailed(exitCode) {
		return mcp.ErrorResult(
			"Failed to update project UIDs: "+stderr,
			"Check if the project is valid",
			"Ensure you have write permissions to the project directory",
		), nil
	}

	return mcp.TextResult("Project UIDs updated successfully.\n\nOutput: " + stdout), nil
}
