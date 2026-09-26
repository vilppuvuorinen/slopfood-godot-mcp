package tools

import (
	"context"
	"encoding/json"

	"github.com/vilppuvuorinen/slopfood-godot-mcp/internal/godot"
	"github.com/vilppuvuorinen/slopfood-godot-mcp/internal/mcp"
)

func getGodotVersion() mcp.Tool {
	return mcp.Tool{
		Name:        "get_godot_version",
		Title:       "Get Godot Version",
		Description: "Get the installed Godot version",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: mcp.Bool(true), OpenWorldHint: mcp.Bool(false)},
		InputSchema: map[string]any{
			"type":       "object",
			"properties": map[string]any{},
			"required":   []string{},
		},
		OutputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"version": map[string]any{"type": "string"},
			},
			"required": []string{"version"},
		},
		Handler: handleGetGodotVersion,
	}
}

func handleGetGodotVersion(ctx context.Context, _ json.RawMessage) (any, error) {
	version, err := godot.Version(ctx, godot.Path())
	if err == nil {
		return mcp.StructuredResult(version, map[string]any{"version": version}), nil
	}
	return mcp.ErrorResult(
		"Failed to get Godot version: "+err.Error(),
		"Ensure Godot is installed correctly",
		"Check if the GODOT_PATH environment variable is set correctly",
	), nil
}
