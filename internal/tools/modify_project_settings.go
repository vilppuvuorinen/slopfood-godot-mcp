package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/vilppuvuorinen/go-dot-mcp/internal/godotconfig"
	"github.com/vilppuvuorinen/go-dot-mcp/internal/mcp"
)

func modifyProjectSettings() mcp.Tool {
	return mcp.Tool{
		Name:        "modify_project_settings",
		Title:       "Modify Project Settings",
		Description: "Modify a project.godot setting",
		Annotations: &mcp.ToolAnnotations{DestructiveHint: mcp.Bool(true), IdempotentHint: mcp.Bool(true), OpenWorldHint: mcp.Bool(false)},
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"projectPath": map[string]any{
					"type":        "string",
					"description": "Godot project path",
				},
				"section": map[string]any{
					"type":        "string",
					"description": `Section in project.godot (e.g., "application", "display", "rendering")`,
				},
				"key": map[string]any{
					"type":        "string",
					"description": `Setting key (e.g., "run/main_scene", "window/size/viewport_width")`,
				},
				"value": map[string]any{
					"type":        "string",
					"description": "Value to set (as a string, will be written as-is)",
				},
			},
			"required": []string{"projectPath", "section", "key", "value"},
		},
		Handler: handleModifyProjectSettings,
	}
}

type modifyProjectSettingsArgs struct {
	ProjectPath string  `json:"projectPath"`
	Section     string  `json:"section"`
	Key         string  `json:"key"`
	Value       *string `json:"value"`
}

func handleModifyProjectSettings(_ context.Context, raw json.RawMessage) (any, error) {
	raw, err := aliasParams(raw, settingsAliases)
	if err != nil {
		return nil, err
	}
	var args modifyProjectSettingsArgs
	if len(raw) > 0 && string(raw) != "null" {
		if err = json.Unmarshal(raw, &args); err != nil {
			return nil, fmt.Errorf("invalid arguments: %w", err)
		}
	}
	if args.ProjectPath == "" || args.Section == "" || args.Key == "" || args.Value == nil {
		return mcp.ErrorResult("projectPath, section, key, and value are required."), nil
	}

	content, failure := readProjectGodot(args.ProjectPath)
	if failure != nil {
		return *failure, nil
	}

	sectionHeader := "[" + godotconfig.EscapeSectionName(args.Section) + "]"
	keyLine := godotconfig.EncodePropertyName(args.Key) + "=" + *args.Value
	if _, valStart, valEnd, ok := findSectionAssignment(content, args.Section, args.Key); ok {
		content = content[:valStart] + *args.Value + content[valEnd:]
	} else if sectionIdx := strings.Index(content, sectionHeader); sectionIdx != -1 {
		insertPos := sectionIdx + len(sectionHeader)
		content = content[:insertPos] + "\n" + keyLine + content[insertPos:]
	} else {
		content += "\n\n" + sectionHeader + "\n\n" + keyLine + "\n"
	}

	if err := writeProjectGodot(args.ProjectPath, content); err != nil {
		return mcp.ErrorResult("Failed to modify project settings: " + err.Error()), nil //nolint:nilerr // the failure is reported through the tool result.
	}

	return mcp.TextResult(fmt.Sprintf("Setting updated: [%s] %s=%s", args.Section, args.Key, *args.Value)), nil
}
