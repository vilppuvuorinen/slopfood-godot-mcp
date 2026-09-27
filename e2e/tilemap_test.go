package e2e

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// TestCreateTilesetSchema pins the create tool's input surface.
func TestCreateTilesetSchema(t *testing.T) {
	t.Parallel()
	schema := requireSchema(t, "create_tileset", "Create a TileSet resource")
	schema.requireRequired(t, "resourcePath")
	schema.requireStringProps(t, "resourcePath", "tileShape", "tileLayout", "tileOffsetAxis")
	schema.requireType(t, "uvClipping", "boolean")
	schema.requirePropCount(t, 6)
}

// TestModifyTilesetSchema pins the modify tool's input surface.
func TestModifyTilesetSchema(t *testing.T) {
	t.Parallel()
	schema := requireSchema(t, "modify_tileset", "Set a TileSet resource's tile size, shape, layout, and UV clipping")
	schema.requireRequired(t, "resourcePath")
	schema.requireStringProps(t, "resourcePath", "tileShape", "tileLayout", "tileOffsetAxis")
	schema.requireType(t, "uvClipping", "boolean")
	schema.requirePropCount(t, 6)
}

// TestAddTilesetSourceSchema pins the add-source tool's input surface.
func TestAddTilesetSourceSchema(t *testing.T) {
	t.Parallel()
	schema := requireSchema(t, "add_tileset_source", "Add an atlas or scenes-collection source to a TileSet")
	schema.requireRequired(t, "resourcePath", "sourceType")
	schema.requireStringProps(t, "resourcePath", "sourceType")
	schema.requireType(t, "sourceId", "integer")
	schema.requireType(t, "useTexturePadding", "boolean")
	schema.requirePropCount(t, 8)
}

// TestRemoveTilesetSourceSchema pins the remove-source tool's input surface.
func TestRemoveTilesetSourceSchema(t *testing.T) {
	t.Parallel()
	schema := requireSchema(t, "remove_tileset_source", "Remove a source from a TileSet")
	schema.requireRequired(t, "resourcePath", "sourceId")
	schema.requireStringProps(t, "resourcePath")
	schema.requireType(t, "sourceId", "integer")
	schema.requirePropCount(t, 2)
}

// TestAddTilesetTileSchema pins the add-tile tool's input surface.
func TestAddTilesetTileSchema(t *testing.T) {
	t.Parallel()
	schema := requireSchema(t, "add_tileset_tile", "Add a tile to a TileSet source")
	schema.requireRequired(t, "resourcePath", "sourceId")
	schema.requireStringProps(t, "resourcePath", "scenePath")
	schema.requireType(t, "sourceId", "integer")
	schema.requireType(t, "sceneTileId", "integer")
	schema.requireType(t, "displayPlaceholder", "boolean")
	schema.requirePropCount(t, 7)
}

// TestRemoveTilesetTileSchema pins the remove-tile tool's input surface.
func TestRemoveTilesetTileSchema(t *testing.T) {
	t.Parallel()
	schema := requireSchema(t, "remove_tileset_tile", "Remove a tile from a TileSet source")
	schema.requireRequired(t, "resourcePath", "sourceId")
	schema.requireStringProps(t, "resourcePath")
	schema.requireType(t, "sourceId", "integer")
	schema.requireType(t, "sceneTileId", "integer")
	schema.requirePropCount(t, 4)
}

// TestAddTilesetAlternativeSchema pins the add-alternative tool's input surface.
func TestAddTilesetAlternativeSchema(t *testing.T) {
	t.Parallel()
	schema := requireSchema(t, "add_tileset_alternative", "Add an alternative tile to a TileSet atlas tile")
	schema.requireRequired(t, "resourcePath", "sourceId", "atlasCoords")
	schema.requireStringProps(t, "resourcePath")
	schema.requireType(t, "sourceId", "integer")
	schema.requireType(t, "alternativeId", "integer")
	schema.requirePropCount(t, 4)
}

// TestRemoveTilesetAlternativeSchema pins the remove-alternative tool's input surface.
func TestRemoveTilesetAlternativeSchema(t *testing.T) {
	t.Parallel()
	schema := requireSchema(t, "remove_tileset_alternative", "Remove an alternative tile from a TileSet atlas tile")
	schema.requireRequired(t, "resourcePath", "sourceId", "atlasCoords", "alternativeId")
	schema.requireStringProps(t, "resourcePath")
	schema.requireType(t, "sourceId", "integer")
	schema.requireType(t, "alternativeId", "integer")
	schema.requirePropCount(t, 4)
}

// TestSetTilesetTileDataSchema pins the set-tile-data tool's input surface.
func TestSetTilesetTileDataSchema(t *testing.T) {
	t.Parallel()
	schema := requireSchema(t, "set_tileset_tile_data", "Set properties on a TileSet atlas tile's data")
	schema.requireRequired(t, "resourcePath", "sourceId", "atlasCoords", "properties")
	schema.requireStringProps(t, "resourcePath")
	schema.requireType(t, "sourceId", "integer")
	schema.requireType(t, "alternativeId", "integer")
	schema.requireType(t, "properties", "object")
	schema.requirePropCount(t, 5)
}

// TestReadTilesetSchema pins the read-only tileset tool's input surface.
func TestReadTilesetSchema(t *testing.T) {
	t.Parallel()
	schema := requireSchema(t, "read_tileset", "Read TileSet resources and their sources and tiles")
	schema.requireRequired(t, "resourcePath")
	schema.requireStringProps(t, "resourcePath")
	schema.requirePropCount(t, 1)
}

// TestReadTilemapSchema pins the read-only tilemap tool's input surface.
func TestReadTilemapSchema(t *testing.T) {
	t.Parallel()
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

// tileAlternatives returns the alternatives of the first tile of the first
// source in a tileset payload.
func tileAlternatives(t *testing.T, payload testTilesetPayload) []map[string]any {
	t.Helper()
	if len(payload.Sources) == 0 {
		t.Fatal("tileset has no sources")
	}
	tiles, _ := payload.Sources[0]["tiles"].([]any)
	if len(tiles) == 0 {
		t.Fatal("atlas source has no tiles")
	}
	tile, ok := tiles[0].(map[string]any)
	if !ok {
		t.Fatalf("tile = %#v", tiles[0])
	}
	raw, _ := tile["alternatives"].([]any)
	out := make([]map[string]any, 0, len(raw))
	for _, item := range raw {
		alt, ok := item.(map[string]any)
		if !ok {
			t.Fatalf("alternative = %#v", item)
		}
		out = append(out, alt)
	}
	return out
}

// requireExtraAlternative returns the id of the first non-base alternative on
// the first tile. The base tile is alternative 0, so a freshly added
// alternative carries a positive id.
func requireExtraAlternative(t *testing.T, payload testTilesetPayload) int {
	t.Helper()
	for _, alt := range tileAlternatives(t, payload) {
		if id, ok := alt["id"].(float64); ok && id != 0 {
			return int(id)
		}
	}
	t.Fatalf("no non-base alternative in %v", tileAlternatives(t, payload))
	return 0
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
	requireToolText(t, session, "create_tileset", map[string]any{
		"resourcePath": "tiles/set.tres",
		"tileSize":     map[string]any{"$type": "Vector2i", "x": 16, "y": 16},
	})
	requireToolText(t, session, "add_tileset_source", map[string]any{
		"resourcePath":      "tiles/set.tres",
		"sourceType":        "atlas",
		"sourceId":          0,
		"texture":           "tiles/atlas.tres",
		"textureRegionSize": map[string]any{"$type": "Vector2i", "x": 16, "y": 16},
	})
	requireToolText(t, session, "add_tileset_tile", map[string]any{
		"resourcePath": "tiles/set.tres",
		"sourceId":     0,
		"atlasCoords":  map[string]any{"$type": "Vector2i", "x": 0, "y": 0},
	})
}

// TestTilesetTools drives the real tileset tools against Godot.
func TestTilesetTools(t *testing.T) {
	t.Parallel()
	t.Run("creates a tileset with an atlas source and tile", func(t *testing.T) {
		root := newProject(t)
		session := connectProject(t, root)
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
		seedAtlasTexture(t, root)
		setupTestTileSet(t, session)

		requireToolText(t, session, "set_tileset_tile_data", map[string]any{
			"resourcePath": "tiles/set.tres",
			"sourceId":     0,
			"atlasCoords":  map[string]any{"$type": "Vector2i", "x": 0, "y": 0},
			"properties":   map[string]any{"z_index": 3},
		})
		requireToolText(t, session, "add_tileset_alternative", map[string]any{
			"resourcePath": "tiles/set.tres",
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

	t.Run("removes a tile alternative", func(t *testing.T) {
		root := newProject(t)
		session := connectProject(t, root)
		seedAtlasTexture(t, root)
		setupTestTileSet(t, session)

		requireToolText(t, session, "add_tileset_alternative", map[string]any{
			"resourcePath": "tiles/set.tres",
			"sourceId":     0,
			"atlasCoords":  map[string]any{"$type": "Vector2i", "x": 0, "y": 0},
		})
		added := readTilesetPayload(t, session)
		if got := len(tileAlternatives(t, added)); got != 2 {
			t.Fatalf("alternatives after add = %d, want 2", got)
		}
		alternativeID := requireExtraAlternative(t, added)

		requireToolText(t, session, "remove_tileset_alternative", map[string]any{
			"resourcePath":  "tiles/set.tres",
			"sourceId":      0,
			"atlasCoords":   map[string]any{"$type": "Vector2i", "x": 0, "y": 0},
			"alternativeId": alternativeID,
		})

		remaining := tileAlternatives(t, readTilesetPayload(t, session))
		if len(remaining) != 1 {
			t.Fatalf("alternatives after remove = %v, want only the base", remaining)
		}
		if base, _ := remaining[0]["id"].(float64); base != 0 {
			t.Errorf("remaining alternative id = %v, want the base 0", base)
		}
	})

	t.Run("adds and removes a scenes source tile", func(t *testing.T) {
		root := newProject(t)
		session := connectProject(t, root)
		seedAtlasTexture(t, root)
		setupTestTileSet(t, session)
		requireToolText(t, session, "create_scene", map[string]any{"scenePath": "props/box.tscn", "rootNodeType": "Node2D"})

		requireToolText(t, session, "add_tileset_source", map[string]any{
			"resourcePath": "tiles/set.tres",
			"sourceType":   "scenes",
			"sourceId":     1,
		})
		requireToolText(t, session, "add_tileset_tile", map[string]any{
			"resourcePath": "tiles/set.tres",
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
			t.Fatalf("scene tile = %v", tile)
		}
		sceneTileID, ok := tile["sceneTileId"].(float64)
		if !ok {
			t.Fatalf("sceneTileId = %#v, want a number", tile["sceneTileId"])
		}

		// A scenes-collection tile is removed by sceneTileId, not atlasCoords.
		requireToolText(t, session, "remove_tileset_tile", map[string]any{
			"resourcePath": "tiles/set.tres",
			"sourceId":     1,
			"sceneTileId":  int(sceneTileID),
		})

		payload = readTilesetPayload(t, session)
		tiles, _ = payload.Sources[1]["tiles"].([]any)
		if len(tiles) != 0 {
			t.Errorf("scene tiles after removal = %v, want none", tiles)
		}
	})

	t.Run("removes a tile and a source", func(t *testing.T) {
		root := newProject(t)
		session := connectProject(t, root)
		seedAtlasTexture(t, root)
		setupTestTileSet(t, session)

		requireToolText(t, session, "remove_tileset_tile", map[string]any{
			"resourcePath": "tiles/set.tres",
			"sourceId":     0,
			"atlasCoords":  map[string]any{"$type": "Vector2i", "x": 0, "y": 0},
		})
		payload := readTilesetPayload(t, session)
		tiles, _ := payload.Sources[0]["tiles"].([]any)
		if len(tiles) != 0 {
			t.Errorf("tiles = %v, want none after removal", tiles)
		}

		requireToolText(t, session, "remove_tileset_source", map[string]any{
			"resourcePath": "tiles/set.tres",
			"sourceId":     0,
		})
		payload = readTilesetPayload(t, session)
		if len(payload.Sources) != 0 {
			t.Errorf("sources = %v, want none after removal", payload.Sources)
		}
	})
}

// TestTilesetErrors covers the validation and engine failure paths.
func TestTilesetErrors(t *testing.T) {
	t.Parallel()
	t.Run("missing params", func(t *testing.T) {
		session := connect(t)
		requireToolError(t, session, "create_tileset", map[string]any{}, "resourcePath is required.")
	})

	t.Run("addAlternative without atlas coords", func(t *testing.T) {
		session := connect(t)
		requireToolError(t, session, "add_tileset_alternative", map[string]any{
			"resourcePath": "tiles/set.tres",
			"sourceId":     0,
		}, "resourcePath, sourceId, and atlasCoords are required.")
	})

	t.Run("unknown source", func(t *testing.T) {
		root := newProject(t)
		session := connectProject(t, root)
		seedAtlasTexture(t, root)
		setupTestTileSet(t, session)

		res := callTool(t, session, "add_tileset_tile", map[string]any{
			"resourcePath": "tiles/set.tres",
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

		res := callTool(t, session, "read_tileset", map[string]any{"resourcePath": "tiles/missing.tres"})
		if !res.IsError {
			t.Fatal("expected an error result")
		}
		requireContains(t, firstText(t, res), "manage_tileset failed", "TileSet resource not found")
	})

	t.Run("not a tileset resource", func(t *testing.T) {
		root := newProject(t)
		session := connectProject(t, root)

		requireToolText(t, session, "create_resource", map[string]any{
			"resourcePath": "resources/mat.tres",
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

// TestReadTilemapErrors covers the read tool's engine failure paths.
func TestReadTilemapErrors(t *testing.T) {
	t.Parallel()
	t.Run("missing node", func(t *testing.T) {
		root := newProject(t)
		session := connectProject(t, root)
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

		requireToolError(t, session, "read_tilemap", map[string]any{
			"scenePath": "scenes/missing.tscn",
			"nodePath":  "root/Ground",
		}, "Scene file does not exist: scenes/missing.tscn")
	})
}
