package tools

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/vilppuvuorinen/go-dot-mcp/internal/godotconfig"
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

// configSchema identifies the project.godot ConfigFile format, keyed by the
// config_version stored in the file. This server implements the Godot 4.x
// schema only; other versions are rejected rather than written with
// conventions that engine would misinterpret (for example Godot 3's
// special-key scancodes, which use a different KEY_SPECIAL bit).
type configSchema int

const (
	// schemaGodot4 is the Godot 4.x schema, which writes config_version=5.
	schemaGodot4 configSchema = iota
)

// godot4ConfigVersion is the config_version Godot 4.x writes to project.godot.
const godot4ConfigVersion = 5

// projectSchema returns the schema of a project.godot document. A missing,
// malformed, or unsupported config_version is an error, so version-specific
// readers and writers never run against a schema they do not understand.
func projectSchema(content string) (configSchema, error) { //nolint:unparam // schemaGodot4 is the only schema today; the return value is the branch point for future config versions.
	var raw string
	found := false
	eachAssignment(content, func(section, rawKey string, _, valStart, valEnd int) bool {
		if section == "" && decodePropertyName(rawKey) == "config_version" {
			raw = strings.TrimSpace(content[valStart:valEnd])
			found = true
			return false
		}
		return true
	})
	if !found {
		return 0, fmt.Errorf("project.godot has no config_version")
	}
	value, err := godotconfig.ParseValue(raw)
	if err != nil {
		return 0, fmt.Errorf("parse config_version %q: %w", raw, err)
	}
	version, ok := value.(godotconfig.Int)
	if !ok {
		return 0, fmt.Errorf("config_version %q is not an integer", raw)
	}
	if int(version) != godot4ConfigVersion {
		return 0, fmt.Errorf("unsupported project.godot config_version %d; only Godot 4.x (config_version=%d) is supported", int(version), godot4ConfigVersion)
	}
	return schemaGodot4, nil
}

// readProjectGodot validates projectPath and returns the raw project.godot
// contents. An unsafe path and a directory without project.godot each produce
// the reference server's message; a project.godot this server does not
// implement is rejected outright. Callers validate required arguments before
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
	content := string(data)
	if _, err := projectSchema(content); err != nil {
		result := mcp.ErrorResult("Unsupported Godot project: " + err.Error())
		return "", &result
	}
	return content, nil
}

// writeProjectGodot persists content to projectPath/project.godot after
// confirming its schema is one this server implements, so a version-specific
// writer can never emit a file the engine would misread.
func writeProjectGodot(projectPath, content string) error {
	if _, err := projectSchema(content); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(projectPath, "project.godot"), []byte(content), 0o644); err != nil { //nolint:gosec // user-facing project file.
		return fmt.Errorf("write project.godot: %w", err)
	}
	return nil
}

// sectionHeaderPattern matches a project.godot section header such as
// "[application]".
var sectionHeaderPattern = regexp.MustCompile(`^\[(.+)\]$`)

// scanKey reads the raw key starting at i and returns its text plus the offset
// of the assignment '='. A key may be quoted (EncodePropertyName quotes keys
// with special characters) and therefore span multiple lines.
func scanKey(content string, i int) (key string, eq int, ok bool) {
	if i >= len(content) {
		return "", 0, false
	}
	if content[i] == '"' {
		j := i + 1
		for j < len(content) {
			if content[j] == '\\' {
				j += 2
				continue
			}
			if content[j] == '"' {
				j++
				break
			}
			j++
		}
		if j > len(content) {
			j = len(content)
		}
		k := j
		for k < len(content) && (content[k] == ' ' || content[k] == '\t') {
			k++
		}
		if k < len(content) && content[k] == '=' {
			return content[i:j], k, true
		}
		return "", 0, false
	}
	for j := i; j < len(content); j++ {
		if content[j] == '\n' {
			return "", 0, false
		}
		if content[j] == '=' {
			return content[i:j], j, true
		}
	}
	return "", 0, false
}

// scanValue returns the offset just past the value starting at i. Quoted
// strings (which may contain literal newlines and escaped quotes) and bracketed
// constructs such as dictionaries, arrays and Object(...) constructors are
// balanced; a newline only ends the value at bracket depth zero. The returned
// offset is at the terminating newline, or len(s).
func scanValue(s string, i int) int {
	depth := 0
	for i < len(s) {
		switch s[i] {
		case '"':
			i++
			for i < len(s) {
				if s[i] == '\\' {
					i += 2
					continue
				}
				if s[i] == '"' {
					i++
					break
				}
				i++
			}
			continue
		case '{', '[', '(':
			depth++
		case '}', ']', ')':
			if depth > 0 {
				depth--
			}
		case '\n':
			if depth == 0 {
				return i
			}
		}
		i++
	}
	if i > len(s) {
		return len(s)
	}
	return i
}

// decodePropertyName returns the decoded key text for a raw project.godot key,
// stripping the quotes EncodePropertyName adds for special characters.
func decodePropertyName(raw string) string {
	if len(raw) >= 2 && raw[0] == '"' && raw[len(raw)-1] == '"' {
		return godotconfig.UnescapeString(raw[1 : len(raw)-1])
	}
	return raw
}

// eachAssignment walks every key/value assignment in file order. fn receives
// the decoded section name, the raw (still encoded) key, and the byte range of
// the raw value. Returning false stops the walk.
func eachAssignment(content string, fn func(section, rawKey string, lineStart, valStart, valEnd int) bool) {
	current := ""
	for i := 0; i < len(content); {
		lineEnd := strings.IndexByte(content[i:], '\n')
		next := len(content)
		if lineEnd >= 0 {
			next = i + lineEnd + 1
		}
		line := strings.TrimSuffix(content[i:next], "\n")
		trimmed := strings.TrimSpace(line)
		if match := sectionHeaderPattern.FindStringSubmatch(trimmed); match != nil {
			current = godotconfig.UnescapeSectionName(match[1])
			i = next
			continue
		}
		if trimmed == "" || strings.HasPrefix(trimmed, ";") {
			i = next
			continue
		}
		keyStart := i + len(line) - len(strings.TrimLeft(line, " \t"))
		if rawKey, eq, found := scanKey(content, keyStart); found {
			valStart := eq + 1
			valEnd := scanValue(content, valStart)
			if !fn(current, strings.TrimSpace(rawKey), i, valStart, valEnd) {
				return
			}
			i = valEnd
			continue
		}
		i = next
	}
}

// findSectionAssignment locates the assignment of key inside section. It
// returns the byte offsets of the line start, the value start (just after '=')
// and the value end (at the terminating newline or EOF). Keys and section names
// are compared in their encoded form, so quoted keys match too. The value may
// span multiple lines.
func findSectionAssignment(content, section, key string) (lineStart, valStart, valEnd int, ok bool) {
	target := godotconfig.EncodePropertyName(key)
	eachAssignment(content, func(s, rawKey string, ls, vs, ve int) bool {
		if s == section && rawKey == target {
			lineStart, valStart, valEnd, ok = ls, vs, ve, true
			return false
		}
		return true
	})
	return lineStart, valStart, valEnd, ok
}

// removeSectionKey removes a whole key from a single project.godot section,
// matching only exact key names inside that section. Unlike a global regexp it
// cannot delete a longer key that merely contains key as a substring (e.g.
// "Player" vs "MyPlayer") or a same-named key in another section. Multi-line
// values are removed whole.
func removeSectionKey(content, section, key string) string {
	lineStart, _, valEnd, ok := findSectionAssignment(content, section, key)
	if !ok {
		return content
	}
	end := len(content)
	if nl := strings.IndexByte(content[valEnd:], '\n'); nl >= 0 {
		end = valEnd + nl + 1
	}
	return content[:lineStart] + content[end:]
}

// setSectionValue replaces the value of section/key in place and reports
// whether the key existed.
func setSectionValue(content, section, key string, value godotconfig.Value) (string, bool) {
	_, valStart, valEnd, ok := findSectionAssignment(content, section, key)
	if !ok {
		return content, false
	}
	return content[:valStart] + godotconfig.FormatValue(value) + content[valEnd:], true
}

// rawEntry is one key/value pair as it appears in the file, before decoding.
type rawEntry struct {
	Key   string
	Value string
}

// sectionEntries returns the raw key/value pairs of one section in file order.
func sectionEntries(content, section string) []rawEntry {
	var entries []rawEntry
	eachAssignment(content, func(s, rawKey string, _, valStart, valEnd int) bool {
		if s == section {
			entries = append(entries, rawEntry{
				Key:   rawKey,
				Value: strings.TrimSpace(content[valStart:valEnd]),
			})
		}
		return true
	})
	return entries
}

// parseProjectSections parses project.godot into section -> key -> raw value,
// mirroring the reference server's line-based parser used by
// read_project_settings and the manage_* list actions. Blank lines and
// semicolon comments are skipped; keys seen before any section are ignored and
// repeated keys overwrite earlier ones. Multi-line values are returned whole
// and quoted keys are decoded.
func parseProjectSections(content string) map[string]map[string]string {
	sections := make(map[string]map[string]string)
	eachAssignment(content, func(section, rawKey string, _, valStart, valEnd int) bool {
		if section == "" {
			return true
		}
		if _, ok := sections[section]; !ok {
			sections[section] = make(map[string]string)
		}
		sections[section][decodePropertyName(rawKey)] = strings.TrimSpace(content[valStart:valEnd])
		return true
	})
	return sections
}
