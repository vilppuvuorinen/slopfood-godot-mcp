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
			"type": "object",
			// The scan is project-wide; there is no per-resource apply, so the
			// tool takes no arguments and applies every pending .import edit.
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

	// The editor session's reimport_project operation drives EditorFileSystem:
	// it materializes missing `.import` sidecars and `.uid` sidecars, registers
	// class_name scripts, and reapplies edited `.import` files, all in the warm
	// engine. Resaving resources at runtime cannot do it: ResourceSaver.save has
	// no UID logic, and without EditorFileSystem the text saver actively strips
	// existing `uid=` references. The reconcile is non-destructive, so it leaves
	// every already-assigned UID and up-to-date artifact untouched. The operation
	// reconciles through the editor in place, so no Invalidate/respawn is needed.
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
