package tools

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const demoPluginConfig = `[plugin]

name="Demo"
description="A demo addon"
author="Test"
version="1.2.3"
script="plugin.gd"
`

// readPluginsPayload calls read_plugins and decodes its payload.
func readPluginsPayload(t *testing.T, root string) pluginList {
	t.Helper()
	result := callToolArgs(t, readPlugins(), root, `{}`)
	if result.IsError {
		t.Fatalf("read_plugins error: %+v", result.Content)
	}
	data, err := json.Marshal(result.StructuredContent)
	if err != nil {
		t.Fatalf("marshal structured content: %v", err)
	}
	var payload pluginList
	if err := json.Unmarshal(data, &payload); err != nil {
		t.Fatalf("unmarshal payload %s: %v", data, err)
	}
	return payload
}

func TestReadPluginsListsAddons(t *testing.T) {
	root := t.TempDir()
	makeProject(t, root)
	seedFile(t, filepath.Join(root, "addons", "demo", "plugin.cfg"), demoPluginConfig)
	seedFile(t, filepath.Join(root, "addons", "other", "plugin.cfg"), "[plugin]\n\nname=\"Other\"\n")
	seedFile(t, filepath.Join(root, "project.godot"), `config_version=5

[editor_plugins]

enabled=PackedStringArray("res://addons/demo/plugin.cfg")
`)

	payload := readPluginsPayload(t, root)
	if payload.PluginCount != 2 || len(payload.Plugins) != 2 {
		t.Fatalf("payload = %+v, want two plugins", payload)
	}
	demo := payload.Plugins[0]
	if demo.Path != "addons/demo/plugin.cfg" {
		t.Errorf("path = %q", demo.Path)
	}
	if demo.Name != "Demo" || demo.Description != "A demo addon" || demo.Author != "Test" || demo.Version != "1.2.3" || demo.Script != "plugin.gd" {
		t.Errorf("demo metadata = %+v", demo)
	}
	if !demo.Enabled || demo.Missing {
		t.Errorf("demo flags = %+v, want enabled and present", demo)
	}
	other := payload.Plugins[1]
	if other.Enabled || other.Missing {
		t.Errorf("other flags = %+v, want disabled and present", other)
	}
}

func TestReadPluginsReportsOrphan(t *testing.T) {
	root := t.TempDir()
	makeProject(t, root)
	seedFile(t, filepath.Join(root, "project.godot"), `config_version=5

[editor_plugins]

enabled=PackedStringArray("res://addons/gone/plugin.cfg")
`)

	payload := readPluginsPayload(t, root)
	if payload.PluginCount != 1 || len(payload.Plugins) != 1 {
		t.Fatalf("payload = %+v, want one orphan", payload)
	}
	orphan := payload.Plugins[0]
	if orphan.Path != "addons/gone/plugin.cfg" || !orphan.Enabled || !orphan.Missing {
		t.Errorf("orphan = %+v", orphan)
	}
}

func TestManagePluginsEnableDisable(t *testing.T) {
	root := t.TempDir()
	makeProject(t, root)
	seedFile(t, filepath.Join(root, "addons", "demo", "plugin.cfg"), demoPluginConfig)
	projectGodot := filepath.Join(root, "project.godot")

	for _, args := range []struct {
		raw     string
		wantErr bool
	}{
		{raw: `{"action":"enable","plugin":"addons/demo/plugin.cfg"}`},
		{raw: `{"action":"enable","plugin":"addons/demo/plugin.cfg"}`},
	} {
		result := callToolArgs(t, managePlugins(), root, args.raw)
		if result.IsError != args.wantErr {
			t.Fatalf("result = %+v, wantErr = %v", result.Content, args.wantErr)
		}
		if strings.Count(readFileString(t, projectGodot), "res://addons/demo/plugin.cfg") != 1 {
			t.Fatalf("plugin not stored exactly once:\n%s", readFileString(t, projectGodot))
		}
	}
	if !strings.Contains(readFileString(t, projectGodot), `enabled=PackedStringArray("res://addons/demo/plugin.cfg")`) {
		t.Errorf("enabled list missing:\n%s", readFileString(t, projectGodot))
	}

	// The addon directory also names the plugin.
	result := callToolArgs(t, managePlugins(), root, `{"action":"disable","plugin":"res://addons/demo"}`)
	if result.IsError {
		t.Fatalf("disable error: %+v", result.Content)
	}
	content := readFileString(t, projectGodot)
	if strings.Contains(content, "editor_plugins") || strings.Contains(content, "res://addons/demo/plugin.cfg") {
		t.Errorf("editor_plugins not removed:\n%s", content)
	}
}

func TestManagePluginsErrors(t *testing.T) {
	root := t.TempDir()
	makeProject(t, root)
	seedFile(t, filepath.Join(root, "addons", "demo", "plugin.cfg"), demoPluginConfig)

	tests := []struct {
		name string
		args string
		want string
	}{
		{name: "missing plugin", args: `{"action":"enable"}`, want: "action and plugin are required."},
		{name: "unknown action", args: `{"action":"bogus","plugin":"addons/demo/plugin.cfg"}`, want: `Invalid action. Use "enable" or "disable".`},
		{name: "bad path", args: `{"action":"enable","plugin":"../escape"}`, want: "Invalid plugin path."},
		{name: "missing addon", args: `{"action":"enable","plugin":"addons/missing/plugin.cfg"}`, want: "Plugin not found: addons/missing/plugin.cfg"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := callToolArgs(t, managePlugins(), root, tt.args)
			if !result.IsError {
				t.Fatalf("expected an error result")
			}
			if got := result.Content[0].Text; !strings.Contains(got, tt.want) {
				t.Errorf("message = %q, want containing %q", got, tt.want)
			}
		})
	}
}

// readFileString reads path, failing the test when it cannot be read.
func readFileString(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(data)
}
