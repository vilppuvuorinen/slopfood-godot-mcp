package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/vilppuvuorinen/go-dot-mcp/internal/godotconfig"
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

// readTranslations returns the existing translations entry as a typed
// PackedStringArray, or nil when it is absent or unparseable.
func readTranslations(content string) godotconfig.PackedStringArray {
	raw, ok := parseProjectSections(content)["internationalization"]["translations"]
	if !ok {
		return nil
	}
	value, err := godotconfig.ParseValue(raw)
	if err != nil {
		return nil
	}
	translations, _ := value.(godotconfig.PackedStringArray)
	return translations
}

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
		translations = append(translations, readTranslations(content)...)
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
		translations := append(readTranslations(content), resPath)
		if updated, ok := setSectionValue(content, "internationalization", "translations", translations); ok {
			content = updated
		} else {
			if !strings.Contains(content, "[internationalization]") {
				content += "\n[internationalization]\n"
			}
			header := "[internationalization]"
			idx := strings.Index(content, header)
			insert := header + "\n\ntranslations=" + godotconfig.FormatValue(translations)
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
		var kept godotconfig.PackedStringArray
		for _, translation := range readTranslations(content) {
			if translation != resPath {
				kept = append(kept, translation)
			}
		}
		if updated, ok := setSectionValue(content, "internationalization", "translations", kept); ok {
			content = updated
		}
		if err := writeProjectGodot(args.ProjectPath, content); err != nil {
			return mcp.ErrorResult("manage_translations failed: " + err.Error()), nil //nolint:nilerr // the failure is reported through the tool result.
		}
		return mcp.TextResult("Translation removed: " + resPath), nil
	}

	return mcp.ErrorResult("Unknown action: " + args.Action), nil
}
