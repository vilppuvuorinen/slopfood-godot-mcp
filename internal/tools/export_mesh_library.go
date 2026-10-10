package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/vilppuvuorinen/slopfood-godot-mcp/internal/godot"
	"github.com/vilppuvuorinen/slopfood-godot-mcp/internal/mcp"
)

func (t *toolset) exportMeshLibrary() mcp.Tool {
	return mcp.Tool{
		Name:        "export_mesh_library",
		Title:       "Export Mesh Library",
		Description: "Export a scene as a MeshLibrary resource, overwriting the output file if it exists",
		Annotations: &mcp.ToolAnnotations{DestructiveHint: mcp.Bool(true), IdempotentHint: mcp.Bool(true), OpenWorldHint: mcp.Bool(false)},
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"scenePath": map[string]any{
					"type":        "string",
					"description": "Path to the scene file (.tscn) to export",
				},
				"outputPath": map[string]any{
					"type":        "string",
					"description": "Path where the mesh library (.res) will be saved; relative paths resolve against the project root, absolute paths are allowed",
				},
				"meshItemNames": map[string]any{
					"type":        "array",
					"items":       map[string]any{"type": "string"},
					"description": "Names of specific mesh items to include. Default: all",
				},
			},
			"required": []string{"scenePath", "outputPath"},
		},
		OutputSchema: statusOutputSchema(map[string]any{
			"scenePath":          map[string]any{"type": "string"},
			"outputPath":         map[string]any{"type": "string", "description": outputPathEchoDescription},
			"resolvedOutputPath": map[string]any{"type": "string", "description": resolvedOutputPathDescription},
		}),
		Handler: t.handleExportMeshLibrary,
	}
}

type exportMeshLibraryArgs struct {
	ScenePath     string   `json:"scenePath"`
	OutputPath    string   `json:"outputPath"`
	MeshItemNames []string `json:"meshItemNames"`
}

func (t *toolset) handleExportMeshLibrary(ctx context.Context, raw json.RawMessage) (any, error) {
	args, err := decodeArgs[exportMeshLibraryArgs](raw)
	if err != nil {
		return nil, err
	}
	project := t.project
	if result, missing := requireFields("scenePath", args.ScenePath, "outputPath", args.OutputPath); missing {
		return result, nil
	}
	if result, ok := requireProject(project); !ok {
		return result, nil
	}
	sceneFull, sceneErr := confinePath(project, args.ScenePath)
	if sceneErr != nil {
		return invalidPathResultFor(sceneErr), nil
	}
	outputPath, outputErr := confineAbsPath(project, args.OutputPath)
	if outputErr != nil {
		return invalidPathResultFor(outputErr), nil
	}
	if !pathExists(sceneFull) {
		return sceneMissingResult(args.ScenePath), nil
	}

	params := map[string]any{
		"scenePath":  args.ScenePath,
		"outputPath": outputPath,
	}
	if len(args.MeshItemNames) > 0 {
		params["meshItemNames"] = args.MeshItemNames
	}

	stdout, _, _, failure, failed := runEngineCall(ctx, func(ctx context.Context) (godot.RunResult, error) {
		return t.runEngine(ctx, "export_mesh_library", params)
	}, "Failed to export mesh library", headlessSolutions, []string{
		"Check if the scene contains valid 3D meshes",
		"Ensure the output path is valid",
		"Verify the scene file is valid",
	})
	if failed {
		return failure, nil //nolint:nilerr // the failure is reported through the tool result.
	}

	message := fmt.Sprintf("MeshLibrary exported successfully to: %s\n\nOutput: %s", outputPath, stdout)
	return statusResult(project, message, map[string]any{
		"scenePath":          args.ScenePath,
		"outputPath":         args.OutputPath,
		"resolvedOutputPath": outputPath,
	}), nil
}
