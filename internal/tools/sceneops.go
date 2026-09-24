package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/vilppuvuorinen/go-dot-mcp/internal/godot"
	"github.com/vilppuvuorinen/go-dot-mcp/internal/godotops"
	"github.com/vilppuvuorinen/go-dot-mcp/internal/mcp"
)

const (
	jsonStartMarker = "<<<GO_DOT_MCP_JSON_START>>>"
	jsonEndMarker   = "<<<GO_DOT_MCP_JSON_END>>>"
)

// markerJSON returns the JSON payload between jsonStartMarker and
// jsonEndMarker, pretty-printed with two-space indentation, and whether both
// markers were present. Invalid JSON is returned verbatim.
func markerJSON(stdout string) (string, bool) {
	startIdx := strings.Index(stdout, jsonStartMarker)
	if startIdx == -1 {
		return "", false
	}
	rest := stdout[startIdx+len(jsonStartMarker):]
	endIdx := strings.Index(rest, jsonEndMarker)
	if endIdx == -1 {
		return "", false
	}
	raw := strings.TrimSpace(rest[:endIdx])
	var buf bytes.Buffer
	if err := json.Indent(&buf, []byte(raw), "", "  "); err != nil {
		return raw, true
	}
	return buf.String(), true
}

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

// headlessSolutions are the shared "Possible solutions" lines for a Godot
// operation that could not run.
var headlessSolutions = []string{
	"Ensure Godot is installed correctly",
	"Check if the GODOT_PATH environment variable is set correctly",
	"Verify the project path is accessible",
}

// runHeadlessOperation runs an operations-script operation and returns the
// reference server's generic result. On success it first attempts structured
// payload extraction via markerJSON, returning the pretty-printed JSON when the
// markers are present; otherwise it falls back to "<operation>
// succeeded.\n\nOutput: <stdout>". On failure it returns "<operation> failed:
// <detail>" with the shared solutions. Callers validate arguments, paths, and
// project/scene existence first.
//
// allowed lists stderr line patterns the operation may emit without it counting
// as a failure; any other stderr line fails the operation even on exit 0.
//
//nolint:unparam // allowed is the per-tool hook for known-benign stderr; no operation needs one yet.
func runHeadlessOperation(ctx context.Context, projectPath, operation string, params map[string]any, allowed ...*regexp.Regexp) (any, error) {
	stdout, stderr, exitCode, err := runOperation(ctx, projectPath, operation, params)
	if err != nil {
		return mcp.ErrorResult(operation+" failed: "+err.Error(), headlessSolutions...), nil //nolint:nilerr // the failure is reported through the tool result.
	}
	if scriptFailed(exitCode, stderr, allowed...) {
		return mcp.ErrorResult(operation+" failed: "+stderr, headlessSolutions...), nil
	}
	if payload, ok := markerJSON(stdout); ok {
		return mcp.TextResult(payload), nil
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
