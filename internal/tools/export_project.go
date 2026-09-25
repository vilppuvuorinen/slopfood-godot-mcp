package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"

	"github.com/vilppuvuorinen/go-dot-mcp/internal/godot"
	"github.com/vilppuvuorinen/go-dot-mcp/internal/mcp"
)

func exportProject() mcp.Tool {
	return mcp.Tool{
		Name:        "export_project",
		Title:       "Export Project",
		Description: "Export a Godot project using a preset",
		Annotations: &mcp.ToolAnnotations{DestructiveHint: mcp.Bool(false), OpenWorldHint: mcp.Bool(false)},
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"projectPath": map[string]any{
					"type":        "string",
					"description": "Godot project path",
				},
				"presetName": map[string]any{
					"type":        "string",
					"description": "Export preset name",
				},
				"outputPath": map[string]any{
					"type":        "string",
					"description": "Output file path for the exported build",
				},
				"debug": map[string]any{
					"type":        "boolean",
					"description": "Use debug export. Default: false",
				},
			},
			"required": []string{"projectPath", "presetName", "outputPath"},
		},
		OutputSchema: statusOutputSchema(map[string]any{
			"presetName": map[string]any{"type": "string"},
			"outputPath": map[string]any{"type": "string"},
			"debug":      map[string]any{"type": "boolean"},
			"output":     map[string]any{"type": "string"},
		}),
		Handler: handleExportProject,
	}
}

type exportProjectArgs struct {
	ProjectPath string `json:"projectPath"`
	PresetName  string `json:"presetName"`
	OutputPath  string `json:"outputPath"`
	Debug       bool   `json:"debug"`
}

func handleExportProject(ctx context.Context, raw json.RawMessage) (any, error) {
	var args exportProjectArgs
	if len(raw) > 0 && string(raw) != "null" {
		if err := json.Unmarshal(raw, &args); err != nil {
			return nil, fmt.Errorf("invalid arguments: %w", err)
		}
	}
	if args.ProjectPath == "" || args.PresetName == "" || args.OutputPath == "" {
		return mcp.ErrorResult("projectPath, presetName, and outputPath are required."), nil
	}
	if !validPath(args.ProjectPath) {
		return mcp.ErrorResult("Invalid project path."), nil
	}
	if _, ok := confineAbsPath(args.ProjectPath, args.OutputPath); !ok {
		return mcp.ErrorResult("Invalid output path."), nil
	}
	if !pathExists(filepath.Join(args.ProjectPath, "project.godot")) {
		return mcp.ErrorResult("Not a valid Godot project: " + args.ProjectPath), nil
	}

	stdout, stderr, exitCode, err := godot.Export(ctx, godot.Path(), args.ProjectPath, args.PresetName, args.OutputPath, args.Debug)
	if err != nil {
		return mcp.ErrorResult("Export failed: " + err.Error()), nil //nolint:nilerr // the failure is reported through the tool result.
	}
	if scriptFailed(exitCode, stderr) {
		return mcp.ErrorResult("Export failed: " + stderr), nil
	}
	output := stdout
	if output == "" {
		output = args.OutputPath
	}
	return statusResult("Export succeeded.\n\nOutput: "+output, map[string]any{
		"presetName": args.PresetName,
		"outputPath": args.OutputPath,
		"debug":      args.Debug,
		"output":     output,
	}), nil
}
