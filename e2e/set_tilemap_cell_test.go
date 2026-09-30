package e2e

import "testing"

func TestSetTilemapCellTool(t *testing.T) {
	t.Parallel()
	root := newProject(t)
	session := connectProject(t, root)
	seedAtlasTexture(t, root)

	t.Run("places and reads cells", func(t *testing.T) {
		const (
			set   = "tiles/set_cells.tres"
			scene = "scenes/level_cells.tscn"
		)
		setupTestTileSet(t, session, set)
		setupTilemapScene(t, session, scene, set)

		text := requireToolText(t, session, "set_tilemap_cell", map[string]any{
			"scenePath":   scene,
			"nodePath":    "root/Ground",
			"coords":      map[string]any{"$type": "Vector2i", "x": 1, "y": 2},
			"sourceId":    0,
			"atlasCoords": map[string]any{"$type": "Vector2i", "x": 0, "y": 0},
		})
		requirePrefix(t, text, "manage_tilemap succeeded.")

		payload := readTilemapPayload(t, session, scene)
		if payload.ScenePath != scene || payload.NodePath != "root/Ground" {
			t.Errorf("payload = %+v", payload)
		}
		if payload.TileSet != set {
			t.Errorf("tileSet = %q, want %s", payload.TileSet, set)
		}
		if len(payload.UsedCells) != 1 {
			t.Fatalf("usedCells = %v, want one", payload.UsedCells)
		}
		if cell := payload.UsedCells[0]; cell["sourceId"] != float64(0) {
			t.Errorf("cell = %v", cell)
		}
	})

	t.Run("places a cell with an alternative tile", func(t *testing.T) {
		const (
			set   = "tiles/set_cell_alt.tres"
			scene = "scenes/level_cell_alt.tscn"
		)
		setupTestTileSet(t, session, set)
		setupTilemapScene(t, session, scene, set)

		requireToolText(t, session, "add_tileset_alternative", map[string]any{
			"resourcePath": set,
			"sourceId":     0,
			"atlasCoords":  map[string]any{"$type": "Vector2i", "x": 0, "y": 0},
		})
		alternativeID := requireExtraAlternative(t, readTilesetPayload(t, session, set))

		requireToolText(t, session, "set_tilemap_cell", map[string]any{
			"scenePath":       scene,
			"nodePath":        "root/Ground",
			"coords":          map[string]any{"$type": "Vector2i", "x": 0, "y": 0},
			"sourceId":        0,
			"atlasCoords":     map[string]any{"$type": "Vector2i", "x": 0, "y": 0},
			"alternativeTile": alternativeID,
		})

		payload := readTilemapPayload(t, session, scene)
		if len(payload.UsedCells) != 1 {
			t.Fatalf("usedCells = %v, want one", payload.UsedCells)
		}
		if got := payload.UsedCells[0]["alternativeTile"]; got != float64(alternativeID) {
			t.Errorf("alternativeTile = %v, want %d", got, alternativeID)
		}
	})
}

func TestSetTilemapCellErrors(t *testing.T) {
	t.Parallel()
	validationSession := connect(t)
	t.Run("missing params", func(t *testing.T) {
		requireToolError(t, validationSession, "set_tilemap_cell", map[string]any{},
			"scenePath and nodePath are required.")
	})

	t.Run("missing coords", func(t *testing.T) {
		requireToolError(t, validationSession, "set_tilemap_cell", map[string]any{
			"scenePath": "scenes/level.tscn",
			"nodePath":  "root/Ground",
		}, "coords, sourceId, and atlasCoords are required.")
	})

	t.Run("missing node", func(t *testing.T) {
		root := newProject(t)
		session := connectProject(t, root)
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
