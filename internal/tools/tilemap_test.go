package tools

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/vilppuvuorinen/slopfood-godot-mcp/internal/godot"
)

func TestReadTilemapSchema(t *testing.T) {
	tool := readTilemap()
	if tool.Name != "read_tilemap" {
		t.Errorf("name = %q", tool.Name)
	}
	if !hasRequired(tool.InputSchema, "scenePath", "nodePath") {
		t.Errorf("required = %v", tool.InputSchema["required"])
	}
}

func tilemapProject(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	makeProject(t, root)
	seedFile(t, filepath.Join(root, "scenes", "level.tscn"), "[gd_scene format=3]\n")
	return root
}

func TestReadTilemapStructured(t *testing.T) {
	t.Setenv(godot.PathEnv, fakeGodotOps(t, `printf '<<<GODOT_MCP_JSON_START>>>\n{"scenePath":"scenes/level.tscn","nodePath":"root/Ground","tileSet":"tiles/set.tres","usedCells":[{"coords":{"$type":"Vector2i","x":0,"y":0},"sourceId":0,"atlasCoords":{"$type":"Vector2i","x":1,"y":1},"alternativeTile":0,"flippedH":false,"flippedV":false,"transposed":false}]}\n<<<GODOT_MCP_JSON_END>>>\n'`))
	root := tilemapProject(t)

	result := callToolArgs(t, readTilemap(), root, `{"scenePath":"scenes/level.tscn","nodePath":"root/Ground"}`)
	if result.IsError {
		t.Fatalf("unexpected error: %+v", result.Content)
	}
	text := result.Content[0].Text
	for _, want := range []string{`"tileSet": "tiles/set.tres"`, `"usedCells": [`, `"sourceId": 0`} {
		if !strings.Contains(text, want) {
			t.Errorf("text = %q, want it to contain %q", text, want)
		}
	}
}

func TestReadTilemapScriptFailure(t *testing.T) {
	t.Setenv(godot.PathEnv, fakeGodotOps(t, `echo 'Node is not a TileMapLayer: root/Ground' >&2; exit 1`))
	root := tilemapProject(t)

	result := callToolArgs(t, readTilemap(), root, `{"scenePath":"scenes/level.tscn","nodePath":"root/Ground"}`)
	if !result.IsError {
		t.Fatalf("expected an error result: %+v", result.Content)
	}
	if !strings.HasPrefix(result.Content[0].Text, "read_tilemap failed: ") {
		t.Errorf("message = %q", result.Content[0].Text)
	}
}
