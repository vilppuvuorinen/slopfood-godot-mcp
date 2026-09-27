package tools

import (
	"encoding/json"
	"path/filepath"
	"regexp"

	"github.com/vilppuvuorinen/slopfood-godot-mcp/internal/godot"
	"github.com/vilppuvuorinen/slopfood-godot-mcp/internal/mcp"
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

	// `--import` drives the editor filesystem scan, the only headless mechanism
	// that both writes `.import` sidecars for new sources and reapplies edited
	// ones, and that writes `.uid` sidecars for scripts and shaders. Resaving
	// resources at runtime cannot do it: ResourceSaver.save has no UID logic,
	// and without EditorFileSystem the text saver actively strips existing
	// `uid=` references. Scanning is non-destructive, so this leaves every
	// already-assigned UID and up-to-date artifact untouched.
	stdout, stderr, exitCode, err := godot.Import(ctx, godot.Path(), project)
	// The scan rewrites .import/.uid artifacts and regenerates the global class
	// cache, so a live session's engine-global state is stale whatever the exit
	// code. Invalidate unconditionally so the next session call respawns.
	ctx.Invalidate()
	if err != nil {
		return mcp.ErrorResult( //nolint:nilerr // the failure is reported through the tool result.
			"Failed to reimport project: "+err.Error(),
			"Ensure Godot is installed correctly",
			"Check if the GODOT_PATH environment variable is set correctly",
			"Verify the project path is accessible",
		), nil
	}
	if scriptFailed(exitCode, stderr, importAllowedStderr...) {
		return mcp.ErrorResult(
			"Failed to reimport project: "+stderr,
			"Check if the project is valid",
			"Ensure you have write permissions to the project directory",
		), nil
	}

	return statusResult(project, "Project reimported successfully.\n\nOutput: "+stdout, nil), nil
}
