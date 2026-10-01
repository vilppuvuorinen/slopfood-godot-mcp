package e2e

import (
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func TestRescanProjectTool(t *testing.T) {
	t.Parallel()
	root, session := seedScene(t, "scenes/main.tscn")

	scriptPath := filepath.Join(root, "player.gd")
	writeE2EFile(t, scriptPath, "extends Node\n")

	usedPath := filepath.Join(root, "used.gd")
	writeE2EFile(t, usedPath, "extends Node\n")
	writeE2EFile(t, usedPath+".uid", "uid://e2euseduid\n")

	scenePath := filepath.Join(root, "scenes", "main.tscn")
	beforeScene := "[gd_scene load_steps=2 format=3 uid=\"uid://e2esceneuid\"]\n\n" +
		"[ext_resource type=\"Script\" uid=\"uid://e2euseduid\" path=\"res://used.gd\" id=\"1_used\"]\n\n" +
		"[node name=\"root\" type=\"Node2D\"]\nscript = ExtResource(\"1_used\")\n"
	writeE2EFile(t, scenePath, beforeScene)

	text := requireToolText(t, session, "rescan_project", map[string]any{})
	requirePrefix(t, text, "Project rescanned successfully.")

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

func TestRescanProjectAppliesSidecarEdit(t *testing.T) {
	t.Parallel()
	root := newProject(t)
	writeE2EFile(t, filepath.Join(root, "icon.svg"), `<svg xmlns="http://www.w3.org/2000/svg" width="16" height="16"><rect width="16" height="16" fill="red"/></svg>`)
	session := connectProject(t, root)

	requireToolText(t, session, "rescan_project", nil)
	sidecarPath := filepath.Join(root, "icon.svg.import")
	sidecar := readFile(t, sidecarPath)
	requireContains(t, sidecar,
		`importer="texture"`,
		`type="CompressedTexture2D"`,
		`uid="uid://`,
	)

	edited := setImportParam(sidecar, "compress/mode", "1")
	writeE2EFile(t, sidecarPath, edited)

	requireToolText(t, session, "rescan_project", nil)
	requireContains(t, readFile(t, sidecarPath), "compress/mode=1")
}

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

func TestRescanProjectErrors(t *testing.T) {
	t.Parallel()
	t.Run("not a project", func(t *testing.T) {
		root := t.TempDir()
		session := connectProject(t, root)
		requireToolError(t, session, "rescan_project", map[string]any{},
			"Not a valid Godot project: "+root)
	})
}
