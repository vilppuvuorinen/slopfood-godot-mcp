package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/vilppuvuorinen/go-dot-mcp/internal/mcp"
)

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

func handleReadProjectSettings(_ context.Context, raw json.RawMessage) (any, error) {
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

	content, failure := readProjectGodot(args.ProjectPath)
	if failure != nil {
		return *failure, nil
	}

	data, err := json.MarshalIndent(parseProjectSections(content), "", "  ")
	if err != nil {
		return mcp.ErrorResult("Failed to read project settings: " + err.Error()), nil //nolint:nilerr // the failure is reported through the tool result.
	}
	return mcp.TextResult(string(data)), nil
}
