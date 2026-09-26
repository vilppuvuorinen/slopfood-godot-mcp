package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"path"
	"path/filepath"
	"strings"

	"github.com/vilppuvuorinen/go-dot-mcp/internal/godotconfig"
	"github.com/vilppuvuorinen/go-dot-mcp/internal/mcp"
)

// editorPluginsSection and editorPluginsKey are where Godot stores the enabled
// editor plugin list in project.godot.
const (
	editorPluginsSection = "editor_plugins"
	editorPluginsKey     = "enabled"
)

// pluginConfigName is the file that marks a directory as an editor plugin.
const pluginConfigName = "plugin.cfg"

func managePlugins() mcp.Tool {
	return mcp.Tool{
		Name:        "manage_plugins",
		Title:       "Manage Editor Plugins",
		Description: "Enable or disable an editor plugin by its addon plugin.cfg path",
		Annotations: &mcp.ToolAnnotations{DestructiveHint: mcp.Bool(true), IdempotentHint: mcp.Bool(true), OpenWorldHint: mcp.Bool(false)},
		InputSchema: actionInputSchema(
			map[string]any{
				"action": map[string]any{
					"type":        "string",
					"enum":        []string{"enable", "disable"},
					"description": `"enable" or "disable"`,
				},
				"plugin": map[string]any{
					"type":        "string",
					"description": `Addon plugin.cfg path, relative to the project or as "res://..." (for example "addons/demo/plugin.cfg"); the addon directory also works`,
				},
			},
			[]string{"action"},
			[]actionVariant{
				{Action: "enable", Required: []string{"plugin"}},
				{Action: "disable", Required: []string{"plugin"}},
			},
		),
		OutputSchema: statusOutputSchema(map[string]any{
			"action":  map[string]any{"type": "string"},
			"plugin":  map[string]any{"type": "string", "description": "Addon plugin.cfg path, relative to the bound project"},
			"enabled": map[string]any{"type": "boolean", "description": "Whether the plugin is enabled after the call"},
		}),
		Handler: handleManagePlugins,
	}
}

type managePluginsArgs struct {
	Action string `json:"action"`
	Plugin string `json:"plugin"`
}

func handleManagePlugins(ctx context.Context, raw json.RawMessage) (any, error) {
	var args managePluginsArgs
	if len(raw) > 0 && string(raw) != "null" {
		if err := json.Unmarshal(raw, &args); err != nil {
			return nil, fmt.Errorf("invalid arguments: %w", err)
		}
	}
	project := projectRoot(ctx)
	if args.Action == "" || args.Plugin == "" {
		return requiredArgsResult("action", "plugin"), nil
	}
	if !validPath(project) {
		return invalidPathResult(), nil
	}
	if !pathExists(filepath.Join(project, "project.godot")) {
		return notAProjectResult(project), nil
	}

	pluginPath, ok := normalizePluginPath(args.Plugin)
	if !ok {
		return pluginPathError(), nil
	}
	full, inside := confinePath(project, pluginPath)
	if !inside {
		return invalidPathResult(), nil
	}

	store, failure := openSettings(project)
	if failure != nil {
		return *failure, nil
	}
	enabled, err := readEnabledPlugins(store)
	if err != nil {
		return mcp.ErrorResult("Failed to manage plugins: " + err.Error()), nil //nolint:nilerr // the failure is reported through the tool result.
	}

	resourcePath := resScheme + pluginPath
	switch args.Action {
	case "enable":
		if !pathExists(full) {
			return mcp.ErrorResult(
				"Plugin not found: "+pluginPath,
				"Ensure the addon is installed under res://addons",
				"Use read_plugins to list the installed addons",
			), nil
		}
		if !containsString(enabled, resourcePath) {
			enabled = append(enabled, resourcePath)
		}
	case "disable":
		enabled = removeString(enabled, resourcePath)
	default:
		return mcp.ErrorResult(`Invalid action. Use "enable" or "disable".`), nil
	}

	setEnabledPlugins(store, enabled)
	if err := store.Save(); err != nil {
		return mcp.ErrorResult("Failed to manage plugins: " + err.Error()), nil //nolint:nilerr // the failure is reported through the tool result.
	}

	verb := "enabled"
	if args.Action == "disable" {
		verb = "disabled"
	}
	return statusResult(project, fmt.Sprintf("Editor plugin %q %s.", pluginPath, verb), map[string]any{
		"action":  args.Action,
		"plugin":  pluginPath,
		"enabled": args.Action == "enable",
	}), nil
}

// pluginPathError is the shared error for a plugin argument that does not name
// an addon plugin.cfg.
func pluginPathError() mcp.ToolResult {
	return mcp.ErrorResult(
		`Invalid plugin path. Provide a project-relative path such as "addons/demo/plugin.cfg".`,
		"Use read_plugins to list the installed addons",
	)
}

// normalizePluginPath rewrites a caller-supplied plugin reference as a
// project-relative plugin.cfg path with forward slashes. It accepts a res://
// path, a path to the addon directory, or the plugin.cfg path itself. ok is
// false for an empty, escaping, or plugin.cfg-less path.
func normalizePluginPath(raw string) (string, bool) {
	cleaned := strings.TrimSpace(raw)
	cleaned = strings.TrimPrefix(cleaned, resScheme)
	cleaned = strings.Trim(cleaned, "/")
	if cleaned == "" || strings.Contains(cleaned, "..") {
		return "", false
	}
	cleaned = path.Clean(cleaned)
	if cleaned == "." || strings.HasPrefix(cleaned, "../") {
		return "", false
	}
	if path.Base(cleaned) != pluginConfigName {
		cleaned = path.Join(cleaned, pluginConfigName)
	}
	return cleaned, true
}

// readEnabledPlugins returns the plugin.cfg res:// paths stored in
// [editor_plugins] enabled. A missing key is an empty list; any other stored
// type is an error rather than silently discarded.
func readEnabledPlugins(store *settingsStore) ([]string, error) {
	value, ok := store.Get(editorPluginsSection, editorPluginsKey)
	if !ok {
		return nil, nil
	}
	plugins, ok := value.(godotconfig.PackedStringArray)
	if !ok {
		return nil, fmt.Errorf("[%s] %s is not a PackedStringArray", editorPluginsSection, editorPluginsKey)
	}
	return append([]string(nil), plugins...), nil
}

// setEnabledPlugins stores the enabled list, removing the key and then the
// section when nothing is enabled so an empty project keeps no editor_plugins
// block.
func setEnabledPlugins(store *settingsStore, enabled []string) {
	if len(enabled) == 0 {
		store.Delete(editorPluginsSection, editorPluginsKey)
		if len(store.config.SectionKeys(editorPluginsSection)) == 0 {
			store.DeleteSection(editorPluginsSection)
		}
		return
	}
	store.Set(editorPluginsSection, editorPluginsKey, godotconfig.PackedStringArray(enabled))
}

// configString returns a ConfigFile string value, or "" when the key is absent
// or is not a string.
func configString(config *godotconfig.Config, section, key string) string {
	value, ok := config.Get(section, key)
	if !ok {
		return ""
	}
	text, _ := value.(godotconfig.String)
	return string(text)
}

// containsString reports whether list contains target.
func containsString(list []string, target string) bool {
	for _, item := range list {
		if item == target {
			return true
		}
	}
	return false
}

// removeString returns list without every occurrence of target, preserving
// order and the input slice when target is absent.
func removeString(list []string, target string) []string {
	out := make([]string, 0, len(list))
	for _, item := range list {
		if item != target {
			out = append(out, item)
		}
	}
	return out
}
