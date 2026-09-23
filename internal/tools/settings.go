package tools

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/vilppuvuorinen/go-dot-mcp/internal/mcp"
)

// settingsAliases lets callers of the project-settings tools use the
// snake_case spellings too, mirroring the reference server's
// normalizeParameters over its PARAMETER_MAPPINGS table.
var settingsAliases = map[string]string{
	"project_path":     "projectPath",
	"scene_path":       "scenePath",
	"action_name":      "actionName",
	"layer_type":       "layerType",
	"plugin_name":      "pluginName",
	"translation_path": "translationPath",
}

// readProjectGodot validates projectPath and returns the raw project.godot
// contents. An unsafe path and a directory without project.godot each produce
// the reference server's message. Callers validate required arguments before
// calling it, so an empty projectPath is reported as "Invalid path.".
func readProjectGodot(projectPath string) (string, *mcp.ToolResult) {
	if !validPath(projectPath) {
		result := mcp.ErrorResult("Invalid path.")
		return "", &result
	}
	data, err := os.ReadFile(filepath.Join(projectPath, "project.godot"))
	if err != nil {
		result := mcp.ErrorResult("Not a valid Godot project: " + projectPath)
		return "", &result
	}
	return string(data), nil
}

// writeProjectGodot persists content to projectPath/project.godot.
func writeProjectGodot(projectPath, content string) error {
	if err := os.WriteFile(filepath.Join(projectPath, "project.godot"), []byte(content), 0o644); err != nil { //nolint:gosec // user-facing project file.
		return fmt.Errorf("write project.godot: %w", err)
	}
	return nil
}

// sectionHeaderPattern matches a project.godot section header such as
// "[application]".
var sectionHeaderPattern = regexp.MustCompile(`^\[(.+)\]$`)

// keyValuePattern matches a project.godot "key=value" line.
var keyValuePattern = regexp.MustCompile(`^([^=]+)=(.*)$`)

// removeSectionKey removes a whole key from a single project.godot section,
// matching only exact key names inside that section. Unlike a global regexp it
// cannot delete a longer key that merely contains key as a substring (e.g.
// "Player" vs "MyPlayer") or a same-named key in another section.
func removeSectionKey(content, section, key string) string {
	lines := strings.Split(content, "\n")
	current := ""
	kept := make([]string, 0, len(lines))
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if match := sectionHeaderPattern.FindStringSubmatch(trimmed); match != nil {
			current = match[1]
			kept = append(kept, line)
			continue
		}
		if current == section {
			if match := keyValuePattern.FindStringSubmatch(trimmed); match != nil && strings.TrimSpace(match[1]) == key {
				continue
			}
		}
		kept = append(kept, line)
	}
	return strings.Join(kept, "\n")
}

// parseProjectSections parses project.godot into section -> key -> raw value,
// mirroring the reference server's line-based parser used by
// read_project_settings and the manage_* list actions. Blank lines and
// semicolon comments are skipped; keys seen before any section are ignored and
// repeated keys overwrite earlier ones.
func parseProjectSections(content string) map[string]map[string]string {
	sections := make(map[string]map[string]string)
	current := ""
	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, ";") {
			continue
		}
		if match := sectionHeaderPattern.FindStringSubmatch(trimmed); match != nil {
			current = match[1]
			if _, ok := sections[current]; !ok {
				sections[current] = make(map[string]string)
			}
			continue
		}
		match := keyValuePattern.FindStringSubmatch(trimmed)
		if match == nil || current == "" {
			continue
		}
		sections[current][strings.TrimSpace(match[1])] = strings.TrimSpace(match[2])
	}
	return sections
}
