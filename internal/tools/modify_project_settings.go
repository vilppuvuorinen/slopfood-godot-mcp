package tools

import (
	"bytes"
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
				"section": map[string]any{
					"type":        "string",
					"description": `Section in project.godot (e.g., "application", "display", "rendering")`,
				},
				"key": map[string]any{
					"type":        "string",
					"description": `Setting key (e.g., "run/main_scene", "window/size/viewport_width")`,
				},
				"value": godotValueSchema(`Value to set in the tool value contract (for example "text", 1280, true, or {"$type":"Vector2","x":1,"y":2})`),
			},
			"required": []string{"section", "key", "value"},
		},
		OutputSchema: statusOutputSchema(map[string]any{
			"section": map[string]any{"type": "string"},
			"key":     map[string]any{"type": "string"},
			"value":   map[string]any{"type": "string"},
		}),
		Handler: handleModifyProjectSettings,
	}
}

type modifyProjectSettingsArgs struct {
	Section string          `json:"section"`
	Key     string          `json:"key"`
	Value   json.RawMessage `json:"value"`
}

func handleModifyProjectSettings(ctx context.Context, raw json.RawMessage) (any, error) {
	var args modifyProjectSettingsArgs
	if len(raw) > 0 && string(raw) != "null" {
		if err := json.Unmarshal(raw, &args); err != nil {
			return nil, fmt.Errorf("invalid arguments: %w", err)
		}
	}
	project := projectRoot(ctx)
	if args.Section == "" || args.Key == "" || len(bytes.TrimSpace(args.Value)) == 0 {
		return mcp.ErrorResult("section, key, and value are required."), nil
	}

	store, failure := openSettings(project)
	if failure != nil {
		return *failure, nil
	}

	settingValue, err := godotconfig.DecodeJSONValue(args.Value)
	if err != nil {
		return mcp.ErrorResult("Invalid value: " + err.Error()), nil //nolint:nilerr // the failure is reported through the tool result.
	}
	value := godotconfig.FormatValue(settingValue)
	if _, err = encodeSettingSection(args.Section); err != nil {
		return mcp.ErrorResult("Invalid section: " + err.Error()), nil //nolint:nilerr // the failure is reported through the tool result.
	}

	store.Set(args.Section, args.Key, settingValue)
	if err := store.Save(); err != nil {
		return mcp.ErrorResult("Failed to modify project settings: " + err.Error()), nil //nolint:nilerr // the failure is reported through the tool result.
	}

	return statusResult(project, fmt.Sprintf("Setting updated: [%s] %s=%s", args.Section, args.Key, value), map[string]any{
		"section": args.Section,
		"key":     args.Key,
		"value":   value,
	}), nil
}

// encodeSettingSection validates a caller-supplied section name and returns the
// escaped text to place inside "[...]". Godot's ConfigFile format escapes only
// ']'; backslashes are literal, so a name ending in one produces a tag that
// never terminates and that Godot itself cannot reload. Rather than guess, the
// escaped header is parsed and must decode back to exactly the requested name,
// which also rejects line breaks (the server's line-based scanner cannot
// represent a multi-line header) and leading or trailing whitespace (the tag
// parser trims it, so it would not round-trip).
func encodeSettingSection(name string) (string, error) {
	if strings.ContainsAny(name, "\r\n") {
		return "", fmt.Errorf("not a valid section name: contains a line break")
	}
	escaped := godotconfig.EscapeSectionName(name)
	cfg, err := godotconfig.Parse("[" + escaped + "]\n\nprobe=1\n")
	if err != nil {
		return "", fmt.Errorf("not a valid section name: %w", err)
	}
	if _, ok := cfg.Get(name, "probe"); !ok {
		return "", fmt.Errorf("not a valid section name: %q does not round-trip", name)
	}
	return escaped, nil
}
