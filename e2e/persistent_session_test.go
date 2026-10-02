package e2e

import (
	"path/filepath"
	"testing"
)

func TestPersistentSessionReloadsEditedScene(t *testing.T) {
	t.Parallel()
	root := newProject(t)
	session := connectProject(t, root)

	requireToolText(t, session, "create_scene", map[string]any{"scenePath": "scenes/main.tscn"})
	requireToolText(t, session, "add_node", map[string]any{
		"scenePath": "scenes/main.tscn", "nodeType": "Sprite2D", "nodeName": "Alpha",
	})

	first := requireToolText(t, session, "read", map[string]any{"type": "scene", "path": "scenes/main.tscn"})
	requireContains(t, first, "Alpha")

	scenePath := filepath.Join(root, "scenes", "main.tscn")
	writeE2EFile(t, scenePath, "[gd_scene format=3]\n\n[node name=\"Fresh\" type=\"Node2D\"]\n")

	second := requireToolText(t, session, "read", map[string]any{"type": "scene", "path": "scenes/main.tscn"})
	requireContains(t, second, "Fresh")
	requireNotContains(t, second, "Alpha")
}
