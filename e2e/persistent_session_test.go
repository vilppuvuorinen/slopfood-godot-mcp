package e2e

import (
	"path/filepath"
	"testing"
)

// TestPersistentSessionReloadsEditedScene proves the persistent session serves a
// fresh view of the project even after an out-of-band edit. It is the only
// bridge the server has now, so this exercises it on every run. A warm engine
// that cached the scene would keep reporting the stale node; CACHE_MODE_IGNORE
// on read must show the new one.
func TestPersistentSessionReloadsEditedScene(t *testing.T) {
	t.Parallel()
	root := newProject(t)
	session := connectProject(t, root)

	requireToolText(t, session, "create_scene", map[string]any{"scenePath": "scenes/main.tscn"})
	requireToolText(t, session, "add_node", map[string]any{
		"scenePath": "scenes/main.tscn", "nodeType": "Sprite2D", "nodeName": "Alpha",
	})

	first := requireToolText(t, session, "read_scene", map[string]any{"scenePath": "scenes/main.tscn"})
	requireContains(t, first, "Alpha")

	// Edit the scene file out of band. A warm engine that skipped reloading
	// would keep serving the cached scene; CACHE_MODE_IGNORE must show the new one.
	scenePath := filepath.Join(root, "scenes", "main.tscn")
	writeE2EFile(t, scenePath, "[gd_scene format=3]\n\n[node name=\"Fresh\" type=\"Node2D\"]\n")

	second := requireToolText(t, session, "read_scene", map[string]any{"scenePath": "scenes/main.tscn"})
	requireContains(t, second, "Fresh")
	requireNotContains(t, second, "Alpha")
}
