package tools

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/vilppuvuorinen/go-dot-mcp/internal/godotconfig"
	"github.com/vilppuvuorinen/go-dot-mcp/internal/mcp"
)

func readPlugins() mcp.Tool {
	return mcp.Tool{
		Name:        "read_plugins",
		Title:       "Read Editor Plugins",
		Description: "List installed editor plugins and whether each is enabled",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: mcp.Bool(true), OpenWorldHint: mcp.Bool(false)},
		InputSchema: map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		},
		OutputSchema: readOutputSchema(payloadOutputSchema(map[string]any{
			"pluginCount": map[string]any{"type": "integer", "description": "Number of installed or orphaned plugins reported"},
			"plugins": map[string]any{
				"type":        "array",
				"description": "Installed addons in path order, followed by enabled entries whose plugin.cfg is missing",
				"items": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"path":        map[string]any{"type": "string", "description": "plugin.cfg path, relative to the bound project"},
						"name":        map[string]any{"type": "string", "description": "Plugin display name from plugin.cfg, or an empty string"},
						"description": map[string]any{"type": "string", "description": "Plugin description from plugin.cfg, or an empty string"},
						"author":      map[string]any{"type": "string", "description": "Plugin author from plugin.cfg, or an empty string"},
						"version":     map[string]any{"type": "string", "description": "Plugin version from plugin.cfg, or an empty string"},
						"script":      map[string]any{"type": "string", "description": "Plugin script path from plugin.cfg, or an empty string"},
						"enabled":     map[string]any{"type": "boolean", "description": "Whether the plugin is listed in [editor_plugins] enabled"},
						"missing":     map[string]any{"type": "boolean", "description": "True for an enabled entry whose plugin.cfg no longer exists"},
					},
					"required": []string{"path", "name", "description", "author", "version", "script", "enabled", "missing"},
				},
			},
		})),
		Handler: handleReadPlugins,
	}
}

// pluginInfo is one entry in read_plugins' payload.
type pluginInfo struct {
	Path        string `json:"path"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Author      string `json:"author"`
	Version     string `json:"version"`
	Script      string `json:"script"`
	Enabled     bool   `json:"enabled"`
	Missing     bool   `json:"missing"`
}

// pluginList is read_plugins' payload.
type pluginList struct {
	PluginCount int          `json:"pluginCount"`
	Plugins     []pluginInfo `json:"plugins"`
}

func handleReadPlugins(ctx context.Context, _ json.RawMessage) (any, error) {
	project := projectRoot(ctx)
	if !validPath(project) {
		return invalidPathResult(), nil
	}
	if !pathExists(filepath.Join(project, "project.godot")) {
		return notAProjectResult(project), nil
	}
	store, failure := openSettings(project)
	if failure != nil {
		return *failure, nil
	}
	enabled, err := readEnabledPlugins(store)
	if err != nil {
		return mcp.ErrorResult("Failed to read plugins: " + err.Error()), nil //nolint:nilerr // the failure is reported through the tool result.
	}

	enabledSet := make(map[string]bool, len(enabled))
	for _, resourcePath := range enabled {
		enabledSet[resourcePath] = true
	}

	plugins := discoverPlugins(project)
	for i := range plugins {
		plugins[i].Enabled = enabledSet[resScheme+plugins[i].Path]
	}

	// An enabled entry with no plugin.cfg on disk is still part of the project
	// state; report it rather than dropping it silently.
	orphans := make([]pluginInfo, 0)
	for _, resourcePath := range enabled {
		rel := projectRelativePath(resourcePath)
		if rel == resourcePath || rel == "" {
			// Not a res:// path: still report it with the raw spelling.
			rel = resourcePath
		}
		if !hasPluginPath(plugins, rel) {
			orphans = append(orphans, pluginInfo{Path: rel, Enabled: true, Missing: true})
		}
	}
	sort.Slice(orphans, func(i, j int) bool { return orphans[i].Path < orphans[j].Path })
	plugins = append(plugins, orphans...)

	payload := pluginList{PluginCount: len(plugins), Plugins: plugins}
	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return mcp.ErrorResult("Failed to read plugins: " + err.Error()), nil //nolint:nilerr // the failure is reported through the tool result.
	}
	return mcp.JSONResult(string(data)), nil
}

// hasPluginPath reports whether plugins already contains a plugin at rel.
func hasPluginPath(plugins []pluginInfo, rel string) bool {
	for _, plugin := range plugins {
		if plugin.Path == rel {
			return true
		}
	}
	return false
}

// discoverPlugins walks the project's addons directory and returns one entry
// per plugin.cfg, sorted by path. A plugin.cfg the ConfigFile parser rejects is
// skipped: it is not a usable plugin. A missing addons directory is an empty
// list.
func discoverPlugins(project string) []pluginInfo {
	plugins := make([]pluginInfo, 0)
	addons := filepath.Join(project, "addons")
	var walk func(dir string)
	walk = func(dir string) {
		entries, err := os.ReadDir(dir)
		if err != nil {
			return
		}
		for _, entry := range entries {
			name := entry.Name()
			if entry.IsDir() {
				if strings.HasPrefix(name, ".") {
					continue
				}
				walk(filepath.Join(dir, name))
				continue
			}
			if !entry.Type().IsRegular() || name != pluginConfigName {
				continue
			}
			full := filepath.Join(dir, name)
			rel, err := filepath.Rel(project, full)
			if err != nil {
				continue
			}
			info := pluginInfo{Path: filepath.ToSlash(rel)}
			if data, err := os.ReadFile(full); err == nil {
				if config, err := godotconfig.Parse(string(data)); err == nil {
					info.Name = configString(config, "plugin", "name")
					info.Description = configString(config, "plugin", "description")
					info.Author = configString(config, "plugin", "author")
					info.Version = configString(config, "plugin", "version")
					info.Script = configString(config, "plugin", "script")
				}
			}
			plugins = append(plugins, info)
		}
	}
	walk(addons)
	sort.Slice(plugins, func(i, j int) bool { return plugins[i].Path < plugins[j].Path })
	return plugins
}
