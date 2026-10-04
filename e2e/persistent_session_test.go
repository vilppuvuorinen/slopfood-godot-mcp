package e2e

import (
	"path/filepath"
	"testing"
)

func TestPersistentSessionReloadsEditedScene(t *testing.T) {
	t.Parallel()
	root := newProject(t)
	session := connectProject(t, root)

	writeE2EFile(t, filepath.Join(root, "scenes", "main.tscn"),
		"[gd_scene format=3]\n\n[node name=\"Alpha\" type=\"Sprite2D\"]\n")

	first := requireToolText(t, session, "read", map[string]any{"type": "scene", "path": "scenes/main.tscn"})
	requireContains(t, first, "Alpha")

	scenePath := filepath.Join(root, "scenes", "main.tscn")
	writeE2EFile(t, scenePath, "[gd_scene format=3]\n\n[node name=\"Fresh\" type=\"Node2D\"]\n")

	second := requireToolText(t, session, "read", map[string]any{"type": "scene", "path": "scenes/main.tscn"})
	requireContains(t, second, "Fresh")
	requireNotContains(t, second, "Alpha")
}
