package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/vilppuvuorinen/go-dot-mcp/internal/godot"
	"github.com/vilppuvuorinen/go-dot-mcp/internal/godotops"
	"github.com/vilppuvuorinen/go-dot-mcp/internal/mcp"
)

// runOperation executes an operations-script operation against a project,
// converting camelCase parameters to snake_case first.
func runOperation(ctx context.Context, projectPath, operation string, params map[string]any) (stdout, stderr string, exitCode int, err error) {
	scriptPath, err := godotops.OperationsScript()
	if err != nil {
		return "", "", 0, fmt.Errorf("materialize operations script: %w", err)
	}
	stdout, stderr, exitCode, err = godot.RunOperation(ctx, godot.Path(), projectPath, scriptPath, operation, toSnakeCase(params))
	if err != nil {
		return stdout, stderr, exitCode, fmt.Errorf("run godot operation: %w", err)
	}
	return stdout, stderr, exitCode, nil
}

// scriptFailed reports whether the operations script failed. The script calls
// quit(1) on every error branch and quit() on success, so a non-zero exit is
// the authoritative signal. The "Failed to" stderr marker is kept as a
// secondary check for error paths that print a failure but still exit cleanly.
func scriptFailed(stderr string, exitCode int) bool {
	return exitCode != 0 || strings.Contains(stderr, "Failed to")
}

// headlessSolutions are the shared "Possible solutions" lines for a Godot
// operation that could not run.
var headlessSolutions = []string{
	"Ensure Godot is installed correctly",
	"Check if the GODOT_PATH environment variable is set correctly",
	"Verify the project path is accessible",
}

// runHeadlessOperation runs an operations-script operation and returns the
// reference server's generic result: "<operation> succeeded.\n\nOutput:
// <stdout>" on success, or "<operation> failed: <detail>" with the shared
// solutions on failure. Callers validate arguments, paths, and project/scene
// existence first.
func runHeadlessOperation(ctx context.Context, projectPath, operation string, params map[string]any) (any, error) {
	stdout, stderr, exitCode, err := runOperation(ctx, projectPath, operation, params)
	if err != nil {
		return mcp.ErrorResult(operation+" failed: "+err.Error(), headlessSolutions...), nil //nolint:nilerr // the failure is reported through the tool result.
	}
	if scriptFailed(stderr, exitCode) {
		return mcp.ErrorResult(operation+" failed: "+stderr, headlessSolutions...), nil
	}
	return mcp.TextResult(operation + " succeeded.\n\nOutput: " + stdout), nil
}

// sceneMissingResult is the shared error for a scene file that does not exist,
// matching the reference server's message and solutions.
func sceneMissingResult(scenePath string) mcp.ToolResult {
	return mcp.ErrorResult(
		"Scene file does not exist: "+scenePath,
		"Ensure the scene path is correct",
		"Use create_scene to create a new scene first",
	)
}

// notAProjectResult is the shared error for a projectPath without a
// project.godot file, matching the reference server's message and solutions.
func notAProjectResult(projectPath string) mcp.ToolResult {
	return mcp.ErrorResult(
		"Not a valid Godot project: "+projectPath,
		"Ensure the path points to a directory containing a project.godot file",
		"Use list_projects to find valid Godot projects",
	)
}
