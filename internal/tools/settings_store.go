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
		result := mcp.ErrorResult("Unsupported Godot project: " + err.Error())
		return nil, &result
	}
	config, err := godotconfig.Parse(content)
	if err != nil {
		result := mcp.ErrorResult("Unsupported Godot project: " + err.Error())
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

// Keys returns a copy of a section's keys in file order.
func (s *settingsStore) Keys(section string) []godotconfig.KeyValue {
	return s.config.SectionKeys(section)
}

// SectionValues returns a section's keys with each value encoded in the tool
// value contract, ready to be marshaled as a list action's JSON payload. An
// absent section yields an empty (non-nil) map.
func (s *settingsStore) SectionValues(section string) (map[string]any, error) {
	out := make(map[string]any)
	for _, kv := range s.config.SectionKeys(section) {
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
