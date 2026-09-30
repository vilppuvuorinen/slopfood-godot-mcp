package e2e

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestExportProjectValidation(t *testing.T) {
	t.Parallel()
	root := newProject(t)
	session := connectProject(t, root)

	t.Run("missing params", func(t *testing.T) {
		requireToolError(t, session, "export_project", map[string]any{},
			"presetName and outputPath are required.")
	})

	t.Run("invalid output path", func(t *testing.T) {
		requireToolError(t, session, "export_project", map[string]any{
			"presetName": "Linux",
			"outputPath": "../escape",
		}, "Invalid path.")
	})

	t.Run("not a project", func(t *testing.T) {
		notProject := t.TempDir()
		notProjectSession := connectProject(t, notProject)
		requireToolError(t, notProjectSession, "export_project", map[string]any{
			"presetName": "Linux",
			"outputPath": "build/game.x86_64",
		}, "Not a valid Godot project: "+notProject)
	})
}

// Godot requires the filter and options keys; an abbreviated preset is rejected.
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

// Requires installed export templates (scripts/download-godot.py with GODOT_TEMPLATES=1); a missing template fails the suite.
func TestExportProjectTool(t *testing.T) {
	t.Parallel()
	root, session := seedExportProject(t)

	t.Run("release build", func(t *testing.T) {
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

func seedExportProject(t *testing.T) (string, *mcp.ClientSession) {
	t.Helper()
	root := newProject(t)
	session := connectProject(t, root)

	writeE2EFile(t, filepath.Join(root, "export_presets.cfg"), exportPresetLinux)
	// Godot packs project resources only and will not create the output directory.
	writeE2EFile(t, filepath.Join(root, "main.gd"), "extends Node\n")
	if err := os.MkdirAll(filepath.Join(root, "build"), 0o755); err != nil {
		t.Fatalf("mkdir build: %v", err)
	}
	return root, session
}
