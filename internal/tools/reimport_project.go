package tools

import (
	"encoding/json"
	"path/filepath"

	"github.com/vilppuvuorinen/slopfood-godot-mcp/internal/mcp"
)

func reimportProject() mcp.Tool {
	return mcp.Tool{
		Name:        "reimport_project",
		Title:       "Reimport Project",
		Description: "Reimport a Godot project by scanning it in the editor, generating .import sidecars and missing resource UIDs",
		Annotations: &mcp.ToolAnnotations{DestructiveHint: mcp.Bool(false), IdempotentHint: mcp.Bool(true), OpenWorldHint: mcp.Bool(false)},
		InputSchema: map[string]any{
			"type":        "object",
			"description": "Runs the editor filesystem scan, which materializes .import sidecars for new sources and applies edits to existing ones.",
			"properties":  map[string]any{},
			"required":    []string{},
		},
		OutputSchema: statusOutputSchema(map[string]any{
			"projectPath": map[string]any{"type": "string"},
		}),
		Handler: withContext(handleReimportProject),
	}
}

func handleReimportProject(ctx Context, _ json.RawMessage) (any, error) {
	project := projectRoot(ctx)
	if !validPath(project) {
		return invalidPathResult(), nil
	}

	if !pathExists(filepath.Join(project, "project.godot")) {
		return notAProjectResult(project), nil
	}

	// Drives EditorFileSystem: ResourceSaver has no UID logic, and the text
	// saver strips existing uid= references without it.
	stdout, stderr, exitCode, err := ctx.Run(project, "reimport_project", nil)
	if err != nil {
		return mcp.ErrorResult( //nolint:nilerr // the failure is reported through the tool result.
			"Failed to reimport project: "+err.Error(),
			"Ensure Godot is installed correctly",
			"Check if the GODOT_PATH environment variable is set correctly",
			"Verify the project path is accessible",
		), nil
	}
	if scriptFailed(exitCode, stderr) {
		return mcp.ErrorResult(
			"Failed to reimport project: "+stderr,
			"Check if the project is valid",
			"Ensure you have write permissions to the project directory",
		), nil
	}

	return statusResult(project, "Project reimported successfully.\n\nOutput: "+stdout, nil), nil
}
