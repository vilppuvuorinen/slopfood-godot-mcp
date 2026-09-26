package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"time"

	"github.com/vilppuvuorinen/slopfood-godot-mcp/internal/mcp"
)

// projectValidationTimeout bounds the Godot project-load probe.
const projectValidationTimeout = 30 * time.Second

func validateProject() mcp.Tool {
	return mcp.Tool{
		Name:        "validate_project",
		Title:       "Validate Project",
		Description: "Load project.godot with the Godot engine and report whether it parsed",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: mcp.Bool(true), OpenWorldHint: mcp.Bool(false)},
		InputSchema: map[string]any{
			"type":       "object",
			"properties": map[string]any{},
			"required":   []string{},
		},
		OutputSchema: payloadOutputSchema(map[string]any{
			"valid": map[string]any{"type": "boolean"},
			"errors": map[string]any{
				"type":        "array",
				"description": "Engine diagnostics reported while loading project.godot; empty when valid",
				"items":       map[string]any{"type": "string"},
			},
		}),
		Handler: handleValidateProject,
	}
}

// validateProjectResult is validate_project's payload. errors is always
// present, empty on success, so the payload pins its shape.
type validateProjectResult struct {
	Valid  bool     `json:"valid"`
	Errors []string `json:"errors"`
}

func handleValidateProject(ctx context.Context, _ json.RawMessage) (any, error) {
	project := projectRoot(ctx)
	if !validPath(project) {
		return invalidPathResult(), nil
	}
	if !pathExists(filepath.Join(project, "project.godot")) {
		return notAProjectResult(project), nil
	}

	// Loading the project through the engine is the whole check. Godot logs a
	// parse diagnostic to stderr while still exiting 0 when project.godot is
	// malformed, so validity is reported as structured data rather than through
	// isError.
	probeCtx, cancel := context.WithTimeout(ctx, projectValidationTimeout)
	defer cancel()
	probe, err := probeProjectWithGodot(probeCtx, project)
	if err != nil {
		return mcp.ErrorResult("Failed to validate project: "+err.Error(), headlessSolutions...), nil //nolint:nilerr // the failure is reported through the tool result.
	}

	payload := validateProjectResult{
		Valid:  true,
		Errors: projectParseErrors(probe.Stderr),
	}
	if len(payload.Errors) > 0 || probe.ExitCode != 0 {
		payload.Valid = false
		if len(payload.Errors) == 0 {
			payload.Errors = []string{fmt.Sprintf("Godot exited with code %d", probe.ExitCode)}
		}
	}
	if payload.Errors == nil {
		payload.Errors = []string{}
	}

	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return mcp.ErrorResult("Failed to validate project: " + err.Error()), nil //nolint:nilerr // the failure is reported through the tool result.
	}
	return mcp.StructuredResult(string(data), payload), nil
}
