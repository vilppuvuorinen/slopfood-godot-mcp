package tools

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/vilppuvuorinen/go-dot-mcp/internal/godot"
	"github.com/vilppuvuorinen/go-dot-mcp/internal/mcp"
)

func TestSetTilemapCellSchema(t *testing.T) {
	tool := setTilemapCell()
	if tool.Name != "set_tilemap_cell" {
		t.Errorf("name = %q", tool.Name)
	}
	if tool.Description != "Place a tile on a TileMapLayer node in a scene" {
		t.Errorf("description = %q", tool.Description)
	}
	if !hasRequired(tool.InputSchema, "scenePath", "nodePath", "coords", "sourceId", "atlasCoords") {
		t.Errorf("required = %v", tool.InputSchema["required"])
	}
	props, ok := tool.InputSchema["properties"].(map[string]any)
	if !ok {
		t.Fatalf("properties = %T", tool.InputSchema["properties"])
	}
	for key, want := range map[string]string{"scenePath": "string", "nodePath": "string", "sourceId": "integer", "alternativeTile": "integer"} {
		prop, propOK := props[key].(map[string]any)
		if !propOK || prop["type"] != want {
			t.Errorf("property %q = %v, want %s", key, props[key], want)
		}
	}
	for _, key := range []string{"coords", "atlasCoords"} {
		prop, propOK := props[key].(map[string]any)
		if !propOK || prop["anyOf"] == nil {
			t.Errorf("property %q = %v, want a value-contract schema", key, props[key])
		}
	}
}

func TestEraseTilemapCellSchema(t *testing.T) {
	tool := eraseTilemapCell()
	if tool.Name != "erase_tilemap_cell" {
		t.Errorf("name = %q", tool.Name)
	}
	if !hasRequired(tool.InputSchema, "scenePath", "nodePath", "coords") {
		t.Errorf("required = %v", tool.InputSchema["required"])
	}
}

func TestClearTilemapSchema(t *testing.T) {
	tool := clearTilemap()
	if tool.Name != "clear_tilemap" {
		t.Errorf("name = %q", tool.Name)
	}
	if !hasRequired(tool.InputSchema, "scenePath", "nodePath") {
		t.Errorf("required = %v", tool.InputSchema["required"])
	}
}

func TestReadTilemapSchema(t *testing.T) {
	tool := readTilemap()
	if tool.Name != "read_tilemap" {
		t.Errorf("name = %q", tool.Name)
	}
	if !hasRequired(tool.InputSchema, "scenePath", "nodePath") {
		t.Errorf("required = %v", tool.InputSchema["required"])
	}
}

// tilemapProject creates a project with a scene the tilemap tools can load.
func tilemapProject(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	makeProject(t, root)
	seedFile(t, filepath.Join(root, "scenes", "level.tscn"), "[gd_scene format=3]\n")
	return root
}

func TestSetTilemapCellForwardsFields(t *testing.T) {
	t.Setenv(godot.PathEnv, fakeGodotOps(t, `echo "$@"`))
	root := tilemapProject(t)

	result := callToolArgs(t, setTilemapCell(), root, `{"scenePath":"scenes/level.tscn","nodePath":"root/Ground","coords":{"$type":"Vector2i","x":1,"y":2},"sourceId":0,"atlasCoords":{"$type":"Vector2i","x":3,"y":4},"alternativeTile":2}`)
	if result.IsError {
		t.Fatalf("unexpected error: %+v", result.Content)
	}
	text := result.Content[0].Text
	for _, want := range []string{`"action":"setCell"`, `"nodePath":"root/Ground"`, `"coords":{"$type":"Vector2i","x":1,"y":2}`, `"sourceId":0`, `"atlasCoords":{"$type":"Vector2i","x":3,"y":4}`, `"alternativeTile":2`} {
		if !strings.Contains(text, want) {
			t.Errorf("text = %q, want it to contain %q", text, want)
		}
	}
}

func TestEraseTilemapCellForwardsCoords(t *testing.T) {
	t.Setenv(godot.PathEnv, fakeGodotOps(t, `echo "$@"`))
	root := tilemapProject(t)

	result := callToolArgs(t, eraseTilemapCell(), root, `{"scenePath":"scenes/level.tscn","nodePath":"root/Ground","coords":{"$type":"Vector2i","x":0,"y":0}}`)
	if result.IsError {
		t.Fatalf("unexpected error: %+v", result.Content)
	}
	text := result.Content[0].Text
	for _, want := range []string{`"action":"eraseCell"`, `"coords":{"$type":"Vector2i","x":0,"y":0}`} {
		if !strings.Contains(text, want) {
			t.Errorf("text = %q, want it to contain %q", text, want)
		}
	}
}

func TestClearTilemapForwardsAction(t *testing.T) {
	t.Setenv(godot.PathEnv, fakeGodotOps(t, `echo "$@"`))
	root := tilemapProject(t)

	result := callToolArgs(t, clearTilemap(), root, `{"scenePath":"scenes/level.tscn","nodePath":"root/Ground"}`)
	if result.IsError {
		t.Fatalf("unexpected error: %+v", result.Content)
	}
	if !strings.Contains(result.Content[0].Text, `"action":"clear"`) {
		t.Errorf("text = %q, want action forwarded", result.Content[0].Text)
	}
}

func TestReadTilemapStructured(t *testing.T) {
	t.Setenv(godot.PathEnv, fakeGodotOps(t, `printf '<<<GO_DOT_MCP_JSON_START>>>\n{"scenePath":"scenes/level.tscn","nodePath":"root/Ground","tileSet":"tiles/set.tres","usedCells":[{"coords":{"$type":"Vector2i","x":0,"y":0},"sourceId":0,"atlasCoords":{"$type":"Vector2i","x":1,"y":1},"alternativeTile":0,"flippedH":false,"flippedV":false,"transposed":false}]}\n<<<GO_DOT_MCP_JSON_END>>>\n'`))
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

func TestTilemapMutationsValidation(t *testing.T) {
	root := tilemapProject(t)

	tests := []struct {
		name string
		tool func() mcp.Tool
		args string
		want string
	}{
		{
			name: "setCell missing node fields",
			tool: setTilemapCell,
			args: `{}`,
			want: "scenePath and nodePath are required.",
		},
		{
			name: "setCell missing coords",
			tool: setTilemapCell,
			args: `{"scenePath":"scenes/level.tscn","nodePath":"root/Ground"}`,
			want: "coords, sourceId, and atlasCoords are required.",
		},
		{
			name: "setCell missing sourceId",
			tool: setTilemapCell,
			args: `{"scenePath":"scenes/level.tscn","nodePath":"root/Ground","coords":{"x":0,"y":0}}`,
			want: "coords, sourceId, and atlasCoords are required.",
		},
		{
			name: "setCell missing atlasCoords",
			tool: setTilemapCell,
			args: `{"scenePath":"scenes/level.tscn","nodePath":"root/Ground","coords":{"x":0,"y":0},"sourceId":0}`,
			want: "coords, sourceId, and atlasCoords are required.",
		},
		{
			name: "eraseCell missing coords",
			tool: eraseTilemapCell,
			args: `{"scenePath":"scenes/level.tscn","nodePath":"root/Ground"}`,
			want: "coords is required.",
		},
		{
			name: "clear missing nodePath",
			tool: clearTilemap,
			args: `{"scenePath":"scenes/level.tscn"}`,
			want: "scenePath and nodePath are required.",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := callToolArgs(t, tt.tool(), root, tt.args)
			if !result.IsError {
				t.Fatalf("expected an error result: %+v", result.Content)
			}
			if !strings.HasPrefix(result.Content[0].Text, tt.want) {
				t.Errorf("message = %q, want prefix %q", result.Content[0].Text, tt.want)
			}
		})
	}
}

func TestReadTilemapScriptFailure(t *testing.T) {
	t.Setenv(godot.PathEnv, fakeGodotOps(t, `echo 'Node is not a TileMapLayer: root/Ground' >&2; exit 1`))
	root := tilemapProject(t)

	result := callToolArgs(t, readTilemap(), root, `{"scenePath":"scenes/level.tscn","nodePath":"root/Ground"}`)
	if !result.IsError {
		t.Fatalf("expected an error result: %+v", result.Content)
	}
	if !strings.HasPrefix(result.Content[0].Text, "manage_tilemap failed: ") {
		t.Errorf("message = %q", result.Content[0].Text)
	}
}
