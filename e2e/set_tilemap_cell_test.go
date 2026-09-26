package e2e

import "testing"

// TestSetTilemapCellSchema pins the tool's input surface.
func TestSetTilemapCellSchema(t *testing.T) {
	schema := requireSchema(t, "set_tilemap_cell", "Place a tile on a TileMapLayer node in a scene")
	schema.requireRequired(t, "scenePath", "nodePath", "coords", "sourceId", "atlasCoords")
	schema.requireStringProps(t, "scenePath", "nodePath")
	schema.requireType(t, "sourceId", "integer")
	schema.requireType(t, "alternativeTile", "integer")
	schema.requirePropCount(t, 6)
}

// TestSetTilemapCellTool drives the real set_tilemap_cell tool against Godot.
func TestSetTilemapCellTool(t *testing.T) {
	t.Run("places and reads cells", func(t *testing.T) {
		root := newProject(t)
		session := connectProject(t, root)
		requireGodot(t, session)
		seedAtlasTexture(t, root)
		setupTestTileSet(t, session)
		setupTilemapScene(t, session)

		text := requireToolText(t, session, "set_tilemap_cell", map[string]any{
			"scenePath":   "scenes/level.tscn",
			"nodePath":    "root/Ground",
			"coords":      map[string]any{"$type": "Vector2i", "x": 1, "y": 2},
			"sourceId":    0,
			"atlasCoords": map[string]any{"$type": "Vector2i", "x": 0, "y": 0},
		})
		requirePrefix(t, text, "manage_tilemap succeeded.")

		payload := readTilemapPayload(t, session)
		if payload.ScenePath != "scenes/level.tscn" || payload.NodePath != "root/Ground" {
			t.Errorf("payload = %+v", payload)
		}
		if payload.TileSet != "tiles/set.tres" {
			t.Errorf("tileSet = %q, want tiles/set.tres", payload.TileSet)
		}
		if len(payload.UsedCells) != 1 {
			t.Fatalf("usedCells = %v, want one", payload.UsedCells)
		}
		if cell := payload.UsedCells[0]; cell["sourceId"] != float64(0) {
			t.Errorf("cell = %v", cell)
		}
	})

	t.Run("places a cell with an alternative tile", func(t *testing.T) {
		root := newProject(t)
		session := connectProject(t, root)
		requireGodot(t, session)
		seedAtlasTexture(t, root)
		setupTestTileSet(t, session)
		setupTilemapScene(t, session)

		requireToolText(t, session, "add_tileset_alternative", map[string]any{
			"resourcePath": "tiles/set.tres",
			"sourceId":     0,
			"atlasCoords":  map[string]any{"$type": "Vector2i", "x": 0, "y": 0},
		})
		alternativeID := requireExtraAlternative(t, readTilesetPayload(t, session))

		requireToolText(t, session, "set_tilemap_cell", map[string]any{
			"scenePath":       "scenes/level.tscn",
			"nodePath":        "root/Ground",
			"coords":          map[string]any{"$type": "Vector2i", "x": 0, "y": 0},
			"sourceId":        0,
			"atlasCoords":     map[string]any{"$type": "Vector2i", "x": 0, "y": 0},
			"alternativeTile": alternativeID,
		})

		payload := readTilemapPayload(t, session)
		if len(payload.UsedCells) != 1 {
			t.Fatalf("usedCells = %v, want one", payload.UsedCells)
		}
		if got := payload.UsedCells[0]["alternativeTile"]; got != float64(alternativeID) {
			t.Errorf("alternativeTile = %v, want %d", got, alternativeID)
		}
	})
}

// TestSetTilemapCellErrors covers validation and engine failure messages.
func TestSetTilemapCellErrors(t *testing.T) {
	t.Run("missing params", func(t *testing.T) {
		session := connect(t)
		requireToolError(t, session, "set_tilemap_cell", map[string]any{},
			"scenePath and nodePath are required.")
	})

	t.Run("missing coords", func(t *testing.T) {
		session := connect(t)
		requireToolError(t, session, "set_tilemap_cell", map[string]any{
			"scenePath": "scenes/level.tscn",
			"nodePath":  "root/Ground",
		}, "coords, sourceId, and atlasCoords are required.")
	})

	t.Run("missing node", func(t *testing.T) {
		root := newProject(t)
		session := connectProject(t, root)
		requireGodot(t, session)
		requireToolText(t, session, "create_scene", map[string]any{"scenePath": "scenes/level.tscn", "rootNodeType": "Node2D"})

		res := callTool(t, session, "set_tilemap_cell", map[string]any{
			"scenePath":   "scenes/level.tscn",
			"nodePath":    "root/Missing",
			"coords":      map[string]any{"$type": "Vector2i", "x": 0, "y": 0},
			"sourceId":    0,
			"atlasCoords": map[string]any{"$type": "Vector2i", "x": 0, "y": 0},
		})
		if !res.IsError {
			t.Fatal("set_tilemap_cell: expected an error result")
		}
		requireContains(t, firstText(t, res), "manage_tilemap failed", "Node not found")
	})
}
