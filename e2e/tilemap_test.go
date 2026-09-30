package e2e

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type testTilesetPayload struct {
	Path      string           `json:"path"`
	TileShape string           `json:"tileShape"`
	Sources   []map[string]any `json:"sources"`
}

func readTilesetPayload(t *testing.T, session *mcp.ClientSession, resourcePath string) testTilesetPayload {
	t.Helper()
	text := requireToolText(t, session, "read_tileset", map[string]any{"resourcePath": resourcePath})
	var payload testTilesetPayload
	if err := json.Unmarshal([]byte(text), &payload); err != nil {
		t.Fatalf("unmarshal tileset payload: %v\n%s", err, text)
	}
	return payload
}

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

// The base tile is alternative 0, so a fresh alternative carries a positive id.
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

// A GradientTexture2D needs no import step, unlike a raw PNG.
func seedAtlasTexture(t *testing.T, root string) {
	t.Helper()
	writeE2EFile(t, filepath.Join(root, "tiles", "atlas.tres"),
		"[gd_resource type=\"GradientTexture2D\" format=3]\n\n[resource]\nwidth = 64\nheight = 64\n")
}

func setupTestTileSet(t *testing.T, session *mcp.ClientSession, resourcePath string) {
	t.Helper()
	requireToolText(t, session, "create_tileset", map[string]any{
		"resourcePath": resourcePath,
		"tileSize":     map[string]any{"$type": "Vector2i", "x": 16, "y": 16},
	})
	requireToolText(t, session, "add_tileset_source", map[string]any{
		"resourcePath":      resourcePath,
		"sourceType":        "atlas",
		"sourceId":          0,
		"texture":           "tiles/atlas.tres",
		"textureRegionSize": map[string]any{"$type": "Vector2i", "x": 16, "y": 16},
	})
	requireToolText(t, session, "add_tileset_tile", map[string]any{
		"resourcePath": resourcePath,
		"sourceId":     0,
		"atlasCoords":  map[string]any{"$type": "Vector2i", "x": 0, "y": 0},
	})
}

func TestTilesetTools(t *testing.T) {
	t.Parallel()
	root := newProject(t)
	session := connectProject(t, root)
	seedAtlasTexture(t, root)

	t.Run("creates a tileset with an atlas source and tile", func(t *testing.T) {
		const set = "tiles/set_create.tres"
		setupTestTileSet(t, session, set)
		requireContains(t, readFile(t, filepath.Join(root, "tiles", "set_create.tres")), `[gd_resource type="TileSet"`)

		payload := readTilesetPayload(t, session, set)
		if payload.Path != set || payload.TileShape != "square" {
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
		const set = "tiles/set_data.tres"
		setupTestTileSet(t, session, set)

		requireToolText(t, session, "set_tileset_tile_data", map[string]any{
			"resourcePath": set,
			"sourceId":     0,
			"atlasCoords":  map[string]any{"$type": "Vector2i", "x": 0, "y": 0},
			"properties":   map[string]any{"z_index": 3},
		})
		requireToolText(t, session, "add_tileset_alternative", map[string]any{
			"resourcePath": set,
			"sourceId":     0,
			"atlasCoords":  map[string]any{"$type": "Vector2i", "x": 0, "y": 0},
		})

		payload := readTilesetPayload(t, session, set)
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
		const set = "tiles/set_alt.tres"
		setupTestTileSet(t, session, set)

		requireToolText(t, session, "add_tileset_alternative", map[string]any{
			"resourcePath": set,
			"sourceId":     0,
			"atlasCoords":  map[string]any{"$type": "Vector2i", "x": 0, "y": 0},
		})
		added := readTilesetPayload(t, session, set)
		if got := len(tileAlternatives(t, added)); got != 2 {
			t.Fatalf("alternatives after add = %d, want 2", got)
		}
		alternativeID := requireExtraAlternative(t, added)

		requireToolText(t, session, "remove_tileset_alternative", map[string]any{
			"resourcePath":  set,
			"sourceId":      0,
			"atlasCoords":   map[string]any{"$type": "Vector2i", "x": 0, "y": 0},
			"alternativeId": alternativeID,
		})

		remaining := tileAlternatives(t, readTilesetPayload(t, session, set))
		if len(remaining) != 1 {
			t.Fatalf("alternatives after remove = %v, want only the base", remaining)
		}
		if base, _ := remaining[0]["id"].(float64); base != 0 {
			t.Errorf("remaining alternative id = %v, want the base 0", base)
		}
	})

	t.Run("adds and removes a scenes source tile", func(t *testing.T) {
		const set = "tiles/set_scenes.tres"
		setupTestTileSet(t, session, set)
		requireToolText(t, session, "create_scene", map[string]any{"scenePath": "props/box.tscn", "rootNodeType": "Node2D"})

		requireToolText(t, session, "add_tileset_source", map[string]any{
			"resourcePath": set,
			"sourceType":   "scenes",
			"sourceId":     1,
		})
		requireToolText(t, session, "add_tileset_tile", map[string]any{
			"resourcePath": set,
			"sourceId":     1,
			"scenePath":    "props/box.tscn",
		})

		payload := readTilesetPayload(t, session, set)
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
			"resourcePath": set,
			"sourceId":     1,
			"sceneTileId":  int(sceneTileID),
		})

		payload = readTilesetPayload(t, session, set)
		tiles, _ = payload.Sources[1]["tiles"].([]any)
		if len(tiles) != 0 {
			t.Errorf("scene tiles after removal = %v, want none", tiles)
		}
	})

	t.Run("removes a tile and a source", func(t *testing.T) {
		const set = "tiles/set_remove.tres"
		setupTestTileSet(t, session, set)

		requireToolText(t, session, "remove_tileset_tile", map[string]any{
			"resourcePath": set,
			"sourceId":     0,
			"atlasCoords":  map[string]any{"$type": "Vector2i", "x": 0, "y": 0},
		})
		payload := readTilesetPayload(t, session, set)
		tiles, _ := payload.Sources[0]["tiles"].([]any)
		if len(tiles) != 0 {
			t.Errorf("tiles = %v, want none after removal", tiles)
		}

		requireToolText(t, session, "remove_tileset_source", map[string]any{
			"resourcePath": set,
			"sourceId":     0,
		})
		payload = readTilesetPayload(t, session, set)
		if len(payload.Sources) != 0 {
			t.Errorf("sources = %v, want none after removal", payload.Sources)
		}
	})
}

func TestTilesetErrors(t *testing.T) {
	t.Parallel()
	validationSession := connect(t)
	t.Run("missing params", func(t *testing.T) {
		requireToolError(t, validationSession, "create_tileset", map[string]any{}, "resourcePath is required.")
	})

	t.Run("addAlternative without atlas coords", func(t *testing.T) {
		requireToolError(t, validationSession, "add_tileset_alternative", map[string]any{
			"resourcePath": "tiles/set.tres",
			"sourceId":     0,
		}, "resourcePath, sourceId, and atlasCoords are required.")
	})

	root := newProject(t)
	session := connectProject(t, root)
	seedAtlasTexture(t, root)

	t.Run("unknown source", func(t *testing.T) {
		const set = "tiles/set_unknown.tres"
		setupTestTileSet(t, session, set)

		res := callTool(t, session, "add_tileset_tile", map[string]any{
			"resourcePath": set,
			"sourceId":     9,
			"atlasCoords":  map[string]any{"$type": "Vector2i", "x": 0, "y": 0},
		})
		if !res.IsError {
			t.Fatal("expected an error result")
		}
		requireContains(t, firstText(t, res), "manage_tileset failed", "unknown sourceId: 9")
	})

	t.Run("missing tileset", func(t *testing.T) {
		res := callTool(t, session, "read_tileset", map[string]any{"resourcePath": "tiles/missing.tres"})
		if !res.IsError {
			t.Fatal("expected an error result")
		}
		requireContains(t, firstText(t, res), "manage_tileset failed", "TileSet resource not found")
	})

	t.Run("not a tileset resource", func(t *testing.T) {
		requireToolText(t, session, "create_resource", map[string]any{
			"resourcePath": "resources/mat_tileset.tres",
			"resourceType": "StandardMaterial3D",
		})
		res := callTool(t, session, "read_tileset", map[string]any{"resourcePath": "resources/mat_tileset.tres"})
		if !res.IsError {
			t.Fatal("expected an error result")
		}
		requireContains(t, firstText(t, res), "manage_tileset failed", "Not a TileSet resource")
	})
}

type testTilemapPayload struct {
	ScenePath string           `json:"scenePath"`
	NodePath  string           `json:"nodePath"`
	TileSet   string           `json:"tileSet"`
	UsedCells []map[string]any `json:"usedCells"`
}

func readTilemapPayload(t *testing.T, session *mcp.ClientSession, scenePath string) testTilemapPayload {
	t.Helper()
	text := requireToolText(t, session, "read_tilemap", map[string]any{
		"scenePath": scenePath,
		"nodePath":  "root/Ground",
	})
	var payload testTilemapPayload
	if err := json.Unmarshal([]byte(text), &payload); err != nil {
		t.Fatalf("unmarshal tilemap payload: %v\n%s", err, text)
	}
	return payload
}

func setupTilemapScene(t *testing.T, session *mcp.ClientSession, scenePath, tileSetPath string) {
	t.Helper()
	requireToolText(t, session, "create_scene", map[string]any{"scenePath": scenePath, "rootNodeType": "Node2D"})
	requireToolText(t, session, "add_node", map[string]any{
		"scenePath": scenePath,
		"nodeType":  "TileMapLayer",
		"nodeName":  "Ground",
	})
	requireToolText(t, session, "modify_scene_node", map[string]any{
		"scenePath":  scenePath,
		"nodePath":   "root/Ground",
		"properties": map[string]any{"tile_set": "res://" + tileSetPath},
	})
}

func TestReadTilemapErrors(t *testing.T) {
	t.Parallel()
	root := newProject(t)
	session := connectProject(t, root)

	t.Run("missing node", func(t *testing.T) {
		requireToolText(t, session, "create_scene", map[string]any{"scenePath": "scenes/level_missing_node.tscn", "rootNodeType": "Node2D"})

		res := callTool(t, session, "read_tilemap", map[string]any{
			"scenePath": "scenes/level_missing_node.tscn",
			"nodePath":  "root/Missing",
		})
		if !res.IsError {
			t.Fatal("expected an error result")
		}
		requireContains(t, firstText(t, res), "manage_tilemap failed", "Node not found")
	})

	t.Run("node is not a TileMapLayer", func(t *testing.T) {
		requireToolText(t, session, "create_scene", map[string]any{"scenePath": "scenes/level_not_tilemap.tscn", "rootNodeType": "Node2D"})
		requireToolText(t, session, "add_node", map[string]any{
			"scenePath": "scenes/level_not_tilemap.tscn",
			"nodeType":  "Sprite2D",
			"nodeName":  "Art",
		})

		res := callTool(t, session, "read_tilemap", map[string]any{
			"scenePath": "scenes/level_not_tilemap.tscn",
			"nodePath":  "root/Art",
		})
		if !res.IsError {
			t.Fatal("expected an error result")
		}
		requireContains(t, firstText(t, res), "manage_tilemap failed", "Node is not a TileMapLayer")
	})

	t.Run("missing scene", func(t *testing.T) {
		requireToolError(t, session, "read_tilemap", map[string]any{
			"scenePath": "scenes/missing.tscn",
			"nodePath":  "root/Ground",
		}, "Scene file does not exist: scenes/missing.tscn")
	})
}
