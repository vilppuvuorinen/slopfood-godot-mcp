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
					"description": `Value to set, as a single Godot literal (for example "\"text\"", "1280" or "true"); it is parsed and re-serialized`,
				},
			},
			"required": []string{"projectPath", "section", "key", "value"},
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

	value, err := encodeSettingValue(*args.Value)
	if err != nil {
		return mcp.ErrorResult("Invalid value: " + err.Error()), nil //nolint:nilerr // the failure is reported through the tool result.
	}
	section, err := encodeSettingSection(args.Section)
	if err != nil {
		return mcp.ErrorResult("Invalid section: " + err.Error()), nil //nolint:nilerr // the failure is reported through the tool result.
	}

	sectionHeader := "[" + section + "]"
	keyLine := godotconfig.EncodePropertyName(args.Key) + "=" + value
	if _, valStart, valEnd, ok := findSectionAssignment(content, args.Section, args.Key); ok {
		content = content[:valStart] + value + content[valEnd:]
	} else if sectionIdx := strings.Index(content, sectionHeader); sectionIdx != -1 {
		insertPos := sectionIdx + len(sectionHeader)
		content = content[:insertPos] + "\n" + keyLine + content[insertPos:]
	} else {
		content += "\n\n" + sectionHeader + "\n\n" + keyLine + "\n"
	}

	if err := writeProjectGodot(args.ProjectPath, content); err != nil {
		return mcp.ErrorResult("Failed to modify project settings: " + err.Error()), nil //nolint:nilerr // the failure is reported through the tool result.
	}

	return statusResult(fmt.Sprintf("Setting updated: [%s] %s=%s", args.Section, args.Key, value), map[string]any{
		"section": args.Section,
		"key":     args.Key,
		"value":   value,
	}), nil
}

// encodeSettingValue validates a caller-supplied project.godot value as a
// single Godot Variant and re-serializes it with the same writer Godot uses.
// The raw literal is never copied through verbatim: a value such as
// "1280\n[input]\nfoo=1" is rejected instead of injecting a new key or
// section, and a quoted string is re-escaped so embedded quotes and newlines
// stay inside the string. An empty value is not a Variant (Godot rejects
// "key=" as "Expected value, got 'EOF'"); an empty string is written as `""`.
func encodeSettingValue(raw string) (string, error) {
	value, err := godotconfig.ParseValue(raw)
	if err != nil {
		return "", fmt.Errorf("not a valid Godot literal: %w", err)
	}
	return godotconfig.FormatValue(value), nil
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
