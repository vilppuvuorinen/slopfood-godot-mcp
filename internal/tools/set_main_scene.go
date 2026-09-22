package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/vilppuvuorinen/go-dot-mcp/internal/mcp"
)

func setMainScene() mcp.Tool {
	return mcp.Tool{
		Name:        "set_main_scene",
		Description: "Set the main scene in project.godot",
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
		Handler: handleSetMainScene,
	}
}

type setMainSceneArgs struct {
	ProjectPath string `json:"projectPath"`
	ScenePath   string `json:"scenePath"`
}

// mainScenePattern matches a single run/main_scene="..." entry. The reference
// server uses a non-global regex, so only the first occurrence is replaced.
var mainScenePattern = regexp.MustCompile(`run/main_scene="[^"]*"`)

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
	settingLine := `run/main_scene="` + resPath + `"`

	if loc := mainScenePattern.FindStringIndex(content); loc != nil {
		content = content[:loc[0]] + settingLine + content[loc[1]:]
	} else if idx := strings.Index(content, "[application]"); idx >= 0 {
		content = content[:idx] + "[application]\n\n" + settingLine + content[idx+len("[application]"):]
	} else {
		content += "\n[application]\n\n" + settingLine + "\n"
	}

	if err := writeProjectGodot(args.ProjectPath, content); err != nil {
		return mcp.ErrorResult("set_main_scene failed: " + err.Error()), nil //nolint:nilerr // the failure is reported through the tool result.
	}
	return mcp.TextResult("Main scene set to " + resPath), nil
}
