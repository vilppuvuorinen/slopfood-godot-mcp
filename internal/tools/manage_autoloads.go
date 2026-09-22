package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/vilppuvuorinen/go-dot-mcp/internal/mcp"
)

func manageAutoloads() mcp.Tool {
	return mcp.Tool{
		Name:        "manage_autoloads",
		Description: "Add, remove, or list autoloads in a Godot project",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"projectPath": map[string]any{
					"type":        "string",
					"description": "Godot project path",
				},
				"action": map[string]any{
					"type":        "string",
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

	content, failure := readProjectGodot(args.ProjectPath)
	if failure != nil {
		return *failure, nil
	}

	switch args.Action {
	case "list":
		autoloads := parseProjectSections(content)["autoload"]
		if autoloads == nil {
			autoloads = map[string]string{}
		}
		data, err := json.MarshalIndent(autoloads, "", "  ")
		if err != nil {
			return mcp.ErrorResult("Failed to manage autoloads: " + err.Error()), nil //nolint:nilerr // the failure is reported through the tool result.
		}
		return mcp.TextResult(string(data)), nil
	case "add":
		return addAutoload(args, content)
	case "remove":
		if args.Name == "" {
			return mcp.ErrorResult("name is required for remove action."), nil
		}
		pattern := regexp.MustCompile(`\n?` + regexp.QuoteMeta(args.Name) + `\s*=.*\n?`)
		content = pattern.ReplaceAllString(content, "\n")
		if err := writeProjectGodot(args.ProjectPath, content); err != nil {
			return mcp.ErrorResult("Failed to manage autoloads: " + err.Error()), nil //nolint:nilerr // the failure is reported through the tool result.
		}
		return mcp.TextResult(fmt.Sprintf("Autoload %q removed.", args.Name)), nil
	}
	return mcp.ErrorResult(`Invalid action. Use "list", "add", or "remove".`), nil
}

// addAutoload inserts an autoload entry after the first [autoload] header
// (creating the section when absent) and persists the result.
func addAutoload(args manageAutoloadsArgs, content string) (any, error) {
	if args.Name == "" || args.Path == "" {
		return mcp.ErrorResult("name and path are required for add action."), nil
	}
	autoloadLine := args.Name + `="*` + args.Path + `"`
	if idx := strings.Index(content, "[autoload]"); idx >= 0 {
		content = content[:idx] + "[autoload]\n\n" + autoloadLine + content[idx+len("[autoload]"):]
	} else {
		content += "\n[autoload]\n\n" + autoloadLine + "\n"
	}
	if err := writeProjectGodot(args.ProjectPath, content); err != nil {
		return mcp.ErrorResult("Failed to manage autoloads: " + err.Error()), nil //nolint:nilerr // the failure is reported through the tool result.
	}
	return mcp.TextResult(fmt.Sprintf("Autoload %q added: %s", args.Name, args.Path)), nil
}
