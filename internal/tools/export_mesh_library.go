package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"

	"github.com/vilppuvuorinen/go-dot-mcp/internal/mcp"
)

func exportMeshLibrary() mcp.Tool {
	return mcp.Tool{
		Name:        "export_mesh_library",
		Description: "Export a scene as a MeshLibrary resource",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"projectPath": map[string]any{
					"type":        "string",
					"description": "Path to the Godot project directory",
				},
				"scenePath": map[string]any{
					"type":        "string",
					"description": "Path to the scene file (.tscn) to export",
				},
				"outputPath": map[string]any{
					"type":        "string",
					"description": "Path where the mesh library (.res) will be saved",
				},
				"meshItemNames": map[string]any{
					"type":        "array",
					"items":       map[string]any{"type": "string"},
					"description": "Optional: Names of specific mesh items to include (defaults to all)",
				},
			},
			"required": []string{"projectPath", "scenePath", "outputPath"},
		},
		Handler: handleExportMeshLibrary,
	}
}

type exportMeshLibraryArgs struct {
	ProjectPath   string   `json:"projectPath"`
	ScenePath     string   `json:"scenePath"`
	OutputPath    string   `json:"outputPath"`
	MeshItemNames []string `json:"meshItemNames"`
}

func handleExportMeshLibrary(ctx context.Context, raw json.RawMessage) (any, error) {
	raw, err := aliasParams(raw, parameterMappings)
	if err != nil {
		return nil, err
	}
	var args exportMeshLibraryArgs
	if len(raw) > 0 && string(raw) != "null" {
		if err = json.Unmarshal(raw, &args); err != nil {
			return nil, fmt.Errorf("invalid arguments: %w", err)
		}
	}
	if args.ProjectPath == "" || args.ScenePath == "" || args.OutputPath == "" {
		return mcp.ErrorResult(
			"Missing required parameters",
			"Provide projectPath, scenePath, and outputPath",
		), nil
	}
	if !validPath(args.ProjectPath) || !validPath(args.ScenePath) || !validPath(args.OutputPath) {
		return mcp.ErrorResult(
			"Invalid path",
			`Provide valid paths without ".." or other potentially unsafe characters`,
		), nil
	}

	if !pathExists(filepath.Join(args.ProjectPath, "project.godot")) {
		return notAProjectResult(args.ProjectPath), nil
	}

	if !pathExists(filepath.Join(args.ProjectPath, args.ScenePath)) {
		return sceneMissingResult(args.ScenePath), nil
	}

	params := map[string]any{
		"scenePath":  args.ScenePath,
		"outputPath": args.OutputPath,
	}
	if len(args.MeshItemNames) > 0 {
		params["meshItemNames"] = args.MeshItemNames
	}

	stdout, stderr, err := runOperation(ctx, args.ProjectPath, "export_mesh_library", params)
	if err != nil {
		return mcp.ErrorResult( //nolint:nilerr // the failure is reported through the tool result.
			"Failed to export mesh library: "+err.Error(),
			"Ensure Godot is installed correctly",
			"Check if the GODOT_PATH environment variable is set correctly",
			"Verify the project path is accessible",
		), nil
	}
	if scriptFailed(stderr) {
		return mcp.ErrorResult(
			"Failed to export mesh library: "+stderr,
			"Check if the scene contains valid 3D meshes",
			"Ensure the output path is valid",
			"Verify the scene file is valid",
		), nil
	}

	message := fmt.Sprintf("MeshLibrary exported successfully to: %s\n\nOutput: %s", args.OutputPath, stdout)
	return mcp.TextResult(message), nil
}
