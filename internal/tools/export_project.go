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
		Annotations: &mcp.ToolAnnotations{DestructiveHint: mcp.Bool(true), OpenWorldHint: mcp.Bool(false)},
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"presetName": map[string]any{
					"type":        "string",
					"description": "Export preset name",
				},
				"outputPath": map[string]any{
					"type":        "string",
					"description": "Output file path for the exported build; relative paths resolve against the project root, absolute paths are allowed",
				},
				"debug": map[string]any{
					"type":        "boolean",
					"description": "Use debug export. Default: false",
				},
			},
			"required": []string{"presetName", "outputPath"},
		},
		OutputSchema: statusOutputSchema(map[string]any{
			"presetName":         map[string]any{"type": "string"},
			"outputPath":         map[string]any{"type": "string", "description": outputPathEchoDescription},
			"resolvedOutputPath": map[string]any{"type": "string", "description": resolvedOutputPathDescription},
			"debug":              map[string]any{"type": "boolean"},
		}),
		Handler: handleExportProject,
	}
}

type exportProjectArgs struct {
	PresetName string `json:"presetName"`
	OutputPath string `json:"outputPath"`
	Debug      bool   `json:"debug"`
}

func handleExportProject(ctx context.Context, raw json.RawMessage) (any, error) {
	var args exportProjectArgs
	if len(raw) > 0 && string(raw) != "null" {
		if err := json.Unmarshal(raw, &args); err != nil {
			return nil, fmt.Errorf("invalid arguments: %w", err)
		}
	}
	project := projectRoot(ctx)
	if args.PresetName == "" || args.OutputPath == "" {
		return requiredArgsResult("presetName", "outputPath"), nil
	}
	if !validPath(project) {
		return invalidPathResult(), nil
	}
	outputPath, ok := confineAbsPath(project, args.OutputPath)
	if !ok {
		return invalidPathResult(), nil
	}
	if !pathExists(filepath.Join(project, "project.godot")) {
		return notAProjectResult(project), nil
	}

	stdout, stderr, exitCode, err := godot.Export(ctx, godot.Path(), project, args.PresetName, outputPath, args.Debug)
	if err != nil {
		return mcp.ErrorResult("Export failed: " + err.Error()), nil //nolint:nilerr // the failure is reported through the tool result.
	}
	if scriptFailed(exitCode, stderr) {
		return mcp.ErrorResult("Export failed: " + stderr), nil
	}
	output := stdout
	if output == "" {
		output = outputPath
	}
	return statusResult(project, "Export succeeded.\n\nOutput: "+output, map[string]any{
		"presetName":         args.PresetName,
		"outputPath":         args.OutputPath,
		"resolvedOutputPath": outputPath,
		"debug":              args.Debug,
	}), nil
}
