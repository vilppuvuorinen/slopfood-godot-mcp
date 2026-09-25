package tools

import (
	"context"
	"encoding/json"
	"path/filepath"
	"regexp"

	"github.com/vilppuvuorinen/go-dot-mcp/internal/godot"
	"github.com/vilppuvuorinen/go-dot-mcp/internal/mcp"
)

// importAllowedStderr lists editor-mode stderr lines that do not indicate a
// failed scan. `--import` starts the full editor, which initializes editor-only
// subsystems (the ObjectDB profiler among them) that can fail to set up a
// `user://` directory without affecting the filesystem scan. The exit code
// remains authoritative: a real scan failure calls quit(EXIT_FAILURE).
var importAllowedStderr = []*regexp.Regexp{
	regexp.MustCompile(`Could not create ObjectDB Snapshots directory:`),
	// The engine prints a stack-trace continuation line under the diagnostic
	// above; it is part of the same benign warning.
	regexp.MustCompile(`^at: _get_and_create_snapshot_storage_dir`),
}

func updateProjectUIDs() mcp.Tool {
	return mcp.Tool{
		Name:        "update_project_uids",
		Title:       "Update Project UIDs",
		Description: "Generate missing resource UIDs in a Godot project by scanning it in the editor (for Godot 4.4+)",
		Annotations: &mcp.ToolAnnotations{DestructiveHint: mcp.Bool(false), IdempotentHint: mcp.Bool(true), OpenWorldHint: mcp.Bool(false)},
		InputSchema: map[string]any{
			"type":       "object",
			"properties": map[string]any{},
			"required":   []string{},
		},
		OutputSchema: statusOutputSchema(map[string]any{
			"projectPath": map[string]any{"type": "string"},
		}),
		Handler: handleUpdateProjectUIDs,
	}
}

func handleUpdateProjectUIDs(ctx context.Context, _ json.RawMessage) (any, error) {
	project := projectRoot(ctx)
	if !validPath(project) {
		return invalidPathResult(), nil
	}

	if !pathExists(filepath.Join(project, "project.godot")) {
		return notAProjectResult(project), nil
	}

	version, err := godot.Version(ctx, godot.Path())
	if err != nil {
		return mcp.ErrorResult( //nolint:nilerr // the failure is reported through the tool result.
			"Failed to update project UIDs: "+err.Error(),
			"Ensure Godot is installed correctly",
			"Check if the GODOT_PATH environment variable is set correctly",
			"Verify the project path is accessible",
		), nil
	}
	if !godot.UIDCapable(version) {
		return mcp.ErrorResult(
			"UIDs are only supported in Godot 4.4 or later. Current version: "+version,
			"Upgrade to Godot 4.4 or later to use UIDs",
			"Use resource paths instead of UIDs for this version of Godot",
		), nil
	}

	// `--import` drives the editor filesystem scan, which is the only headless
	// mechanism that writes `.uid` sidecars for scripts and shaders. Resaving
	// resources at runtime cannot do it: ResourceSaver.save has no UID logic,
	// and without EditorFileSystem the text saver actively strips existing
	// `uid=` references. Scanning is non-destructive, so this leaves every
	// already-assigned UID untouched.
	stdout, stderr, exitCode, err := godot.Import(ctx, godot.Path(), project)
	if err != nil {
		return mcp.ErrorResult( //nolint:nilerr // the failure is reported through the tool result.
			"Failed to update project UIDs: "+err.Error(),
			"Ensure Godot is installed correctly",
			"Check if the GODOT_PATH environment variable is set correctly",
			"Verify the project path is accessible",
		), nil
	}
	if scriptFailed(exitCode, stderr, importAllowedStderr...) {
		return mcp.ErrorResult(
			"Failed to update project UIDs: "+stderr,
			"Check if the project is valid",
			"Ensure you have write permissions to the project directory",
		), nil
	}

	return statusResult(project, "Project UIDs updated successfully.\n\nOutput: "+stdout, nil), nil
}
