package e2e

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// TestExportProjectSchema ports the reference's tool-definition expectations
// for export_project over the real stdio surface.
func TestExportProjectSchema(t *testing.T) {
	t.Parallel()
	schema := requireSchema(t, "export_project", "Export a Godot project using a preset")
	schema.requireRequired(t, "presetName", "outputPath")
	schema.requirePropCount(t, 3)
	schema.requireStringProps(t, "presetName", "outputPath")
	schema.requireType(t, "debug", "boolean")
}

// TestExportProjectValidation covers the reference's validation messages.
func TestExportProjectValidation(t *testing.T) {
	t.Parallel()
	t.Run("missing params", func(t *testing.T) {
		root := newProject(t)
		session := connectProject(t, root)
		requireToolError(t, session, "export_project", map[string]any{},
			"presetName and outputPath are required.")
	})

	t.Run("invalid output path", func(t *testing.T) {
		root := newProject(t)
		session := connectProject(t, root)
		requireToolError(t, session, "export_project", map[string]any{
			"presetName": "Linux",
			"outputPath": "../escape",
		}, "Invalid path.")
	})

	t.Run("not a project", func(t *testing.T) {
		root := t.TempDir()
		session := connectProject(t, root)
		requireToolError(t, session, "export_project", map[string]any{
			"presetName": "Linux",
			"outputPath": "build/game.x86_64",
		}, "Not a valid Godot project: "+root)
	})
}

// exportPresetLinux is a complete Linux export preset. Godot requires the
// filter and options keys to be present; an abbreviated preset is rejected.
const exportPresetLinux = `[preset.0]

name="Linux"
platform="Linux"
runnable=true
dedicated_server=false
custom_features=""
export_filter="all_resources"
include_filter=""
exclude_filter=""
export_path=""
patches=PackedStringArray()
encryption_include_filters=""
encryption_exclude_filters=""
seed=0
encrypt_pck=false
encrypt_directory=false
script_export_mode=2

[preset.0.options]
binary_format/embed_pck=false
`

// TestExportProjectTool drives a real export. The real export needs installed
// export templates (scripts/download-godot.py with GODOT_TEMPLATES=1); a missing
// template fails the suite rather than passing it.
func TestExportProjectTool(t *testing.T) {
	t.Parallel()
	t.Run("release build", func(t *testing.T) {
		root, session := seedExportProject(t)

		res := callTool(t, session, "export_project", map[string]any{
			"presetName": "Linux",
			"outputPath": "build/game.x86_64",
		})
		text := firstText(t, res)
		if res.IsError {
			t.Fatalf("unexpected error: %s", text)
		}
		requirePrefix(t, text, "Export succeeded.\n\nOutput:")
		assertNonEmptyFile(t, filepath.Join(root, "build", "game.x86_64"))
	})

	t.Run("debug build", func(t *testing.T) {
		root, session := seedExportProject(t)

		res := callTool(t, session, "export_project", map[string]any{
			"presetName": "Linux",
			"outputPath": "build/game_debug.x86_64",
			"debug":      true,
		})
		text := firstText(t, res)
		if res.IsError {
			t.Fatalf("unexpected error: %s", text)
		}
		requirePrefix(t, text, "Export succeeded.\n\nOutput:")
		assertNonEmptyFile(t, filepath.Join(root, "build", "game_debug.x86_64"))

		structured, ok := res.StructuredContent.(map[string]any)
		if !ok {
			t.Fatalf("structuredContent type %T", res.StructuredContent)
		}
		if structured["debug"] != true {
			t.Errorf("debug = %v, want true", structured["debug"])
		}
	})
}

// seedExportProject creates a project with a complete Linux preset and an
// existing output directory, ready for a real export.
func seedExportProject(t *testing.T) (string, *mcp.ClientSession) {
	t.Helper()
	root := newProject(t)
	session := connectProject(t, root)

	writeE2EFile(t, filepath.Join(root, "export_presets.cfg"), exportPresetLinux)
	// Godot packs project resources only, so the project needs at least one
	// file besides project.godot, and it will not create the output directory.
	writeE2EFile(t, filepath.Join(root, "main.gd"), "extends Node\n")
	if err := os.MkdirAll(filepath.Join(root, "build"), 0o755); err != nil {
		t.Fatalf("mkdir build: %v", err)
	}
	return root, session
}
