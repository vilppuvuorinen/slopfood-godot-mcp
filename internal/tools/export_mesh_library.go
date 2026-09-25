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
		Title:       "Export Mesh Library",
		Description: "Export a scene as a MeshLibrary resource",
		Annotations: &mcp.ToolAnnotations{DestructiveHint: mcp.Bool(true), OpenWorldHint: mcp.Bool(false)},
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
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
			"required": []string{"scenePath", "outputPath"},
		},
		OutputSchema: statusOutputSchema(map[string]any{
			"scenePath":  map[string]any{"type": "string"},
			"outputPath": map[string]any{"type": "string"},
			"output":     map[string]any{"type": "string"},
		}),
		Handler: handleExportMeshLibrary,
	}
}

type exportMeshLibraryArgs struct {
	ScenePath     string   `json:"scenePath"`
	OutputPath    string   `json:"outputPath"`
	MeshItemNames []string `json:"meshItemNames"`
}

func handleExportMeshLibrary(ctx context.Context, raw json.RawMessage) (any, error) {
	var args exportMeshLibraryArgs
	if len(raw) > 0 && string(raw) != "null" {
		if err := json.Unmarshal(raw, &args); err != nil {
			return nil, fmt.Errorf("invalid arguments: %w", err)
		}
	}
	project := projectRoot(ctx)
	if args.ScenePath == "" || args.OutputPath == "" {
		return mcp.ErrorResult(
			"Missing required parameters",
			"Provide scenePath and outputPath",
		), nil
	}
	sceneFull, sceneOK := confinePath(project, args.ScenePath)
	if !validPath(project) || !validPath(args.OutputPath) || !sceneOK {
		return mcp.ErrorResult(
			"Invalid path",
			`Provide valid paths without ".." or other potentially unsafe characters`,
		), nil
	}

	if !pathExists(filepath.Join(project, "project.godot")) {
		return notAProjectResult(project), nil
	}

	if !pathExists(sceneFull) {
		return sceneMissingResult(args.ScenePath), nil
	}

	params := map[string]any{
		"scenePath":  args.ScenePath,
		"outputPath": args.OutputPath,
	}
	if len(args.MeshItemNames) > 0 {
		params["meshItemNames"] = args.MeshItemNames
	}

	stdout, stderr, exitCode, err := runOperation(ctx, project, "export_mesh_library", params)
	if err != nil {
		return mcp.ErrorResult( //nolint:nilerr // the failure is reported through the tool result.
			"Failed to export mesh library: "+err.Error(),
			"Ensure Godot is installed correctly",
			"Check if the GODOT_PATH environment variable is set correctly",
			"Verify the project path is accessible",
		), nil
	}
	if scriptFailed(exitCode, stderr) {
		return mcp.ErrorResult(
			"Failed to export mesh library: "+stderr,
			"Check if the scene contains valid 3D meshes",
			"Ensure the output path is valid",
			"Verify the scene file is valid",
		), nil
	}

	message := fmt.Sprintf("MeshLibrary exported successfully to: %s\n\nOutput: %s", args.OutputPath, stdout)
	return statusResult(project, message, map[string]any{
		"scenePath":  args.ScenePath,
		"outputPath": args.OutputPath,
		"output":     stdout,
	}), nil
}
