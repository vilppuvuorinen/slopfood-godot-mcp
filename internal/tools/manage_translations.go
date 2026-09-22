package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/vilppuvuorinen/go-dot-mcp/internal/mcp"
)

func manageTranslations() mcp.Tool {
	return mcp.Tool{
		Name:        "manage_translations",
		Description: "List/add/remove translation files in project",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"projectPath": map[string]any{
					"type":        "string",
					"description": "Godot project path",
				},
				"action": map[string]any{
					"type":        "string",
					"description": "Action: list, add, remove",
				},
				"translationPath": map[string]any{
					"type":        "string",
					"description": "Translation file path",
				},
			},
			"required": []string{"projectPath", "action"},
		},
		Handler: handleManageTranslations,
	}
}

type manageTranslationsArgs struct {
	ProjectPath     string `json:"projectPath"`
	Action          string `json:"action"`
	TranslationPath string `json:"translationPath"`
}

// translationsArrayPattern matches a project.godot translations entry.
var translationsArrayPattern = regexp.MustCompile(`translations=PackedStringArray\(([^)]*)\)`)

func handleManageTranslations(_ context.Context, raw json.RawMessage) (any, error) {
	raw, err := aliasParams(raw, settingsAliases)
	if err != nil {
		return nil, err
	}
	var args manageTranslationsArgs
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
		translations := make([]string, 0)
		if match := translationsArrayPattern.FindStringSubmatch(content); match != nil {
			for _, part := range strings.Split(match[1], ",") {
				value := strings.ReplaceAll(strings.TrimSpace(part), `"`, "")
				if value != "" {
					translations = append(translations, value)
				}
			}
		}
		payload := struct {
			Translations []string `json:"translations"`
		}{Translations: translations}
		data, err := json.MarshalIndent(payload, "", "  ")
		if err != nil {
			return mcp.ErrorResult("manage_translations failed: " + err.Error()), nil //nolint:nilerr // the failure is reported through the tool result.
		}
		return mcp.TextResult(string(data)), nil

	case "add":
		if args.TranslationPath == "" {
			return mcp.ErrorResult("translationPath is required."), nil
		}
		resPath := args.TranslationPath
		if !strings.HasPrefix(resPath, "res://") {
			resPath = "res://" + resPath
		}
		if loc := translationsArrayPattern.FindStringSubmatchIndex(content); loc != nil {
			existing := content[loc[2]:loc[3]]
			var newVal string
			if existing != "" {
				newVal = existing + `, "` + resPath + `"`
			} else {
				newVal = `"` + resPath + `"`
			}
			replacement := "translations=PackedStringArray(" + newVal + ")"
			content = content[:loc[0]] + replacement + content[loc[1]:]
		} else {
			if !strings.Contains(content, "[internationalization]") {
				content += "\n[internationalization]\n"
			}
			header := "[internationalization]"
			idx := strings.Index(content, header)
			insert := header + "\n\ntranslations=PackedStringArray(\"" + resPath + "\")"
			content = content[:idx] + insert + content[idx+len(header):]
		}
		if err := writeProjectGodot(args.ProjectPath, content); err != nil {
			return mcp.ErrorResult("manage_translations failed: " + err.Error()), nil //nolint:nilerr // the failure is reported through the tool result.
		}
		return mcp.TextResult("Translation added: " + resPath), nil

	case "remove":
		if args.TranslationPath == "" {
			return mcp.ErrorResult("translationPath is required."), nil
		}
		resPath := args.TranslationPath
		if !strings.HasPrefix(resPath, "res://") {
			resPath = "res://" + resPath
		}
		entryPattern := regexp.MustCompile(`,?\s*"` + regexp.QuoteMeta(resPath) + `"`)
		if loc := entryPattern.FindStringIndex(content); loc != nil {
			content = content[:loc[0]] + content[loc[1]:]
		}
		if err := writeProjectGodot(args.ProjectPath, content); err != nil {
			return mcp.ErrorResult("manage_translations failed: " + err.Error()), nil //nolint:nilerr // the failure is reported through the tool result.
		}
		return mcp.TextResult("Translation removed: " + resPath), nil
	}

	return mcp.ErrorResult("Unknown action: " + args.Action), nil
}
