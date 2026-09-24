package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/vilppuvuorinen/go-dot-mcp/internal/mcp"
)

// projectSettingsReadTimeout bounds the Godot project-load probe.
const projectSettingsReadTimeout = 30 * time.Second

func readProjectSettings() mcp.Tool {
	return mcp.Tool{
		Name:        "read_project_settings",
		Description: "Read project.godot as structured JSON",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"projectPath": map[string]any{
					"type":        "string",
					"description": "Godot project path",
				},
			},
			"required": []string{"projectPath"},
		},
		Handler: handleReadProjectSettings,
	}
}

type readProjectSettingsArgs struct {
	ProjectPath string `json:"projectPath"`
}

func handleReadProjectSettings(ctx context.Context, raw json.RawMessage) (any, error) {
	raw, err := aliasParams(raw, settingsAliases)
	if err != nil {
		return nil, err
	}
	var args readProjectSettingsArgs
	if len(raw) > 0 && string(raw) != "null" {
		if err = json.Unmarshal(raw, &args); err != nil {
			return nil, fmt.Errorf("invalid arguments: %w", err)
		}
	}
	if args.ProjectPath == "" {
		return mcp.ErrorResult("projectPath is required."), nil
	}

	// Internal sanity check: reject an unsafe path or a directory without
	// project.godot before invoking the engine.
	if _, failure := readProjectGodot(args.ProjectPath); failure != nil {
		return *failure, nil
	}

	// Always load the project through the real Godot engine and return the
	// settings it actually applied. A project.godot the engine rejects, or an
	// engine that cannot be run at all, is reported instead of a stale view.
	probeCtx, cancel := context.WithTimeout(ctx, projectSettingsReadTimeout)
	defer cancel()
	probe, probeErr := probeProjectWithGodot(probeCtx, args.ProjectPath)
	if probeErr != nil {
		return mcp.ErrorResult("Failed to read project settings: "+probeErr.Error(), headlessSolutions...), nil //nolint:nilerr // the failure is reported through the tool result.
	}
	if probe.ParseError != "" {
		return mcp.ErrorResult(
			"Godot could not parse project.godot: "+probe.ParseError,
			"Ensure project.godot contains valid Godot ConfigFile syntax",
			"Use modify_project_settings to repair the offending setting",
		), nil
	}
	if probe.Failed() {
		detail := strings.TrimSpace(probe.Stderr)
		if detail == "" {
			detail = fmt.Sprintf("Godot exited with code %d", probe.ExitCode)
		}
		return mcp.ErrorResult("Failed to read project settings: "+detail, headlessSolutions...), nil
	}
	if payload, ok := markerJSON(probe.Stdout); ok {
		return mcp.TextResult(payload), nil
	}
	return mcp.ErrorResult(
		"Failed to read project settings: no settings payload from Godot",
		"Ensure Godot is installed correctly",
		"Check if the GODOT_PATH environment variable is set correctly",
	), nil
}
