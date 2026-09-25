package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/vilppuvuorinen/go-dot-mcp/internal/godotconfig"
	"github.com/vilppuvuorinen/go-dot-mcp/internal/mcp"
)

func managePlugins() mcp.Tool {
	return mcp.Tool{
		Name:        "manage_plugins",
		Title:       "Manage Plugins",
		Description: "List/enable/disable editor plugins",
		Annotations: &mcp.ToolAnnotations{DestructiveHint: mcp.Bool(true), OpenWorldHint: mcp.Bool(false)},
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"projectPath": map[string]any{
					"type":        "string",
					"description": "Godot project path",
				},
				"action": map[string]any{
					"type":        "string",
					"enum":        []string{"list", "enable", "disable"},
					"description": "Action: list, enable, disable",
				},
				"pluginName": map[string]any{
					"type":        "string",
					"description": "Plugin name",
				},
			},
			"required": []string{"projectPath", "action"},
		},
		OutputSchema: actionOutputSchema(
			map[string]any{
				"pluginName":    map[string]any{"type": "string"},
				"pluginEnabled": map[string]any{"type": "boolean"},
			},
			payloadOutputSchema(map[string]any{
				"enabled":   map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
				"available": map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
			}),
		),
		Handler: handleManagePlugins,
	}
}

type managePluginsArgs struct {
	ProjectPath string `json:"projectPath"`
	Action      string `json:"action"`
	PluginName  string `json:"pluginName"`
}

// pluginList is the JSON document returned by the list action.
type pluginList struct {
	Enabled   []string `json:"enabled"`
	Available []string `json:"available"`
}

func handleManagePlugins(_ context.Context, raw json.RawMessage) (any, error) {
	var args managePluginsArgs
	if len(raw) > 0 && string(raw) != "null" {
		if err := json.Unmarshal(raw, &args); err != nil {
			return nil, fmt.Errorf("invalid arguments: %w", err)
		}
	}
	if args.ProjectPath == "" || args.Action == "" {
		return mcp.ErrorResult("projectPath and action are required."), nil
	}

	store, failure := openSettings(args.ProjectPath)
	if failure != nil {
		return *failure, nil
	}

	switch args.Action {
	case "list":
		enabled := make([]string, 0)
		for _, kv := range store.Keys("editor_plugins") {
			if !strings.HasSuffix(kv.Key, "/enabled") {
				continue
			}
			if on, ok := kv.Value.(godotconfig.Bool); !ok || !bool(on) {
				continue
			}
			enabled = append(enabled, strings.TrimSuffix(kv.Key, "/enabled"))
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
		return mcp.JSONResult(string(encoded)), nil
	case "enable", "disable":
		if args.PluginName == "" {
			return mcp.ErrorResult("pluginName is required."), nil
		}
		key := args.PluginName + "/enabled"
		store.Set("editor_plugins", key, godotconfig.Bool(args.Action == "enable"))
		if err := store.Save(); err != nil {
			return mcp.ErrorResult("manage_plugins failed: " + err.Error()), nil //nolint:nilerr // the failure is reported through the tool result.
		}
		return statusResult(fmt.Sprintf("Plugin %s %sd.", args.PluginName, args.Action), map[string]any{
			"pluginName":    args.PluginName,
			"pluginEnabled": args.Action == "enable",
		}), nil
	}
	return mcp.ErrorResult("Unknown action: " + args.Action), nil
}
