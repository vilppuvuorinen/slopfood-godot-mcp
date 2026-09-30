package e2e

import "testing"

func TestEraseTilemapCellTool(t *testing.T) {
	t.Parallel()
	root := newProject(t)
	session := connectProject(t, root)
	seedAtlasTexture(t, root)
	setupTestTileSet(t, session, "tiles/set.tres")
	setupTilemapScene(t, session, "scenes/level.tscn", "tiles/set.tres")

	requireToolText(t, session, "set_tilemap_cell", map[string]any{
		"scenePath":   "scenes/level.tscn",
		"nodePath":    "root/Ground",
		"coords":      map[string]any{"$type": "Vector2i", "x": 0, "y": 0},
		"sourceId":    0,
		"atlasCoords": map[string]any{"$type": "Vector2i", "x": 0, "y": 0},
	})
	requireToolText(t, session, "erase_tilemap_cell", map[string]any{
		"scenePath": "scenes/level.tscn",
		"nodePath":  "root/Ground",
		"coords":    map[string]any{"$type": "Vector2i", "x": 0, "y": 0},
	})

	payload := readTilemapPayload(t, session, "scenes/level.tscn")
	if len(payload.UsedCells) != 0 {
		t.Errorf("usedCells = %v, want none after erase", payload.UsedCells)
	}
}

func TestEraseTilemapCellErrors(t *testing.T) {
	t.Parallel()
	session := connect(t)
	t.Run("missing params", func(t *testing.T) {
		requireToolError(t, session, "erase_tilemap_cell", map[string]any{},
			"scenePath and nodePath are required.")
	})

	t.Run("missing coords", func(t *testing.T) {
		requireToolError(t, session, "erase_tilemap_cell", map[string]any{
			"scenePath": "scenes/level.tscn",
			"nodePath":  "root/Ground",
		}, "coords is required.")
	})
}
