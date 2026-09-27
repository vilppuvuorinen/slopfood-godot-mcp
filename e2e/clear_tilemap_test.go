package e2e

import "testing"

// TestClearTilemapSchema pins the tool's input surface.
func TestClearTilemapSchema(t *testing.T) {
	schema := requireSchema(t, "clear_tilemap", "Clear every cell on a TileMapLayer node in a scene")
	schema.requireRequired(t, "scenePath", "nodePath")
	schema.requireStringProps(t, "scenePath", "nodePath")
	schema.requirePropCount(t, 2)
}

// TestClearTilemapTool drives the real clear_tilemap tool against Godot.
func TestClearTilemapTool(t *testing.T) {
	root := newProject(t)
	session := connectProject(t, root)
	seedAtlasTexture(t, root)
	setupTestTileSet(t, session)
	setupTilemapScene(t, session)

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

	payload := readTilemapPayload(t, session)
	if len(payload.UsedCells) != 0 {
		t.Errorf("usedCells = %v, want none after clear", payload.UsedCells)
	}
}

// TestClearTilemapErrors covers validation messages.
func TestClearTilemapErrors(t *testing.T) {
	session := connect(t)
	requireToolError(t, session, "clear_tilemap", map[string]any{
		"scenePath": "scenes/level.tscn",
	}, "scenePath and nodePath are required.")
}
