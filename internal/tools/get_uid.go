package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"

	"github.com/vilppuvuorinen/slopfood-godot-mcp/internal/godot"
	"github.com/vilppuvuorinen/slopfood-godot-mcp/internal/mcp"
)

func getUID() mcp.Tool {
	return mcp.Tool{
		Name:        "get_uid",
		Title:       "Get UID",
		Description: "Get the UID for a specific file in a Godot project",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: mcp.Bool(true), OpenWorldHint: mcp.Bool(false)},
		InputSchema: map[string]any{
			"type":        "object",
			"description": "Requires Godot 4.4 or later, which introduced UID sidecars.",
			"properties": map[string]any{
				"resourcePath": map[string]any{
					"type":        "string",
					"description": "Resource path (relative to project) for which to get the UID",
				},
			},
			"required": []string{"resourcePath"},
		},
		// A file without a .uid sidecar has no UID to return, so it is an error
		// result rather than a second success shape with no uid. The declared
		// payload therefore always carries the UID, and a caller branches only
		// on isError.
		OutputSchema: payloadOutputSchema(map[string]any{
			"file":         map[string]any{"type": "string", "description": "Path to the file, relative to the bound project"},
			"absolutePath": map[string]any{"type": "string", "description": "Absolute host filesystem path to the file"},
			"uid":          map[string]any{"type": "string"},
		}),
		Handler: handleGetUID,
	}
}

type getUIDArgs struct {
	ResourcePath string `json:"resourcePath"`
}

func handleGetUID(ctx context.Context, raw json.RawMessage) (any, error) {
	var args getUIDArgs
	if len(raw) > 0 && string(raw) != "null" {
		if err := json.Unmarshal(raw, &args); err != nil {
			return nil, fmt.Errorf("invalid arguments: %w", err)
		}
	}
	project := projectRoot(ctx)
	if args.ResourcePath == "" {
		return requiredArgsResult("resourcePath"), nil
	}
	fileFull, pathOK := confinePath(project, args.ResourcePath)
	if !validPath(project) || !pathOK {
		return invalidPathResult(), nil
	}

	if !pathExists(filepath.Join(project, "project.godot")) {
		return notAProjectResult(project), nil
	}

	if !pathExists(fileFull) {
		return mcp.ErrorResult(
			"File does not exist: "+args.ResourcePath,
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

	params := map[string]any{"resourcePath": args.ResourcePath}
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

	// A run that yielded no structured payload cannot supply the declared UID,
	// so it is a failure rather than a degraded success with no uid. The raw
	// engine output is kept in the message so the cause is still inspectable.
	if payload, ok := markerJSON(stdout); ok {
		return mcp.JSONResult(payload), nil
	}
	return mcp.ErrorResult(
		"Failed to get UID: get_uid returned no structured output: "+stdout,
		headlessSolutions...,
	), nil
}
