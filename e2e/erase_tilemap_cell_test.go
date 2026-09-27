package e2e

import "testing"

// TestEraseTilemapCellSchema pins the tool's input surface.
func TestEraseTilemapCellSchema(t *testing.T) {
	schema := requireSchema(t, "erase_tilemap_cell", "Erase a tile from a TileMapLayer node in a scene")
	schema.requireRequired(t, "scenePath", "nodePath", "coords")
	schema.requireStringProps(t, "scenePath", "nodePath")
	schema.requirePropCount(t, 3)
}

// TestEraseTilemapCellTool drives the real erase_tilemap_cell tool against
// Godot.
func TestEraseTilemapCellTool(t *testing.T) {
	root := newProject(t)
	session := connectProject(t, root)
	seedAtlasTexture(t, root)
	setupTestTileSet(t, session)
	setupTilemapScene(t, session)

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

	payload := readTilemapPayload(t, session)
	if len(payload.UsedCells) != 0 {
		t.Errorf("usedCells = %v, want none after erase", payload.UsedCells)
	}
}

// TestEraseTilemapCellErrors covers validation messages.
func TestEraseTilemapCellErrors(t *testing.T) {
	t.Run("missing params", func(t *testing.T) {
		session := connect(t)
		requireToolError(t, session, "erase_tilemap_cell", map[string]any{},
			"scenePath and nodePath are required.")
	})

	t.Run("missing coords", func(t *testing.T) {
		session := connect(t)
		requireToolError(t, session, "erase_tilemap_cell", map[string]any{
			"scenePath": "scenes/level.tscn",
			"nodePath":  "root/Ground",
		}, "coords is required.")
	})
}
