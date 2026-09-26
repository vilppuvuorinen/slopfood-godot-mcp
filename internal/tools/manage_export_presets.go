package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/vilppuvuorinen/go-dot-mcp/internal/godotconfig"
	"github.com/vilppuvuorinen/go-dot-mcp/internal/mcp"
)

func manageExportPresets() mcp.Tool {
	return mcp.Tool{
		Name:        "manage_export_presets",
		Title:       "Manage Export Presets",
		Description: "Create or remove export preset configuration",
		Annotations: &mcp.ToolAnnotations{DestructiveHint: mcp.Bool(true), OpenWorldHint: mcp.Bool(false)},
		InputSchema: actionInputSchema(
			map[string]any{
				"action": map[string]any{
					"type":        "string",
					"enum":        []string{"add", "remove"},
					"description": `"add" or "remove"`,
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
			[]string{"action"},
			[]actionVariant{
				{Action: "add", Required: []string{"name", "platform"}},
				{Action: "remove", Required: []string{"name"}},
			},
		),
		OutputSchema: statusOutputSchema(map[string]any{
			"name":     map[string]any{"type": "string"},
			"platform": map[string]any{"type": "string"},
			"runnable": map[string]any{"type": "boolean"},
		}),
		Handler: handleManageExportPresets,
	}
}

type manageExportPresetsArgs struct {
	Action   string `json:"action"`
	Name     string `json:"name"`
	Platform string `json:"platform"`
	Runnable bool   `json:"runnable"`
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
	exportPresetNamePattern     = regexp.MustCompile(`(?m)^name="((?:[^"\\]|\\.)*)"`)
	exportPresetPlatformPattern = regexp.MustCompile(`(?m)^platform="((?:[^"\\]|\\.)*)"`)
	exportPresetBlockPattern    = regexp.MustCompile(`(?m)^\[preset\.`)
	exportPresetIndexPattern    = regexp.MustCompile(`(?m)^\[preset\.(\d+)\]`)
)

func handleManageExportPresets(ctx context.Context, raw json.RawMessage) (any, error) {
	var args manageExportPresetsArgs
	if len(raw) > 0 && string(raw) != "null" {
		if err := json.Unmarshal(raw, &args); err != nil {
			return nil, fmt.Errorf("invalid arguments: %w", err)
		}
	}
	project := projectRoot(ctx)
	if args.Action == "" {
		return requiredArgsResult("action"), nil
	}
	if !validPath(project) {
		return invalidPathResult(), nil
	}
	if !pathExists(filepath.Join(project, "project.godot")) {
		return notAProjectResult(project), nil
	}

	presetsFile := filepath.Join(project, "export_presets.cfg")
	switch args.Action {
	case "add":
		return addExportPreset(project, presetsFile, args), nil
	case "remove":
		return removeExportPreset(project, presetsFile, args.Name), nil
	}
	return mcp.ErrorResult(`Invalid action. Use "add" or "remove".`), nil
}

// listExportPresets reads the name and platform of every [preset.N] block,
// defaulting the platform to "unknown", and pretty-prints the list. A missing
// export_presets.cfg is an empty list.
func listExportPresets(presetsFile string) mcp.ToolResult {
	presets := []exportPreset{}
	data, err := os.ReadFile(presetsFile)
	if err != nil && !os.IsNotExist(err) {
		return exportPresetsFailure(err)
	}
	if err == nil {
		for _, block := range exportPresetBlocks(string(data)) {
			match := exportPresetNamePattern.FindStringSubmatch(block)
			if match == nil {
				continue
			}
			preset := exportPreset{Name: godotconfig.UnescapeString(match[1]), Platform: "unknown"}
			if platform := exportPresetPlatformPattern.FindStringSubmatch(block); platform != nil {
				preset.Platform = godotconfig.UnescapeString(platform[1])
			}
			presets = append(presets, preset)
		}
	}
	encoded, err := json.MarshalIndent(exportPresetList{Presets: presets}, "", "  ")
	if err != nil {
		return exportPresetsFailure(err)
	}
	return mcp.JSONResult(string(encoded))
}

// exportPresetBlocks splits content into its [preset.N] blocks, preserving
// order.
func exportPresetBlocks(content string) []string {
	blocks := exportPresetBlockPattern.FindAllStringIndex(content, -1)
	out := make([]string, 0, len(blocks))
	for i, block := range blocks {
		end := len(content)
		if i+1 < len(blocks) {
			end = blocks[i+1][0]
		}
		out = append(out, content[block[0]:end])
	}
	return out
}

// nextExportPresetIndex returns one past the highest existing `[preset.N]`
// index, or 0 when there are none. The editor stores presets sequentially, so
// appending at max+1 matches what it would write and survives gaps left by a
// previous remove.
func nextExportPresetIndex(content string) int {
	next := 0
	for _, match := range exportPresetIndexPattern.FindAllStringSubmatch(content, -1) {
		n, err := strconv.Atoi(match[1])
		if err != nil {
			continue
		}
		if n >= next {
			next = n + 1
		}
	}
	return next
}

func addExportPreset(project, presetsFile string, args manageExportPresetsArgs) mcp.ToolResult {
	if args.Name == "" || args.Platform == "" {
		return mcp.ErrorResult("name and platform are required for add action.")
	}
	content := ""
	data, err := os.ReadFile(presetsFile)
	if err != nil && !os.IsNotExist(err) {
		return exportPresetsFailure(err)
	}
	if err == nil {
		content = string(data)
	}
	block := fmt.Sprintf("\n[preset.%d]\n\nname=%s\nplatform=%s\nrunnable=%s\n",
		nextExportPresetIndex(content),
		godotconfig.FormatValue(godotconfig.String(args.Name)),
		godotconfig.FormatValue(godotconfig.String(args.Platform)),
		godotconfig.FormatValue(godotconfig.Bool(args.Runnable)))
	content += block
	if err := os.WriteFile(presetsFile, []byte(content), 0o644); err != nil { //nolint:gosec // user-facing project file.
		return exportPresetsFailure(err)
	}
	return statusResult(project, fmt.Sprintf("Export preset \"%s\" added for platform \"%s\".", args.Name, args.Platform), map[string]any{
		"name":     args.Name,
		"platform": args.Platform,
		"runnable": args.Runnable,
	})
}

func removeExportPreset(project, presetsFile, name string) mcp.ToolResult {
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
	return statusResult(project, fmt.Sprintf("Export preset \"%s\" removed.", name), map[string]any{
		"name": name,
	})
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
	encodedName := godotconfig.EscapeString(name)
	namePattern := regexp.MustCompile(`(?m)^name="` + regexp.QuoteMeta(encodedName) + `"`)
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
