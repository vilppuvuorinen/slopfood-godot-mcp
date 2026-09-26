package e2e

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// TestManageTilesetSchema pins the action tool's input surface.
func TestManageTilesetSchema(t *testing.T) {
	schema := requireSchema(t, "manage_tileset", "Create or modify TileSet resources and their sources and tiles")
	schema.requireRequired(t, "resourcePath", "action")
	schema.requireStringProps(t, "resourcePath", "action", "tileShape", "tileLayout", "tileOffsetAxis", "sourceType", "scenePath")
	schema.requireType(t, "uvClipping", "boolean")
	schema.requireType(t, "useTexturePadding", "boolean")
	schema.requireType(t, "displayPlaceholder", "boolean")
	schema.requireType(t, "sourceId", "integer")
	schema.requireType(t, "sceneTileId", "integer")
	schema.requireType(t, "alternativeId", "integer")
	schema.requirePropCount(t, 21)
}

// TestReadTilesetSchema pins the read-only tileset tool's input surface.
func TestReadTilesetSchema(t *testing.T) {
	schema := requireSchema(t, "read_tileset", "Read TileSet resources and their sources and tiles")
	schema.requireRequired(t, "resourcePath")
	schema.requireStringProps(t, "resourcePath")
	schema.requirePropCount(t, 1)
}

// TestManageTilemapSchema pins the tilemap tool's input surface.
func TestManageTilemapSchema(t *testing.T) {
	schema := requireSchema(t, "manage_tilemap", "Place, erase, or clear cells on a TileMapLayer node in a scene")
	schema.requireRequired(t, "scenePath", "nodePath", "action")
	schema.requireStringProps(t, "scenePath", "nodePath", "action")
	schema.requireType(t, "sourceId", "integer")
	schema.requireType(t, "alternativeTile", "integer")
	schema.requirePropCount(t, 7)
}

// TestReadTilemapSchema pins the read-only tilemap tool's input surface.
func TestReadTilemapSchema(t *testing.T) {
	schema := requireSchema(t, "read_tilemap", "Read the cells of a TileMapLayer node in a scene")
	schema.requireRequired(t, "scenePath", "nodePath")
	schema.requireStringProps(t, "scenePath", "nodePath")
	schema.requirePropCount(t, 2)
}

// testTilesetPayload is the subset of read_tileset's payload the tests use.
type testTilesetPayload struct {
	Path      string           `json:"path"`
	TileShape string           `json:"tileShape"`
	Sources   []map[string]any `json:"sources"`
}

// readTilesetPayload calls read_tileset and decodes its payload.
func readTilesetPayload(t *testing.T, session *mcp.ClientSession) testTilesetPayload {
	t.Helper()
	text := requireToolText(t, session, "read_tileset", map[string]any{"resourcePath": "tiles/set.tres"})
	var payload testTilesetPayload
	if err := json.Unmarshal([]byte(text), &payload); err != nil {
		t.Fatalf("unmarshal tileset payload: %v\n%s", err, text)
	}
	return payload
}

// seedAtlasTexture writes a Texture2D resource the atlas source can reference.
// A GradientTexture2D needs no import step, unlike a raw PNG.
func seedAtlasTexture(t *testing.T, root string) {
	t.Helper()
	writeE2EFile(t, filepath.Join(root, "tiles", "atlas.tres"),
		"[gd_resource type=\"GradientTexture2D\" format=3]\n\n[resource]\nwidth = 64\nheight = 64\n")
}

// setupTestTileSet creates tiles/set.tres with atlas source 0 holding one tile.
func setupTestTileSet(t *testing.T, session *mcp.ClientSession) {
	t.Helper()
	requireToolText(t, session, "manage_tileset", map[string]any{
		"resourcePath": "tiles/set.tres",
		"action":       "create",
		"tileSize":     map[string]any{"$type": "Vector2i", "x": 16, "y": 16},
	})
	requireToolText(t, session, "manage_tileset", map[string]any{
		"resourcePath":      "tiles/set.tres",
		"action":            "addSource",
		"sourceType":        "atlas",
		"sourceId":          0,
		"texture":           "tiles/atlas.tres",
		"textureRegionSize": map[string]any{"$type": "Vector2i", "x": 16, "y": 16},
	})
	requireToolText(t, session, "manage_tileset", map[string]any{
		"resourcePath": "tiles/set.tres",
		"action":       "addTile",
		"sourceId":     0,
		"atlasCoords":  map[string]any{"$type": "Vector2i", "x": 0, "y": 0},
	})
}

// TestManageTilesetTool drives the real tileset tools against Godot.
func TestManageTilesetTool(t *testing.T) {
	t.Run("creates a tileset with an atlas source and tile", func(t *testing.T) {
		root := newProject(t)
		session := connectProject(t, root)
		requireGodot(t, session)
		seedAtlasTexture(t, root)

		setupTestTileSet(t, session)
		requireContains(t, readFile(t, filepath.Join(root, "tiles", "set.tres")), `[gd_resource type="TileSet"`)

		payload := readTilesetPayload(t, session)
		if payload.Path != "tiles/set.tres" || payload.TileShape != "square" {
			t.Errorf("payload = %+v", payload)
		}
		if len(payload.Sources) != 1 || payload.Sources[0]["type"] != "atlas" {
			t.Fatalf("sources = %v, want one atlas source", payload.Sources)
		}
		if payload.Sources[0]["texture"] != "tiles/atlas.tres" {
			t.Errorf("texture = %v", payload.Sources[0]["texture"])
		}
		tiles, _ := payload.Sources[0]["tiles"].([]any)
		if len(tiles) != 1 {
			t.Fatalf("tiles = %v, want one", tiles)
		}
	})

	t.Run("sets tile data and adds an alternative", func(t *testing.T) {
		root := newProject(t)
		session := connectProject(t, root)
		requireGodot(t, session)
		seedAtlasTexture(t, root)
		setupTestTileSet(t, session)

		requireToolText(t, session, "manage_tileset", map[string]any{
			"resourcePath": "tiles/set.tres",
			"action":       "setTileData",
			"sourceId":     0,
			"atlasCoords":  map[string]any{"$type": "Vector2i", "x": 0, "y": 0},
			"properties":   map[string]any{"z_index": 3},
		})
		requireToolText(t, session, "manage_tileset", map[string]any{
			"resourcePath": "tiles/set.tres",
			"action":       "addAlternative",
			"sourceId":     0,
			"atlasCoords":  map[string]any{"$type": "Vector2i", "x": 0, "y": 0},
		})

		payload := readTilesetPayload(t, session)
		tiles, _ := payload.Sources[0]["tiles"].([]any)
		tile, _ := tiles[0].(map[string]any)
		alternatives, _ := tile["alternatives"].([]any)
		if len(alternatives) != 2 {
			t.Fatalf("alternatives = %v, want two", alternatives)
		}
		base, _ := alternatives[0].(map[string]any)
		data, _ := base["data"].(map[string]any)
		if data["z_index"] != float64(3) {
			t.Errorf("tile data = %v, want z_index 3", data)
		}
	})

	t.Run("adds a scenes source with a scene tile", func(t *testing.T) {
		root := newProject(t)
		session := connectProject(t, root)
		requireGodot(t, session)
		seedAtlasTexture(t, root)
		setupTestTileSet(t, session)
		requireToolText(t, session, "create_scene", map[string]any{"scenePath": "props/box.tscn", "rootNodeType": "Node2D"})

		requireToolText(t, session, "manage_tileset", map[string]any{
			"resourcePath": "tiles/set.tres",
			"action":       "addSource",
			"sourceType":   "scenes",
			"sourceId":     1,
		})
		requireToolText(t, session, "manage_tileset", map[string]any{
			"resourcePath": "tiles/set.tres",
			"action":       "addTile",
			"sourceId":     1,
			"scenePath":    "props/box.tscn",
		})

		payload := readTilesetPayload(t, session)
		if len(payload.Sources) != 2 || payload.Sources[1]["type"] != "scenes" {
			t.Fatalf("sources = %v, want two with a scenes source", payload.Sources)
		}
		tiles, _ := payload.Sources[1]["tiles"].([]any)
		tile, _ := tiles[0].(map[string]any)
		if tile["scene"] != "props/box.tscn" {
			t.Errorf("scene tile = %v", tile)
		}
	})

	t.Run("removes a tile and a source", func(t *testing.T) {
		root := newProject(t)
		session := connectProject(t, root)
		requireGodot(t, session)
		seedAtlasTexture(t, root)
		setupTestTileSet(t, session)

		requireToolText(t, session, "manage_tileset", map[string]any{
			"resourcePath": "tiles/set.tres",
			"action":       "removeTile",
			"sourceId":     0,
			"atlasCoords":  map[string]any{"$type": "Vector2i", "x": 0, "y": 0},
		})
		payload := readTilesetPayload(t, session)
		tiles, _ := payload.Sources[0]["tiles"].([]any)
		if len(tiles) != 0 {
			t.Errorf("tiles = %v, want none after removal", tiles)
		}

		requireToolText(t, session, "manage_tileset", map[string]any{
			"resourcePath": "tiles/set.tres",
			"action":       "removeSource",
			"sourceId":     0,
		})
		payload = readTilesetPayload(t, session)
		if len(payload.Sources) != 0 {
			t.Errorf("sources = %v, want none after removal", payload.Sources)
		}
	})
}

// TestManageTilesetErrors covers the validation and engine failure paths.
func TestManageTilesetErrors(t *testing.T) {
	t.Run("missing params", func(t *testing.T) {
		session := connect(t)
		requireToolError(t, session, "manage_tileset", map[string]any{}, "resourcePath and action are required.")
	})

	t.Run("unknown action", func(t *testing.T) {
		root := newProject(t)
		session := connectProject(t, root)
		requireGodot(t, session)

		res := callTool(t, session, "manage_tileset", map[string]any{"resourcePath": "set.tres", "action": "bogus"})
		if !res.IsError {
			t.Fatal("expected an error result")
		}
		requireContains(t, firstText(t, res), "Unknown action: bogus")
	})

	t.Run("unknown source", func(t *testing.T) {
		root := newProject(t)
		session := connectProject(t, root)
		requireGodot(t, session)
		seedAtlasTexture(t, root)
		setupTestTileSet(t, session)

		res := callTool(t, session, "manage_tileset", map[string]any{
			"resourcePath": "tiles/set.tres",
			"action":       "addTile",
			"sourceId":     9,
			"atlasCoords":  map[string]any{"$type": "Vector2i", "x": 0, "y": 0},
		})
		if !res.IsError {
			t.Fatal("expected an error result")
		}
		requireContains(t, firstText(t, res), "manage_tileset failed", "unknown sourceId: 9")
	})

	t.Run("missing tileset", func(t *testing.T) {
		root := newProject(t)
		session := connectProject(t, root)
		requireGodot(t, session)

		res := callTool(t, session, "read_tileset", map[string]any{"resourcePath": "tiles/missing.tres"})
		if !res.IsError {
			t.Fatal("expected an error result")
		}
		requireContains(t, firstText(t, res), "manage_tileset failed", "TileSet resource not found")
	})

	t.Run("not a tileset resource", func(t *testing.T) {
		root := newProject(t)
		session := connectProject(t, root)
		requireGodot(t, session)

		requireToolText(t, session, "manage_resource", map[string]any{
			"resourcePath": "resources/mat.tres",
			"action":       "create",
			"resourceType": "StandardMaterial3D",
		})
		res := callTool(t, session, "read_tileset", map[string]any{"resourcePath": "resources/mat.tres"})
		if !res.IsError {
			t.Fatal("expected an error result")
		}
		requireContains(t, firstText(t, res), "manage_tileset failed", "Not a TileSet resource")
	})
}

// testTilemapPayload is the subset of read_tilemap's payload the tests use.
type testTilemapPayload struct {
	ScenePath string           `json:"scenePath"`
	NodePath  string           `json:"nodePath"`
	TileSet   string           `json:"tileSet"`
	UsedCells []map[string]any `json:"usedCells"`
}

// readTilemapPayload calls read_tilemap and decodes its payload.
func readTilemapPayload(t *testing.T, session *mcp.ClientSession) testTilemapPayload {
	t.Helper()
	text := requireToolText(t, session, "read_tilemap", map[string]any{
		"scenePath": "scenes/level.tscn",
		"nodePath":  "root/Ground",
	})
	var payload testTilemapPayload
	if err := json.Unmarshal([]byte(text), &payload); err != nil {
		t.Fatalf("unmarshal tilemap payload: %v\n%s", err, text)
	}
	return payload
}

// setupTilemapScene creates a scene whose Ground node is a TileMapLayer bound to
// the tileset built by setupTestTileSet.
func setupTilemapScene(t *testing.T, session *mcp.ClientSession) {
	t.Helper()
	requireToolText(t, session, "create_scene", map[string]any{"scenePath": "scenes/level.tscn", "rootNodeType": "Node2D"})
	requireToolText(t, session, "add_node", map[string]any{
		"scenePath": "scenes/level.tscn",
		"nodeType":  "TileMapLayer",
		"nodeName":  "Ground",
	})
	requireToolText(t, session, "modify_scene_node", map[string]any{
		"scenePath":  "scenes/level.tscn",
		"nodePath":   "root/Ground",
		"properties": map[string]any{"tile_set": "res://tiles/set.tres"},
	})
}

// TestManageTilemapTool drives the real tilemap tools against Godot.
func TestManageTilemapTool(t *testing.T) {
	t.Run("places and reads cells", func(t *testing.T) {
		root := newProject(t)
		session := connectProject(t, root)
		requireGodot(t, session)
		seedAtlasTexture(t, root)
		setupTestTileSet(t, session)
		setupTilemapScene(t, session)

		text := requireToolText(t, session, "manage_tilemap", map[string]any{
			"scenePath":   "scenes/level.tscn",
			"nodePath":    "root/Ground",
			"action":      "setCell",
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
		cell := payload.UsedCells[0]
		if cell["sourceId"] != float64(0) {
			t.Errorf("cell = %v", cell)
		}
	})

	t.Run("erases a cell", func(t *testing.T) {
		root := newProject(t)
		session := connectProject(t, root)
		requireGodot(t, session)
		seedAtlasTexture(t, root)
		setupTestTileSet(t, session)
		setupTilemapScene(t, session)

		requireToolText(t, session, "manage_tilemap", map[string]any{
			"scenePath":   "scenes/level.tscn",
			"nodePath":    "root/Ground",
			"action":      "setCell",
			"coords":      map[string]any{"$type": "Vector2i", "x": 0, "y": 0},
			"sourceId":    0,
			"atlasCoords": map[string]any{"$type": "Vector2i", "x": 0, "y": 0},
		})
		requireToolText(t, session, "manage_tilemap", map[string]any{
			"scenePath": "scenes/level.tscn",
			"nodePath":  "root/Ground",
			"action":    "eraseCell",
			"coords":    map[string]any{"$type": "Vector2i", "x": 0, "y": 0},
		})

		payload := readTilemapPayload(t, session)
		if len(payload.UsedCells) != 0 {
			t.Errorf("usedCells = %v, want none after erase", payload.UsedCells)
		}
	})

	t.Run("clears every cell", func(t *testing.T) {
		root := newProject(t)
		session := connectProject(t, root)
		requireGodot(t, session)
		seedAtlasTexture(t, root)
		setupTestTileSet(t, session)
		setupTilemapScene(t, session)

		for x := range 2 {
			requireToolText(t, session, "manage_tilemap", map[string]any{
				"scenePath":   "scenes/level.tscn",
				"nodePath":    "root/Ground",
				"action":      "setCell",
				"coords":      map[string]any{"$type": "Vector2i", "x": x, "y": 0},
				"sourceId":    0,
				"atlasCoords": map[string]any{"$type": "Vector2i", "x": 0, "y": 0},
			})
		}
		requireToolText(t, session, "manage_tilemap", map[string]any{
			"scenePath": "scenes/level.tscn",
			"nodePath":  "root/Ground",
			"action":    "clear",
		})

		payload := readTilemapPayload(t, session)
		if len(payload.UsedCells) != 0 {
			t.Errorf("usedCells = %v, want none after clear", payload.UsedCells)
		}
	})
}

// TestManageTilemapErrors covers the validation and engine failure paths.
func TestManageTilemapErrors(t *testing.T) {
	t.Run("missing params", func(t *testing.T) {
		session := connect(t)
		requireToolError(t, session, "manage_tilemap", map[string]any{}, "scenePath, nodePath, and action are required.")
	})

	t.Run("setCell without coords", func(t *testing.T) {
		session := connect(t)
		requireToolError(t, session, "manage_tilemap", map[string]any{
			"scenePath": "scenes/level.tscn",
			"nodePath":  "root/Ground",
			"action":    "setCell",
		}, "coords is required for the setCell action.")
	})

	t.Run("unknown action", func(t *testing.T) {
		root := newProject(t)
		session := connectProject(t, root)
		requireGodot(t, session)

		res := callTool(t, session, "manage_tilemap", map[string]any{
			"scenePath": "scenes/level.tscn",
			"nodePath":  "root/Ground",
			"action":    "bogus",
		})
		if !res.IsError {
			t.Fatal("expected an error result")
		}
		requireContains(t, firstText(t, res), "Unknown action: bogus")
	})

	t.Run("missing node", func(t *testing.T) {
		root := newProject(t)
		session := connectProject(t, root)
		requireGodot(t, session)
		requireToolText(t, session, "create_scene", map[string]any{"scenePath": "scenes/level.tscn", "rootNodeType": "Node2D"})

		res := callTool(t, session, "read_tilemap", map[string]any{
			"scenePath": "scenes/level.tscn",
			"nodePath":  "root/Missing",
		})
		if !res.IsError {
			t.Fatal("expected an error result")
		}
		requireContains(t, firstText(t, res), "manage_tilemap failed", "Node not found")
	})

	t.Run("node is not a TileMapLayer", func(t *testing.T) {
		root := newProject(t)
		session := connectProject(t, root)
		requireGodot(t, session)
		requireToolText(t, session, "create_scene", map[string]any{"scenePath": "scenes/level.tscn", "rootNodeType": "Node2D"})
		requireToolText(t, session, "add_node", map[string]any{
			"scenePath": "scenes/level.tscn",
			"nodeType":  "Sprite2D",
			"nodeName":  "Art",
		})

		res := callTool(t, session, "read_tilemap", map[string]any{
			"scenePath": "scenes/level.tscn",
			"nodePath":  "root/Art",
		})
		if !res.IsError {
			t.Fatal("expected an error result")
		}
		requireContains(t, firstText(t, res), "manage_tilemap failed", "Node is not a TileMapLayer")
	})

	t.Run("missing scene", func(t *testing.T) {
		root := newProject(t)
		session := connectProject(t, root)
		requireGodot(t, session)

		requireToolError(t, session, "read_tilemap", map[string]any{
			"scenePath": "scenes/missing.tscn",
			"nodePath":  "root/Ground",
		}, "Scene file does not exist: scenes/missing.tscn")
	})
}
