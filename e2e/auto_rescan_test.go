package e2e

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"
)

// The daemon watches the project and emits import sidecars for new sources
// without an explicit rescan call.
func TestAutoRescanEmitsSidecarForNewAsset(t *testing.T) {
	t.Parallel()
	root := newProject(t)
	connectProject(t, root)

	svg := filepath.Join(root, "icon.svg")
	writeE2EFile(t, svg, `<svg xmlns="http://www.w3.org/2000/svg" width="16" height="16"><rect width="16" height="16" fill="red"/></svg>`)

	sidecar := svg + ".import"
	waitForFile(t, sidecar, 20*time.Second)
	requireContains(t, readFile(t, sidecar), `importer="texture"`, `uid="uid://`)
}

// A hand-edited import sidecar is reconciled by the watcher without an explicit
// rescan call.
func TestAutoRescanAppliesSidecarEdit(t *testing.T) {
	t.Parallel()
	root := newProject(t)
	connectProject(t, root)

	svg := filepath.Join(root, "icon.svg")
	writeE2EFile(t, svg, `<svg xmlns="http://www.w3.org/2000/svg" width="16" height="16"><rect width="16" height="16" fill="red"/></svg>`)

	sidecar := svg + ".import"
	waitForFile(t, sidecar, 20*time.Second)

	edited := setImportParam(readFile(t, sidecar), "compress/mode", "1")
	writeE2EFile(t, sidecar, edited)

	waitForContains(t, sidecar, "compress/mode=1", 20*time.Second)
}

// A tool call made immediately after an out-of-band save returns with the
// project scanned: the new script's .uid sidecar is present.
func TestToolCallSeesOutOfBandSave(t *testing.T) {
	t.Parallel()
	root, session := seedScene(t, "scenes/main.tscn")

	writeE2EFile(t, filepath.Join(root, "late.gd"), "extends Node\n")
	requireToolText(t, session, "read", map[string]any{"type": "scene", "path": "scenes/main.tscn"})

	if _, err := os.Stat(filepath.Join(root, "late.gd.uid")); err != nil {
		t.Fatalf("late.gd.uid missing after the tool call: %v", err)
	}
}

// The automatic rescan generates a .uid sidecar for a new source while leaving
// an existing UID referenced by a scene intact.
func TestAutoRescanGeneratesMissingUIDs(t *testing.T) {
	t.Parallel()
	root, _ := seedScene(t, "scenes/main.tscn")

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

	waitForFile(t, scriptPath+".uid", 20*time.Second)

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

func waitForFile(t *testing.T, path string, within time.Duration) {
	t.Helper()
	if !pollUntil(within, func() bool {
		info, err := os.Stat(path)
		return err == nil && info.Size() > 0
	}) {
		t.Fatalf("file %s did not appear within %s", path, within)
	}
}

func waitForContains(t *testing.T, path, want string, within time.Duration) {
	t.Helper()
	if !pollUntil(within, func() bool {
		data, err := os.ReadFile(path)
		return err == nil && strings.Contains(string(data), want)
	}) {
		t.Fatalf("%s did not contain %q within %s", path, want, within)
	}
}

func pollUntil(within time.Duration, pred func() bool) bool {
	deadline := time.Now().Add(within)
	for time.Now().Before(deadline) {
		if pred() {
			return true
		}
		time.Sleep(50 * time.Millisecond)
	}
	return pred()
}
