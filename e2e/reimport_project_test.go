package e2e

import (
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// TestReimportProjectSchema pins the project-wide scan tool's input surface.
func TestReimportProjectSchema(t *testing.T) {
	t.Parallel()
	schema := requireSchema(t, "reimport_project", "Reimport a Godot project by scanning it in the editor, generating .import sidecars and missing resource UIDs")
	schema.requireRequired(t)
	schema.requirePropCount(t, 0)
}

// TestReimportProjectTool drives the real reimport_project tool against Godot. It
// verifies the two properties the scan must have: it generates a missing .uid
// sidecar for an un-UID'd script, and it leaves existing UIDs (a scene's own
// header uid and an ext_resource uid) untouched.
func TestReimportProjectTool(t *testing.T) {
	t.Parallel()
	root, session := seedScene(t, "scenes/main.tscn")

	// An un-UID'd script that the scan must give a sidecar.
	scriptPath := filepath.Join(root, "player.gd")
	writeE2EFile(t, scriptPath, "extends Node\n")

	// A script that already has a .uid sidecar, referenced by a scene that
	// already carries uid= attributes. The scan must not rewrite them.
	usedPath := filepath.Join(root, "used.gd")
	writeE2EFile(t, usedPath, "extends Node\n")
	writeE2EFile(t, usedPath+".uid", "uid://e2euseduid\n")

	scenePath := filepath.Join(root, "scenes", "main.tscn")
	beforeScene := "[gd_scene load_steps=2 format=3 uid=\"uid://e2esceneuid\"]\n\n" +
		"[ext_resource type=\"Script\" uid=\"uid://e2euseduid\" path=\"res://used.gd\" id=\"1_used\"]\n\n" +
		"[node name=\"root\" type=\"Node2D\"]\nscript = ExtResource(\"1_used\")\n"
	writeE2EFile(t, scenePath, beforeScene)

	text := requireToolText(t, session, "reimport_project", map[string]any{})
	requirePrefix(t, text, "Project reimported successfully.")

	generated := strings.TrimSpace(readFile(t, scriptPath+".uid"))
	if !strings.HasPrefix(generated, "uid://") {
		t.Errorf("generated uid = %q, want a uid:// value", generated)
	}

	afterScene := readFile(t, scenePath)
	if afterScene != beforeScene {
		t.Errorf("existing UIDs were changed by the scan:\nbefore:\n%s\nafter:\n%s", beforeScene, afterScene)
	}
	requireContains(t, afterScene, `uid="uid://e2euseduid"`)
}

// TestReimportProjectAppliesSidecarEdit covers the text-edit-then-scan workflow
// the import surface is built around: the agent edits the .import sidecar text
// directly, then runs reimport_project once to make the edit take effect.
func TestReimportProjectAppliesSidecarEdit(t *testing.T) {
	t.Parallel()
	root := newProject(t)
	writeE2EFile(t, filepath.Join(root, "icon.svg"), `<svg xmlns="http://www.w3.org/2000/svg" width="16" height="16"><rect width="16" height="16" fill="red"/></svg>`)
	session := connectProject(t, root)

	// The scan materializes the first-shot sidecar for a new source.
	requireToolText(t, session, "reimport_project", nil)
	sidecarPath := filepath.Join(root, "icon.svg.import")
	sidecar := readFile(t, sidecarPath)
	requireContains(t, sidecar,
		`importer="texture"`,
		`type="CompressedTexture2D"`,
		`uid="uid://`,
	)

	// Edit the sidecar as text instead of a JSON round-trip through an encoder.
	edited := setImportParam(sidecar, "compress/mode", "1")
	writeE2EFile(t, sidecarPath, edited)

	// The scan is the only step that makes the text edit take effect.
	requireToolText(t, session, "reimport_project", nil)
	requireContains(t, readFile(t, sidecarPath), "compress/mode=1")
}

// setImportParam replaces a [params] key's value, or inserts the key into the
// section, so a test can stage a text edit the way an agent would.
func setImportParam(sidecar, key, value string) string {
	line := regexp.MustCompile(`(?m)^` + regexp.QuoteMeta(key) + `=.*$`)
	if line.MatchString(sidecar) {
		return line.ReplaceAllString(sidecar, key+"="+value)
	}
	header := regexp.MustCompile(`(?m)^\[params\]\s*$`)
	if header.MatchString(sidecar) {
		return header.ReplaceAllString(sidecar, "[params]\n\n"+key+"="+value)
	}
	return sidecar + "\n[params]\n\n" + key + "=" + value + "\n"
}

// TestReimportProjectErrors covers the reference's validation messages.
func TestReimportProjectErrors(t *testing.T) {
	t.Parallel()
	t.Run("not a project", func(t *testing.T) {
		root := t.TempDir()
		session := connectProject(t, root)
		requireToolError(t, session, "reimport_project", map[string]any{},
			"Not a valid Godot project: "+root)
	})
}
