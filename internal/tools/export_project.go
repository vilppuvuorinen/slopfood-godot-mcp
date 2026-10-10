package tools

import (
	"context"
	"encoding/json"

	"github.com/vilppuvuorinen/slopfood-godot-mcp/internal/godot"
	"github.com/vilppuvuorinen/slopfood-godot-mcp/internal/mcp"
)

func (t *toolset) exportProject() mcp.Tool {
	return mcp.Tool{
		Name:        "export_project",
		Title:       "Export Project",
		Description: "Export a Godot project using a preset from export_presets.cfg, overwriting the output file if it exists",
		Annotations: &mcp.ToolAnnotations{DestructiveHint: mcp.Bool(true), IdempotentHint: mcp.Bool(true), OpenWorldHint: mcp.Bool(false)},
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
		Handler: t.handleExportProject,
	}
}

type exportProjectArgs struct {
	PresetName string `json:"presetName"`
	OutputPath string `json:"outputPath"`
	Debug      bool   `json:"debug"`
}

func (t *toolset) handleExportProject(ctx context.Context, raw json.RawMessage) (any, error) {
	args, err := decodeArgs[exportProjectArgs](raw)
	if err != nil {
		return nil, err
	}
	project := t.project
	if result, missing := requireFields("presetName", args.PresetName, "outputPath", args.OutputPath); missing {
		return result, nil
	}
	if result, ok := requireProject(project); !ok {
		return result, nil
	}
	outputPath, err := confineAbsPath(project, args.OutputPath)
	if err != nil {
		return invalidPathResultFor(err), nil
	}

	res, failure := runEngineCall(ctx, func(ctx context.Context) (godot.RunResult, error) {
		out, errOut, code, err := t.engine.Export(ctx, project, args.PresetName, outputPath, args.Debug)
		return godot.RunResult{Stdout: out, Stderr: errOut, ExitCode: code}, err //nolint:wrapcheck // the caller renders the export error.
	}, "Export failed", nil, nil)
	if failure != nil {
		return *failure, nil //nolint:nilerr // the failure is reported through the tool result.
	}
	output := res.Stdout
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
