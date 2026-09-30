package e2e

import "testing"

// TestClearTilemapTool drives the real clear_tilemap tool against Godot.
func TestClearTilemapTool(t *testing.T) {
	t.Parallel()
	root := newProject(t)
	session := connectProject(t, root)
	seedAtlasTexture(t, root)
	setupTestTileSet(t, session, "tiles/set.tres")
	setupTilemapScene(t, session, "scenes/level.tscn", "tiles/set.tres")

	for x := range 2 {
		requireToolText(t, session, "set_tilemap_cell", map[string]any{
			"scenePath":   "scenes/level.tscn",
			"nodePath":    "root/Ground",
			"coords":      map[string]any{"$type": "Vector2i", "x": x, "y": 0},
			"sourceId":    0,
			"atlasCoords": map[string]any{"$type": "Vector2i", "x": 0, "y": 0},
		})
	}
	requireToolText(t, session, "clear_tilemap", map[string]any{
		"scenePath": "scenes/level.tscn",
		"nodePath":  "root/Ground",
	})

	payload := readTilemapPayload(t, session, "scenes/level.tscn")
	if len(payload.UsedCells) != 0 {
		t.Errorf("usedCells = %v, want none after clear", payload.UsedCells)
	}
}

// TestClearTilemapErrors covers validation messages.
func TestClearTilemapErrors(t *testing.T) {
	t.Parallel()
	session := connect(t)
	requireToolError(t, session, "clear_tilemap", map[string]any{
		"scenePath": "scenes/level.tscn",
	}, "scenePath and nodePath are required.")
}
