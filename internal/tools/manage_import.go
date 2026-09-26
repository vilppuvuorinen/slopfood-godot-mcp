package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/vilppuvuorinen/slopfood-godot-mcp/internal/godot"
	"github.com/vilppuvuorinen/slopfood-godot-mcp/internal/godotconfig"
	"github.com/vilppuvuorinen/slopfood-godot-mcp/internal/mcp"
)

func manageImport() mcp.Tool {
	return mcp.Tool{
		Name:        "manage_import",
		Title:       "Manage Import Metadata",
		Description: "Set or remove a resource's import parameter and reimport it",
		Annotations: &mcp.ToolAnnotations{DestructiveHint: mcp.Bool(true), IdempotentHint: mcp.Bool(true), OpenWorldHint: mcp.Bool(false)},
		InputSchema: actionInputSchema(
			map[string]any{
				"resourcePath": map[string]any{
					"type":        "string",
					"description": `Imported resource path, relative to the project (for example "icon.svg"); the ".import" path is also accepted`,
				},
				"action": map[string]any{
					"type":        "string",
					"enum":        []string{"setParam", "removeParam"},
					"description": `"setParam" writes a [params] value; "removeParam" deletes an override so the importer default applies. Both reimport the source.`,
				},
				"name": map[string]any{
					"type":        "string",
					"description": `Import parameter name such as "compress/mode" (setParam, removeParam)`,
				},
				"value": godotValueSchema(`Parameter value (setParam), for example 0 or {"$type":"Vector2","x":1,"y":2}`),
			},
			[]string{"resourcePath", "action"},
			[]actionVariant{
				{Action: "setParam", Required: []string{"name", "value"}},
				{Action: "removeParam", Required: []string{"name"}},
			},
		),
		OutputSchema: statusOutputSchema(map[string]any{
			"resourcePath": map[string]any{"type": "string", "description": "Imported resource path, relative to the bound project"},
			"importPath":   map[string]any{"type": "string", "description": "Metadata path, relative to the bound project"},
			"action":       map[string]any{"type": "string"},
			"name":         map[string]any{"type": "string"},
			"value":        godotValueSchema("The stored parameter value in the tool value contract (setParam only)"),
		}),
		Handler: handleManageImport,
	}
}

type manageImportArgs struct {
	ResourcePath string          `json:"resourcePath"`
	Action       string          `json:"action"`
	Name         string          `json:"name"`
	Value        json.RawMessage `json:"value"`
}

func handleManageImport(ctx context.Context, raw json.RawMessage) (any, error) {
	var args manageImportArgs
	if len(raw) > 0 && string(raw) != "null" {
		if err := json.Unmarshal(raw, &args); err != nil {
			return nil, fmt.Errorf("invalid arguments: %w", err)
		}
	}
	project := projectRoot(ctx)
	if args.ResourcePath == "" || args.Action == "" {
		return requiredArgsResult("resourcePath", "action"), nil
	}
	if result, ok := validateImportAction(args); !ok {
		return result, nil
	}
	if !validPath(project) {
		return invalidPathResult(), nil
	}
	if !pathExists(filepath.Join(project, "project.godot")) {
		return notAProjectResult(project), nil
	}

	sourcePath, importPath, ok := importPaths(args.ResourcePath)
	if !ok {
		return importPathError(), nil
	}
	full, inside := confinePath(project, importPath)
	if !inside {
		return invalidPathResult(), nil
	}
	data, err := os.ReadFile(full)
	if err != nil {
		if os.IsNotExist(err) {
			return importMissingResult(importPath), nil
		}
		return mcp.ErrorResult("Failed to manage import metadata: " + err.Error()), nil //nolint:nilerr // the failure is reported through the tool result.
	}
	config, err := godotconfig.Parse(string(data))
	if err != nil {
		return mcp.ErrorResult("Failed to manage import metadata: " + err.Error()), nil //nolint:nilerr // the failure is reported through the tool result.
	}

	fields := map[string]any{
		"resourcePath": sourcePath,
		"importPath":   importPath,
		"action":       args.Action,
		"name":         args.Name,
	}
	switch args.Action {
	case "setParam":
		value, err := godotconfig.DecodeJSONValue(args.Value)
		if err != nil {
			return mcp.ErrorResult("Invalid value: " + err.Error()), nil //nolint:nilerr // the failure is reported through the tool result.
		}
		contract, err := godotconfig.ValueToJSON(value)
		if err != nil {
			return mcp.ErrorResult("Invalid value: " + err.Error()), nil //nolint:nilerr // the failure is reported through the tool result.
		}
		config.Set("params", args.Name, value)
		fields["value"] = contract
	case "removeParam":
		config.Delete("params", args.Name)
	}

	if err := os.WriteFile(full, []byte(config.Encode()), 0o644); err != nil { //nolint:gosec // user-facing project file.
		return mcp.ErrorResult("Failed to manage import metadata: " + err.Error()), nil //nolint:nilerr // the failure is reported through the tool result.
	}

	// The parameter only takes effect once the engine reimports the source, so
	// drive the editor filesystem scan rather than leaving a stale artifact.
	_, stderr, exitCode, importErr := godot.Import(ctx, godot.Path(), project)
	if importErr != nil {
		return mcp.ErrorResult("Failed to manage import metadata: "+importErr.Error(), headlessSolutions...), nil //nolint:nilerr // the failure is reported through the tool result.
	}
	if scriptFailed(exitCode, stderr, importAllowedStderr...) {
		return mcp.ErrorResult("Failed to manage import metadata: "+stderr, headlessSolutions...), nil
	}

	message := fmt.Sprintf("Import parameter %q updated on %s.", args.Name, importPath)
	if args.Action == "removeParam" {
		message = fmt.Sprintf("Import parameter %q removed from %s; the importer default applies after the scan.", args.Name, importPath)
	}
	return statusResult(project, message, fields), nil
}

// validateImportAction enforces each action's own required fields. The input
// schema's oneOf advertises them; this is the server-side guarantee.
func validateImportAction(args manageImportArgs) (mcp.ToolResult, bool) {
	missing := func(field string) (mcp.ToolResult, bool) {
		return mcp.ErrorResult(field + " is required for the " + args.Action + " action."), false
	}
	switch args.Action {
	case "setParam":
		if args.Name == "" {
			return missing("name")
		}
		if len(args.Value) == 0 || string(args.Value) == "null" {
			return missing("value")
		}
	case "removeParam":
		if args.Name == "" {
			return missing("name")
		}
	default:
		return mcp.ErrorResult(`Unknown action: ` + args.Action + `. Use setParam or removeParam.`), false
	}
	return mcp.ToolResult{}, true
}
