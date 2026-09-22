package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/vilppuvuorinen/go-dot-mcp/internal/mcp"
)

func managePlugins() mcp.Tool {
	return mcp.Tool{
		Name:        "manage_plugins",
		Description: "List/enable/disable editor plugins",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"projectPath": map[string]any{
					"type":        "string",
					"description": "Godot project path",
				},
				"action": map[string]any{
					"type":        "string",
					"description": "Action: list, enable, disable",
				},
				"pluginName": map[string]any{
					"type":        "string",
					"description": "Plugin name",
				},
			},
			"required": []string{"projectPath", "action"},
		},
		Handler: handleManagePlugins,
	}
}

type managePluginsArgs struct {
	ProjectPath string `json:"projectPath"`
	Action      string `json:"action"`
	PluginName  string `json:"pluginName"`
}

// pluginEnabledPattern matches every "<name>/enabled=true" entry in a
// project.godot file, mirroring the reference server's /(\w+)\/enabled=true/g.
var pluginEnabledPattern = regexp.MustCompile(`(\w+)/enabled=true`)

// pluginList is the JSON document returned by the list action.
type pluginList struct {
	Enabled   []string `json:"enabled"`
	Available []string `json:"available"`
}

func handleManagePlugins(_ context.Context, raw json.RawMessage) (any, error) {
	raw, err := aliasParams(raw, settingsAliases)
	if err != nil {
		return nil, err
	}
	var args managePluginsArgs
	if len(raw) > 0 && string(raw) != "null" {
		if err = json.Unmarshal(raw, &args); err != nil {
			return nil, fmt.Errorf("invalid arguments: %w", err)
		}
	}
	if args.ProjectPath == "" || args.Action == "" {
		return mcp.ErrorResult("projectPath and action are required."), nil
	}

	content, failure := readProjectGodot(args.ProjectPath)
	if failure != nil {
		return *failure, nil
	}

	switch args.Action {
	case "list":
		enabled := make([]string, 0)
		for _, match := range pluginEnabledPattern.FindAllStringSubmatch(content, -1) {
			enabled = append(enabled, match[1])
		}

		available := make([]string, 0)
		entries, err := os.ReadDir(filepath.Join(args.ProjectPath, "addons"))
		if err != nil {
			if !os.IsNotExist(err) {
				return mcp.ErrorResult("manage_plugins failed: " + err.Error()), nil //nolint:nilerr // the failure is reported through the tool result.
			}
		} else {
			for _, entry := range entries {
				if entry.IsDir() {
					available = append(available, entry.Name())
				}
			}
		}

		encoded, err := json.MarshalIndent(pluginList{Enabled: enabled, Available: available}, "", "  ")
		if err != nil {
			return mcp.ErrorResult("manage_plugins failed: " + err.Error()), nil //nolint:nilerr // the failure is reported through the tool result.
		}
		return mcp.TextResult(string(encoded)), nil
	case "enable", "disable":
		if args.PluginName == "" {
			return mcp.ErrorResult("pluginName is required."), nil
		}
		key := args.PluginName + "/enabled"
		value := "false"
		if args.Action == "enable" {
			value = "true"
		}
		replacement := key + "=" + value
		existingPattern := regexp.MustCompile(regexp.QuoteMeta(args.PluginName) + `/enabled=\w+`)
		if loc := existingPattern.FindStringIndex(content); loc != nil {
			content = content[:loc[0]] + replacement + content[loc[1]:]
		} else {
			if !strings.Contains(content, "[editor_plugins]") {
				content += "\n[editor_plugins]\n"
			}
			content = strings.Replace(content, "[editor_plugins]", "[editor_plugins]\n"+replacement, 1)
		}
		if err := writeProjectGodot(args.ProjectPath, content); err != nil {
			return mcp.ErrorResult("manage_plugins failed: " + err.Error()), nil //nolint:nilerr // the failure is reported through the tool result.
		}
		return mcp.TextResult(fmt.Sprintf("Plugin %s %sd.", args.PluginName, args.Action)), nil
	}
	return mcp.ErrorResult("Unknown action: " + args.Action), nil
}
