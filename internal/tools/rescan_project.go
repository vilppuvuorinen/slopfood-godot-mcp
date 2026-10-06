package tools

import (
	"context"
	"encoding/json"

	"github.com/vilppuvuorinen/slopfood-godot-mcp/internal/mcp"
)

func rescanProject() mcp.Tool {
	return mcp.Tool{
		Name:        "rescan_project",
		Title:       "Rescan Project",
		Description: "Rescan a Godot project in the editor, generating .import sidecars and missing resource UIDs",
		Annotations: &mcp.ToolAnnotations{DestructiveHint: mcp.Bool(true), IdempotentHint: mcp.Bool(true), OpenWorldHint: mcp.Bool(false)},
		InputSchema: map[string]any{
			"type":        "object",
			"description": "Runs the editor filesystem scan, which materializes .import sidecars for new sources and applies edits to existing ones.",
			"properties":  map[string]any{},
			"required":    []string{},
		},
		OutputSchema: statusOutputSchema(map[string]any{
			"projectPath": map[string]any{"type": "string"},
		}),
		Handler: handleRescanProject,
	}
}

func handleRescanProject(ctx context.Context, _ json.RawMessage) (any, error) {
	project := projectRoot(ctx)
	if result, ok := requireProject(project); !ok {
		return result, nil
	}

	// Drives EditorFileSystem: ResourceSaver has no UID logic, and the text
	// saver strips existing uid= references without it.
	stdout, stderr, exitCode, err := runEngine(ctx, project, "rescan_project", nil)
	if err != nil {
		return mcp.ErrorResult( //nolint:nilerr // the failure is reported through the tool result.
			"Failed to rescan project: "+err.Error(),
			"Ensure Godot is installed correctly",
			"Check if the GODOT_PATH environment variable is set correctly",
			"Verify the project path is accessible",
		), nil
	}
	if scriptFailed(exitCode, stderr) {
		return mcp.ErrorResult(
			"Failed to rescan project: "+stderr,
			"Check if the project is valid",
			"Ensure you have write permissions to the project directory",
		), nil
	}

	return statusResult(project, "Project rescanned successfully.\n\nOutput: "+stdout, nil), nil
}
