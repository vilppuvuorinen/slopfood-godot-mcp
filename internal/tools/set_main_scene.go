package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/vilppuvuorinen/go-dot-mcp/internal/godotconfig"
	"github.com/vilppuvuorinen/go-dot-mcp/internal/mcp"
)

func setMainScene() mcp.Tool {
	return mcp.Tool{
		Name:        "set_main_scene",
		Title:       "Set Main Scene",
		Description: "Set the main scene in project.godot",
		Annotations: &mcp.ToolAnnotations{DestructiveHint: mcp.Bool(false), IdempotentHint: mcp.Bool(true), OpenWorldHint: mcp.Bool(false)},
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"projectPath": map[string]any{
					"type":        "string",
					"description": "Godot project path",
				},
				"scenePath": map[string]any{
					"type":        "string",
					"description": "Scene path (relative to project)",
				},
			},
			"required": []string{"projectPath", "scenePath"},
		},
		OutputSchema: statusOutputSchema(map[string]any{
			"scenePath": map[string]any{"type": "string"},
		}),
		Handler: handleSetMainScene,
	}
}

type setMainSceneArgs struct {
	ProjectPath string `json:"projectPath"`
	ScenePath   string `json:"scenePath"`
}

func handleSetMainScene(_ context.Context, raw json.RawMessage) (any, error) {
	raw, err := aliasParams(raw, settingsAliases)
	if err != nil {
		return nil, err
	}
	var args setMainSceneArgs
	if len(raw) > 0 && string(raw) != "null" {
		if err = json.Unmarshal(raw, &args); err != nil {
			return nil, fmt.Errorf("invalid arguments: %w", err)
		}
	}
	if args.ProjectPath == "" || args.ScenePath == "" {
		return mcp.ErrorResult("projectPath and scenePath are required."), nil
	}

	content, failure := readProjectGodot(args.ProjectPath)
	if failure != nil {
		return *failure, nil
	}

	resPath := args.ScenePath
	if !strings.HasPrefix(resPath, "res://") {
		resPath = "res://" + resPath
	}
	settingLine := godotconfig.EncodePropertyName("run/main_scene") + "=" + godotconfig.FormatValue(godotconfig.String(resPath))

	if updated, ok := setSectionValue(content, "application", "run/main_scene", godotconfig.String(resPath)); ok {
		content = updated
	} else if idx := strings.Index(content, "[application]"); idx >= 0 {
		content = content[:idx] + "[application]\n\n" + settingLine + content[idx+len("[application]"):]
	} else {
		content += "\n[application]\n\n" + settingLine + "\n"
	}

	if err := writeProjectGodot(args.ProjectPath, content); err != nil {
		return mcp.ErrorResult("set_main_scene failed: " + err.Error()), nil //nolint:nilerr // the failure is reported through the tool result.
	}
	return statusResult("Main scene set to "+resPath, map[string]any{
		"scenePath": resPath,
	}), nil
}
