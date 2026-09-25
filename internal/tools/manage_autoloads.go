package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/vilppuvuorinen/go-dot-mcp/internal/godotconfig"
	"github.com/vilppuvuorinen/go-dot-mcp/internal/mcp"
)

func manageAutoloads() mcp.Tool {
	return mcp.Tool{
		Name:        "manage_autoloads",
		Title:       "Manage Autoloads",
		Description: "Add, remove, or list autoloads in a Godot project",
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
					"enum":        []string{"list", "add", "remove"},
					"description": `"list", "add", or "remove"`,
				},
				"name": map[string]any{
					"type":        "string",
					"description": "Autoload name (required for add/remove)",
				},
				"path": map[string]any{
					"type":        "string",
					"description": `Script/scene path (required for add, e.g. "res://globals.gd")`,
				},
			},
			"required": []string{"projectPath", "action"},
		},
		OutputSchema: actionOutputSchema(
			map[string]any{
				"name": map[string]any{"type": "string"},
				"path": map[string]any{"type": "string"},
			},
			map[string]any{
				"type":                 "object",
				"description":          "Autoload entries by name, with values in the tool value contract",
				"additionalProperties": godotValueSchema("A value in the tool value contract"),
			},
		),
		Handler: handleManageAutoloads,
	}
}

type manageAutoloadsArgs struct {
	ProjectPath string `json:"projectPath"`
	Action      string `json:"action"`
	Name        string `json:"name"`
	Path        string `json:"path"`
}

func handleManageAutoloads(_ context.Context, raw json.RawMessage) (any, error) {
	raw, err := aliasParams(raw, settingsAliases)
	if err != nil {
		return nil, err
	}
	var args manageAutoloadsArgs
	if len(raw) > 0 && string(raw) != "null" {
		if err = json.Unmarshal(raw, &args); err != nil {
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
		autoloads, err := store.SectionValues("autoload")
		if err != nil {
			return mcp.ErrorResult("Failed to manage autoloads: " + err.Error()), nil //nolint:nilerr // the failure is reported through the tool result.
		}
		data, err := json.MarshalIndent(autoloads, "", "  ")
		if err != nil {
			return mcp.ErrorResult("Failed to manage autoloads: " + err.Error()), nil //nolint:nilerr // the failure is reported through the tool result.
		}
		return mcp.JSONResult(string(data)), nil
	case "add":
		return addAutoload(store, args)
	case "remove":
		if args.Name == "" {
			return mcp.ErrorResult("name is required for remove action."), nil
		}
		store.Delete("autoload", args.Name)
		if err := store.Save(); err != nil {
			return mcp.ErrorResult("Failed to manage autoloads: " + err.Error()), nil //nolint:nilerr // the failure is reported through the tool result.
		}
		return statusResult(fmt.Sprintf("Autoload %q removed.", args.Name), map[string]any{
			"name": args.Name,
		}), nil
	}
	return mcp.ErrorResult(`Invalid action. Use "list", "add", or "remove".`), nil
}

// addAutoload stores an autoload entry (creating the section when absent) and
// persists the result.
func addAutoload(store *settingsStore, args manageAutoloadsArgs) (any, error) {
	if args.Name == "" || args.Path == "" {
		return mcp.ErrorResult("name and path are required for add action."), nil
	}
	resPath := args.Path
	if !strings.HasPrefix(resPath, "res://") {
		resPath = "res://" + resPath
	}
	store.Set("autoload", args.Name, godotconfig.String("*"+resPath))
	if err := store.Save(); err != nil {
		return mcp.ErrorResult("Failed to manage autoloads: " + err.Error()), nil //nolint:nilerr // the failure is reported through the tool result.
	}
	return statusResult(fmt.Sprintf("Autoload %q added: %s", args.Name, resPath), map[string]any{
		"name": args.Name,
		"path": resPath,
	}), nil
}
