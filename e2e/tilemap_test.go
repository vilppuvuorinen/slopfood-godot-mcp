package e2e

import (
	"encoding/json"
	"fmt"
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

// requireRunScript runs a fixture script and fails the test unless it compiled
// cleanly and reported no engine errors.
func requireRunScript(t *testing.T, session *mcp.ClientSession, source string) runScriptDoc {
	t.Helper()
	doc := decodeRunScript(t, requireToolText(t, session, "run_script", map[string]any{"script": source}))
	if !doc.Compiled {
		t.Fatalf("run_script did not compile: %+v", doc.Errors)
	}
	if len(doc.Errors) != 0 {
		t.Fatalf("run_script errors: %+v", doc.Errors)
	}
	return doc
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

// A GradientTexture2D needs no import step, unlike a raw PNG.
func seedAtlasTexture(t *testing.T, root string) {
	t.Helper()
	writeE2EFile(t, filepath.Join(root, "tiles", "atlas.tres"),
		"[gd_resource type=\"GradientTexture2D\" format=3]\n\n[resource]\nwidth = 64\nheight = 64\n")
}

// setupTestTileSet builds a TileSet with one atlas source and tile via
// run_script; the tileset mutation tools it replaces were removed in favor of
// scripting. It returns the ResourceSaver error code (0 is OK).
func setupTestTileSet(t *testing.T, session *mcp.ClientSession, resourcePath string) {
	t.Helper()
	doc := requireRunScript(t, session, fmt.Sprintf(`extends RefCounted

func run():
	var tile_set := TileSet.new()
	tile_set.tile_size = Vector2i(16, 16)
	var source := TileSetAtlasSource.new()
	source.texture = load("res://tiles/atlas.tres")
	source.texture_region_size = Vector2i(16, 16)
	tile_set.add_source(source, 0)
	source.create_tile(Vector2i(0, 0))
	return ResourceSaver.save(tile_set, "res://%s")
`, resourcePath))
	if code, ok := doc.Result.(float64); !ok || code != 0 {
		t.Fatalf("save tileset %s: result = %v, want 0", resourcePath, doc.Result)
	}
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
