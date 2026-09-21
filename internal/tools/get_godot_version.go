package tools

import (
	"context"
	"encoding/json"

	"github.com/vilppuvuorinen/go-dot-mcp/internal/godot"
	"github.com/vilppuvuorinen/go-dot-mcp/internal/mcp"
)

func getGodotVersion() mcp.Tool {
	return mcp.Tool{
		Name:        "get_godot_version",
		Description: "Get the installed Godot version",
		InputSchema: map[string]any{
			"type":       "object",
			"properties": map[string]any{},
			"required":   []string{},
		},
		Handler: handleGetGodotVersion,
	}
}

func handleGetGodotVersion(ctx context.Context, _ json.RawMessage) (any, error) {
	version, err := godot.Version(ctx, godot.Path())
	if err == nil {
		return mcp.TextResult(version), nil
	}
	return mcp.ErrorResult(
		"Failed to get Godot version: "+err.Error(),
		"Ensure Godot is installed correctly",
		"Check if the GODOT_PATH environment variable is set correctly",
	), nil
}
