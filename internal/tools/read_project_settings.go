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
		Title:       "Read Project Settings",
		Description: "Read project.godot as structured JSON",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: mcp.Bool(true), OpenWorldHint: mcp.Bool(false)},
		InputSchema: map[string]any{
			"type":       "object",
			"properties": map[string]any{},
			"required":   []string{},
		},
		OutputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"projectPath":  map[string]any{"type": "string", "description": "Absolute host filesystem path to the bound project root"},
				"settingCount": map[string]any{"type": "integer"},
				"sections": map[string]any{
					"type":        "object",
					"description": "Every setting stored in project.godot, grouped by section, with values in the tool value contract",
					"additionalProperties": map[string]any{
						"type":                 "object",
						"additionalProperties": godotValueSchema("A setting value in the tool value contract"),
					},
				},
			},
			"required": []string{"projectPath", "settingCount", "sections"},
		},
		Handler: handleReadProjectSettings,
	}
}

// readProjectSettingsResult is the JSON payload returned on success. Field
// order matches the engine payload's object literal.
type readProjectSettingsResult struct {
	ProjectPath  string                    `json:"projectPath"`
	SettingCount int                       `json:"settingCount"`
	Sections     map[string]map[string]any `json:"sections"`
}

func handleReadProjectSettings(ctx context.Context, _ json.RawMessage) (any, error) {
	project := projectRoot(ctx)

	// Internal sanity check: reject an unsafe path or a directory without
	// project.godot before invoking the engine.
	if _, failure := readProjectGodot(project); failure != nil {
		return *failure, nil
	}

	// Always load the project through the real Godot engine so a project.godot
	// it rejects, or an engine that cannot be run at all, is reported instead
	// of a stale view.
	probeCtx, cancel := context.WithTimeout(ctx, projectSettingsReadTimeout)
	defer cancel()
	probe, probeErr := probeProjectWithGodot(probeCtx, project)
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

	// The engine accepted the file, so report every key physically stored in it
	// through the same typed store the specialized readers use. Returning the
	// engine's applied settings instead would drop a key that is present in the
	// file but equals its engine default, disagreeing with the specialized
	// readers for the same project.
	store, failure := openSettings(project)
	if failure != nil {
		return *failure, nil
	}
	sections, count, err := store.AllSectionValues()
	if err != nil {
		return mcp.ErrorResult("Failed to read project settings: " + err.Error()), nil //nolint:nilerr // the failure is reported through the tool result.
	}
	payload := readProjectSettingsResult{
		ProjectPath:  project,
		SettingCount: count,
		Sections:     sections,
	}
	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return mcp.ErrorResult("Failed to read project settings: " + err.Error()), nil //nolint:nilerr // the failure is reported through the tool result.
	}
	return mcp.StructuredResult(string(data), payload), nil
}
