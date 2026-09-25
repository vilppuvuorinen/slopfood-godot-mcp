package tools

import (
	"fmt"

	"github.com/vilppuvuorinen/go-dot-mcp/internal/godotconfig"
	"github.com/vilppuvuorinen/go-dot-mcp/internal/mcp"
)

// settingsStore is the single access point for project.godot. It holds the
// parsed ConfigFile and the project path, so every settings tool reads and
// writes typed Values and persists through writeProjectGodot. Callers never
// splice text into the file themselves: a new key is added to the Config and
// re-encoded whole, which removes the substring matching that could mistake a
// section header inside a value for the real header.
type settingsStore struct {
	projectPath string
	schema      configSchema
	config      *godotconfig.Config
}

// openSettings validates projectPath, reads project.godot, and parses it into a
// settingsStore. A path that is unsafe, is not a project, uses an unsupported
// config_version, or contains text the ConfigFile parser rejects is reported as
// a failure result, matching readProjectGodot's contract.
func openSettings(projectPath string) (*settingsStore, *mcp.ToolResult) {
	content, failure := readProjectGodot(projectPath)
	if failure != nil {
		return nil, failure
	}
	schema, err := projectSchema(content)
	if err != nil {
		result := unsupportedProjectResult(err)
		return nil, &result
	}
	config, err := godotconfig.Parse(content)
	if err != nil {
		result := unsupportedProjectResult(err)
		return nil, &result
	}
	return &settingsStore{projectPath: projectPath, schema: schema, config: config}, nil
}

// Get returns the Value stored at section/key.
func (s *settingsStore) Get(section, key string) (godotconfig.Value, bool) {
	return s.config.Get(section, key)
}

// Set stores a value, creating the section when it is missing and replacing an
// existing key in place. Call save to persist.
func (s *settingsStore) Set(section, key string, value godotconfig.Value) {
	s.config.Set(section, key, value)
}

// Delete removes one key, reporting whether it existed.
func (s *settingsStore) Delete(section, key string) bool {
	return s.config.Delete(section, key)
}

// DeleteSection removes a whole section, reporting whether it existed.
func (s *settingsStore) DeleteSection(section string) bool {
	return s.config.DeleteSection(section)
}

// SectionValues returns a section's keys with each value encoded in the tool
// value contract, ready to be marshaled as a list action's JSON payload. An
// absent section yields an empty (non-nil) map.
func (s *settingsStore) SectionValues(section string) (map[string]any, error) {
	return encodeSectionValues(s.config.SectionKeys(section))
}

// FilteredSectionValues returns the parsed sections encoded in the tool value
// contract, restricted to one section and/or a set of key names. Unlike
// SectionValues it is driven by the parsed document, so the section-less prefix
// is keyed by "". An empty section keeps every section; empty keys keeps every
// key. Only sections with at least one selected key are emitted, so a filter
// that matches nothing yields an empty (non-nil) map with a zero count rather
// than empty sections. The specialized readers use SectionValues so both share
// one encoding.
func (s *settingsStore) FilteredSectionValues(section string, keys []string) (map[string]map[string]any, int, error) {
	keySet := make(map[string]bool, len(keys))
	for _, key := range keys {
		keySet[key] = true
	}
	out := make(map[string]map[string]any, len(s.config.Sections))
	count := 0
	for _, sec := range s.config.Sections {
		if section != "" && sec.Name != section {
			continue
		}
		selected := make([]godotconfig.KeyValue, 0, len(sec.Keys))
		for _, kv := range sec.Keys {
			if len(keySet) > 0 && !keySet[kv.Key] {
				continue
			}
			selected = append(selected, kv)
		}
		if len(selected) == 0 {
			continue
		}
		values, err := encodeSectionValues(selected)
		if err != nil {
			return nil, 0, err
		}
		out[sec.Name] = values
		count += len(selected)
	}
	return out, count, nil
}

// encodeSectionValues encodes a section's key/value pairs in the tool value
// contract.
func encodeSectionValues(keys []godotconfig.KeyValue) (map[string]any, error) {
	out := make(map[string]any, len(keys))
	for _, kv := range keys {
		value, err := godotconfig.ValueToJSON(kv.Value)
		if err != nil {
			return nil, fmt.Errorf("encode setting %q: %w", kv.Key, err)
		}
		out[kv.Key] = value
	}
	return out, nil
}

// Save re-encodes the whole document and writes it through writeProjectGodot,
// so the schema check and the strict parse gate always apply.
func (s *settingsStore) Save() error {
	return writeProjectGodot(s.projectPath, s.config.Encode())
}
