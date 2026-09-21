package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/vilppuvuorinen/go-dot-mcp/internal/mcp"
)

func listProjects() mcp.Tool {
	return mcp.Tool{
		Name:        "list_projects",
		Description: "List Godot projects in a directory",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"directory": map[string]any{
					"type":        "string",
					"description": "Directory to search for Godot projects",
				},
				"recursive": map[string]any{
					"type":        "boolean",
					"description": "Whether to search recursively (default: false)",
				},
			},
			"required": []string{"directory"},
		},
		Handler: handleListProjects,
	}
}

type listProjectsArgs struct {
	Directory string `json:"directory"`
	Recursive bool   `json:"recursive"`
}

func handleListProjects(_ context.Context, raw json.RawMessage) (any, error) {
	var args listProjectsArgs
	if len(raw) > 0 && string(raw) != "null" {
		if err := json.Unmarshal(raw, &args); err != nil {
			return nil, fmt.Errorf("invalid arguments: %w", err)
		}
	}

	if args.Directory == "" {
		return mcp.ErrorResult(
			"Directory is required",
			"Provide a valid directory path to search for Godot projects",
		), nil
	}
	if !validPath(args.Directory) {
		return mcp.ErrorResult(
			"Invalid directory path",
			`Provide a valid path without ".." or other potentially unsafe characters`,
		), nil
	}
	if !pathExists(args.Directory) {
		return mcp.ErrorResult(
			"Directory does not exist: "+args.Directory,
			"Provide a valid directory path that exists on the system",
		), nil
	}

	projects := findGodotProjects(args.Directory, args.Recursive)
	data, err := json.MarshalIndent(projects, "", "  ")
	if err != nil {
		return mcp.ErrorResult( //nolint:nilerr // the failure is reported through the tool result.
			"Failed to list projects: "+err.Error(),
			"Ensure the directory exists and is accessible",
			"Check if you have permission to read the directory",
		), nil
	}
	return mcp.TextResult(string(data)), nil
}

// pathExists mirrors the reference server's existsSync check: it reports
// whether anything (file or directory) exists at path.
func pathExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
