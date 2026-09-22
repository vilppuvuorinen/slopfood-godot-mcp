package e2e

import (
	"path/filepath"
	"strings"
	"testing"
)

// TestExportProjectSchema ports the reference's tool-definition expectations
// for export_project over the real stdio surface.
func TestExportProjectSchema(t *testing.T) {
	schema := requireSchema(t, "export_project", "Export a Godot project using a preset")
	schema.requireRequired(t, "projectPath", "presetName", "outputPath")
	schema.requirePropCount(t, 4)
	schema.requireStringProps(t, "projectPath", "presetName", "outputPath")
	schema.requireType(t, "debug", "boolean")
}

// TestExportProjectValidation covers the reference's validation messages.
func TestExportProjectValidation(t *testing.T) {
	session := connect(t)

	t.Run("missing params", func(t *testing.T) {
		requireToolError(t, session, "export_project", map[string]any{},
			"projectPath, presetName, and outputPath are required.")
	})

	t.Run("invalid path", func(t *testing.T) {
		requireToolError(t, session, "export_project", map[string]any{
			"projectPath": "../escape",
			"presetName":  "Linux",
			"outputPath":  "build/game.x86_64",
		}, "Invalid project path.")
	})

	t.Run("not a project", func(t *testing.T) {
		root := t.TempDir()
		requireToolError(t, session, "export_project", map[string]any{
			"projectPath": root,
			"presetName":  "Linux",
			"outputPath":  "build/game.x86_64",
		}, "Not a valid Godot project: "+root)
	})
}

// TestExportProjectTool drives a real export. Export templates are usually not
// installed, so a template error skips rather than fails the suite.
func TestExportProjectTool(t *testing.T) {
	session := connect(t)
	requireGodot(t, session)

	root := t.TempDir()
	seedProject(t, root)
	writeE2EFile(t, filepath.Join(root, "export_presets.cfg"),
		"[preset.0]\n\nname=\"Linux\"\nplatform=\"Linux\"\nrunnable=true\n")

	res := callTool(t, session, "export_project", map[string]any{
		"projectPath": root,
		"presetName":  "Linux",
		"outputPath":  "build/game.x86_64",
	})
	text := firstText(t, res)
	if res.IsError {
		if strings.Contains(text, "export template") {
			t.Skipf("export templates unavailable, skipping: %s", text)
		}
		t.Fatalf("unexpected error: %s", text)
	}
	requirePrefix(t, text, "Export succeeded.\n\nOutput:")
}
