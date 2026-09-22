package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/vilppuvuorinen/go-dot-mcp/internal/mcp"
)

func manageExportPresets() mcp.Tool {
	return mcp.Tool{
		Name:        "manage_export_presets",
		Description: "Create or modify export preset configuration",
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
					"description": "Preset name (required for add/remove)",
				},
				"platform": map[string]any{
					"type":        "string",
					"description": `Platform (for add, e.g. "Windows Desktop", "Linux", "Web")`,
				},
				"runnable": map[string]any{
					"type":        "boolean",
					"description": "Whether this preset is runnable. Default: false",
				},
			},
			"required": []string{"projectPath", "action"},
		},
		Handler: handleManageExportPresets,
	}
}

type manageExportPresetsArgs struct {
	ProjectPath string `json:"projectPath"`
	Action      string `json:"action"`
	Name        string `json:"name"`
	Platform    string `json:"platform"`
	Runnable    bool   `json:"runnable"`
}

// exportPreset is one entry in the list action's JSON document.
type exportPreset struct {
	Name     string `json:"name"`
	Platform string `json:"platform"`
}

// exportPresetList is the JSON document returned by the list action.
type exportPresetList struct {
	Presets []exportPreset `json:"presets"`
}

var (
	exportPresetNamePattern     = regexp.MustCompile(`name="([^"]+)"`)
	exportPresetPlatformPattern = regexp.MustCompile(`platform="([^"]+)"`)
	exportPresetBlockPattern    = regexp.MustCompile(`(?m)^\[preset\.`)
)

func handleManageExportPresets(_ context.Context, raw json.RawMessage) (any, error) {
	raw, err := aliasParams(raw, parameterMappings)
	if err != nil {
		return nil, err
	}
	var args manageExportPresetsArgs
	if len(raw) > 0 && string(raw) != "null" {
		if err = json.Unmarshal(raw, &args); err != nil {
			return nil, fmt.Errorf("invalid arguments: %w", err)
		}
	}
	if args.ProjectPath == "" || args.Action == "" {
		return mcp.ErrorResult("projectPath and action are required."), nil
	}
	if !validPath(args.ProjectPath) {
		return mcp.ErrorResult("Invalid path."), nil
	}
	if !pathExists(filepath.Join(args.ProjectPath, "project.godot")) {
		return mcp.ErrorResult("Not a valid Godot project: " + args.ProjectPath), nil
	}

	presetsFile := filepath.Join(args.ProjectPath, "export_presets.cfg")
	switch args.Action {
	case "list":
		return listExportPresets(presetsFile), nil
	case "add":
		return addExportPreset(presetsFile, args), nil
	case "remove":
		return removeExportPreset(presetsFile, args.Name), nil
	}
	return mcp.ErrorResult(`Invalid action. Use "list", "add", or "remove".`), nil
}

// listExportPresets pairs every name="..." with the platform="..." at the same
// index, defaulting the platform to "unknown", and pretty-prints the list. A
// missing export_presets.cfg is an empty list.
func listExportPresets(presetsFile string) mcp.ToolResult {
	presets := []exportPreset{}
	data, err := os.ReadFile(presetsFile)
	if err != nil && !os.IsNotExist(err) {
		return exportPresetsFailure(err)
	}
	if err == nil {
		content := string(data)
		names := submatchValues(exportPresetNamePattern, content)
		platforms := submatchValues(exportPresetPlatformPattern, content)
		for i, name := range names {
			platform := "unknown"
			if i < len(platforms) {
				platform = platforms[i]
			}
			presets = append(presets, exportPreset{Name: name, Platform: platform})
		}
	}
	encoded, err := json.MarshalIndent(exportPresetList{Presets: presets}, "", "  ")
	if err != nil {
		return exportPresetsFailure(err)
	}
	return mcp.TextResult(string(encoded))
}

func addExportPreset(presetsFile string, args manageExportPresetsArgs) mcp.ToolResult {
	if args.Name == "" || args.Platform == "" {
		return mcp.ErrorResult("name and platform are required for add action.")
	}
	runnable := "false"
	if args.Runnable {
		runnable = "true"
	}
	block := fmt.Sprintf("\n[preset.%d]\n\nname=\"%s\"\nplatform=\"%s\"\nrunnable=%s\n",
		time.Now().UnixMilli(), args.Name, args.Platform, runnable)

	content := ""
	data, err := os.ReadFile(presetsFile)
	if err != nil && !os.IsNotExist(err) {
		return exportPresetsFailure(err)
	}
	if err == nil {
		content = string(data)
	}
	content += block
	if err := os.WriteFile(presetsFile, []byte(content), 0o644); err != nil { //nolint:gosec // user-facing project file.
		return exportPresetsFailure(err)
	}
	return mcp.TextResult(fmt.Sprintf("Export preset \"%s\" added for platform \"%s\".", args.Name, args.Platform))
}

func removeExportPreset(presetsFile, name string) mcp.ToolResult {
	if name == "" {
		return mcp.ErrorResult("name is required for remove action.")
	}
	data, err := os.ReadFile(presetsFile)
	if err != nil {
		if os.IsNotExist(err) {
			return mcp.ErrorResult("No export_presets.cfg file found.")
		}
		return exportPresetsFailure(err)
	}
	content := deleteExportPresetBlock(string(data), name)
	if err := os.WriteFile(presetsFile, []byte(content), 0o644); err != nil { //nolint:gosec // user-facing project file.
		return exportPresetsFailure(err)
	}
	return mcp.TextResult(fmt.Sprintf("Export preset \"%s\" removed.", name))
}

// exportPresetsFailure is the reference server's catch-all message for an
// unreadable or unwritable export_presets.cfg.
func exportPresetsFailure(err error) mcp.ToolResult {
	return mcp.ErrorResult("Failed to manage export presets: " + err.Error())
}

// deleteExportPresetBlock drops the `[preset.N]` blocks whose name="<name>"
// entry matches, leaving every other block byte-for-byte intact.
func deleteExportPresetBlock(content, name string) string {
	blocks := exportPresetBlockPattern.FindAllStringIndex(content, -1)
	if len(blocks) == 0 {
		return content
	}
	namePattern := regexp.MustCompile(`name="` + regexp.QuoteMeta(name) + `"`)
	var out strings.Builder
	out.WriteString(content[:blocks[0][0]])
	for i, block := range blocks {
		end := len(content)
		if i+1 < len(blocks) {
			end = blocks[i+1][0]
		}
		if namePattern.MatchString(content[block[0]:end]) {
			continue
		}
		out.WriteString(content[block[0]:end])
	}
	return out.String()
}

// submatchValues returns the first capture group of every match.
func submatchValues(pattern *regexp.Regexp, content string) []string {
	matches := pattern.FindAllStringSubmatch(content, -1)
	values := make([]string, 0, len(matches))
	for _, match := range matches {
		values = append(values, match[1])
	}
	return values
}
