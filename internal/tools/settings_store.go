package tools

import (
	"github.com/vilppuvuorinen/slopfood-godot-mcp/internal/godotconfig"
	"github.com/vilppuvuorinen/slopfood-godot-mcp/internal/mcp"
)

// settingsStore is the single access point for project.godot. It holds the
// parsed ConfigFile and the project path, so a settings-backed tool reads and
// writes typed Values and persists through writeProjectGodot. Callers never
// splice text into the file themselves: a changed key is re-encoded with the
// Config and written whole, which removes the substring matching that could
// mistake a section header inside a value for the real header.
type settingsStore struct {
	projectPath string
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
	if _, err := projectSchema(content); err != nil {
		result := unsupportedProjectResult(err)
		return nil, &result
	}
	config, err := godotconfig.Parse(content)
	if err != nil {
		result := unsupportedProjectResult(err)
		return nil, &result
	}
	return &settingsStore{projectPath: projectPath, config: config}, nil
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

// Save re-encodes the whole document and writes it through writeProjectGodot,
// so the schema check and the strict parse gate always apply.
func (s *settingsStore) Save() error {
	return writeProjectGodot(s.projectPath, s.config.Encode())
}
