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
		Title:       "Manage Translations",
		Description: "List/add/remove translation files in project",
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
					"description": "Action: list, add, remove",
				},
				"translationPath": map[string]any{
					"type":        "string",
					"description": "Translation file path",
				},
			},
			"required": []string{"projectPath", "action"},
		},
		OutputSchema: actionOutputSchema(
			map[string]any{
				"translationPath": map[string]any{"type": "string"},
			},
			payloadOutputSchema(map[string]any{
				"translations": godotValueSchema(`The translations setting as a tagged {"$type":"PackedStringArray","values":[...]} value`),
			}),
		),
		Handler: handleManageTranslations,
	}
}

type manageTranslationsArgs struct {
	ProjectPath     string `json:"projectPath"`
	Action          string `json:"action"`
	TranslationPath string `json:"translationPath"`
}

// readTranslations returns the existing translations entry as a typed
// PackedStringArray, or nil when it is absent or another type.
func readTranslations(store *settingsStore) godotconfig.PackedStringArray {
	value, ok := store.Get("internationalization", "translations")
	if !ok {
		return nil
	}
	translations, _ := value.(godotconfig.PackedStringArray)
	return translations
}

func handleManageTranslations(_ context.Context, raw json.RawMessage) (any, error) {
	var args manageTranslationsArgs
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
		translations := append(godotconfig.PackedStringArray{}, readTranslations(store)...)
		value, err := godotconfig.ValueToJSON(translations)
		if err != nil {
			return mcp.ErrorResult("manage_translations failed: " + err.Error()), nil //nolint:nilerr // the failure is reported through the tool result.
		}
		data, err := json.MarshalIndent(map[string]any{"translations": value}, "", "  ")
		if err != nil {
			return mcp.ErrorResult("manage_translations failed: " + err.Error()), nil //nolint:nilerr // the failure is reported through the tool result.
		}
		return mcp.JSONResult(string(data)), nil

	case "add":
		if args.TranslationPath == "" {
			return mcp.ErrorResult("translationPath is required."), nil
		}
		resPath := args.TranslationPath
		if !strings.HasPrefix(resPath, "res://") {
			resPath = "res://" + resPath
		}
		translations := append(append(godotconfig.PackedStringArray{}, readTranslations(store)...), resPath)
		store.Set("internationalization", "translations", translations)
		if err := store.Save(); err != nil {
			return mcp.ErrorResult("manage_translations failed: " + err.Error()), nil //nolint:nilerr // the failure is reported through the tool result.
		}
		return statusResult("Translation added: "+resPath, map[string]any{
			"translationPath": resPath,
		}), nil

	case "remove":
		if args.TranslationPath == "" {
			return mcp.ErrorResult("translationPath is required."), nil
		}
		resPath := args.TranslationPath
		if !strings.HasPrefix(resPath, "res://") {
			resPath = "res://" + resPath
		}
		var kept godotconfig.PackedStringArray
		for _, translation := range readTranslations(store) {
			if translation != resPath {
				kept = append(kept, translation)
			}
		}
		store.Set("internationalization", "translations", kept)
		if err := store.Save(); err != nil {
			return mcp.ErrorResult("manage_translations failed: " + err.Error()), nil //nolint:nilerr // the failure is reported through the tool result.
		}
		return statusResult("Translation removed: "+resPath, map[string]any{
			"translationPath": resPath,
		}), nil
	}

	return mcp.ErrorResult("Unknown action: " + args.Action), nil
}
