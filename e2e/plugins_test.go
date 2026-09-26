package e2e

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// TestManagePluginsSchema pins the action tool's input surface.
func TestManagePluginsSchema(t *testing.T) {
	schema := requireSchema(t, "manage_plugins", "Enable or disable an editor plugin by its addon plugin.cfg path")
	schema.requireRequired(t, "action")
	schema.requireStringProps(t, "action", "plugin")
	schema.requirePropCount(t, 2)
}

// TestReadPluginsSchema pins the read-only plugin tool's input surface.
func TestReadPluginsSchema(t *testing.T) {
	schema := requireSchema(t, "read_plugins", "List installed editor plugins and whether each is enabled")
	schema.requireRequired(t)
	schema.requirePropCount(t, 0)
}

// testPluginList is the subset of read_plugins' payload the tests assert against.
type testPluginList struct {
	PluginCount int `json:"pluginCount"`
	Plugins     []struct {
		Path    string `json:"path"`
		Name    string `json:"name"`
		Enabled bool   `json:"enabled"`
		Missing bool   `json:"missing"`
	} `json:"plugins"`
}

// readPluginsPayloadE2E calls read_plugins and decodes its payload.
func readPluginsPayloadE2E(t *testing.T, session *mcp.ClientSession) testPluginList {
	t.Helper()
	text := requireToolText(t, session, "read_plugins", map[string]any{})
	var payload testPluginList
	if err := json.Unmarshal([]byte(text), &payload); err != nil {
		t.Fatalf("unmarshal plugins payload: %v\n%s", err, text)
	}
	return payload
}

// TestManagePluginsTool drives the real plugin tools.
func TestManagePluginsTool(t *testing.T) {
	root := newProject(t)
	writeE2EFile(t, filepath.Join(root, "addons", "demo", "plugin.cfg"), "[plugin]\n\nname=\"Demo\"\ndescription=\"A demo\"\n")
	session := connectProject(t, root)
	requireGodot(t, session)

	payload := readPluginsPayloadE2E(t, session)
	if payload.PluginCount != 1 || len(payload.Plugins) != 1 {
		t.Fatalf("plugins = %+v, want one", payload)
	}
	if payload.Plugins[0].Enabled || payload.Plugins[0].Missing {
		t.Errorf("plugin flags = %+v, want disabled and present", payload.Plugins[0])
	}

	requireToolText(t, session, "manage_plugins", map[string]any{"action": "enable", "plugin": "addons/demo/plugin.cfg"})
	payload = readPluginsPayloadE2E(t, session)
	if !payload.Plugins[0].Enabled {
		t.Errorf("plugin = %+v, want enabled", payload.Plugins[0])
	}

	// The engine must still accept project.godot after the rewrite.
	content := readFile(t, filepath.Join(root, "project.godot"))
	requireContains(t, content, "[editor_plugins]", "res://addons/demo/plugin.cfg")
	requireValidProject(t, session)

	requireToolText(t, session, "manage_plugins", map[string]any{"action": "disable", "plugin": "addons/demo"})
	payload = readPluginsPayloadE2E(t, session)
	if payload.Plugins[0].Enabled {
		t.Errorf("plugin = %+v, want disabled", payload.Plugins[0])
	}
	content = readFile(t, filepath.Join(root, "project.godot"))
	requireNotContains(t, content, "editor_plugins")
	requireValidProject(t, session)
}

// TestManagePluginsErrors covers the validation paths.
func TestManagePluginsErrors(t *testing.T) {
	session := connect(t)
	requireToolError(t, session, "manage_plugins", map[string]any{}, "action and plugin are required.")

	root := newProject(t)
	projectSession := connectProject(t, root)

	res := callTool(t, projectSession, "manage_plugins", map[string]any{"action": "enable", "plugin": "addons/missing/plugin.cfg"})
	if !res.IsError {
		t.Fatal("expected an error result")
	}
	requireContains(t, firstText(t, res), "Plugin not found: addons/missing/plugin.cfg")
}
