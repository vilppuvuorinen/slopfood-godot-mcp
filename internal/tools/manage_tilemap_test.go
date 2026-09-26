package tools

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/vilppuvuorinen/go-dot-mcp/internal/godot"
	"github.com/vilppuvuorinen/go-dot-mcp/internal/mcp"
)

func TestManageTilemapSchema(t *testing.T) {
	tool := manageTilemap()
	if tool.Name != "manage_tilemap" {
		t.Errorf("name = %q", tool.Name)
	}
	required, ok := tool.InputSchema["required"].([]string)
	if !ok || strings.Join(required, ",") != "scenePath,nodePath,action" {
		t.Errorf("required = %v", tool.InputSchema["required"])
	}
	props, ok := tool.InputSchema["properties"].(map[string]any)
	if !ok {
		t.Fatalf("properties = %T", tool.InputSchema["properties"])
	}
	wantTypes := map[string]string{
		"scenePath":       "string",
		"nodePath":        "string",
		"action":          "string",
		"sourceId":        "integer",
		"alternativeTile": "integer",
	}
	for key, want := range wantTypes {
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
	action, _ := props["action"].(map[string]any)
	enum, _ := action["enum"].([]string)
	if strings.Join(enum, ",") != "setCell,eraseCell,clear" {
		t.Errorf("action enum = %v", action["enum"])
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

func TestManageTilemapSetCellForwardsFields(t *testing.T) {
	t.Setenv(godot.PathEnv, fakeGodotOps(t, `echo "$@"`))
	root := tilemapProject(t)

	result := callToolArgs(t, manageTilemap(), root, `{"scenePath":"scenes/level.tscn","nodePath":"root/Ground","action":"setCell","coords":{"$type":"Vector2i","x":1,"y":2},"sourceId":0,"atlasCoords":{"$type":"Vector2i","x":3,"y":4},"alternativeTile":2}`)
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

func TestManageTilemapClearForwardsAction(t *testing.T) {
	t.Setenv(godot.PathEnv, fakeGodotOps(t, `echo "$@"`))
	root := tilemapProject(t)

	result := callToolArgs(t, manageTilemap(), root, `{"scenePath":"scenes/level.tscn","nodePath":"root/Ground","action":"clear"}`)
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

func TestManageTilemapValidation(t *testing.T) {
	root := tilemapProject(t)
	notAProject := t.TempDir()

	tests := []struct {
		name    string
		tool    func() mcp.Tool
		project string
		args    string
		want    string
	}{
		{name: "missing params", tool: manageTilemap, args: `{}`, want: "scenePath, nodePath, and action are required."},
		{name: "missing node path on read", tool: readTilemap, project: root, args: `{"scenePath":"scenes/level.tscn"}`, want: "scenePath and nodePath are required."},
		{name: "setCell without coords", tool: manageTilemap, project: root, args: `{"scenePath":"scenes/level.tscn","nodePath":"root/Ground","action":"setCell"}`, want: "coords is required for the setCell action."},
		{name: "setCell without sourceId", tool: manageTilemap, project: root, args: `{"scenePath":"scenes/level.tscn","nodePath":"root/Ground","action":"setCell","coords":{"x":0,"y":0}}`, want: "sourceId is required for the setCell action."},
		{name: "setCell without atlasCoords", tool: manageTilemap, project: root, args: `{"scenePath":"scenes/level.tscn","nodePath":"root/Ground","action":"setCell","coords":{"x":0,"y":0},"sourceId":0}`, want: "atlasCoords is required for the setCell action."},
		{name: "eraseCell without coords", tool: manageTilemap, project: root, args: `{"scenePath":"scenes/level.tscn","nodePath":"root/Ground","action":"eraseCell"}`, want: "coords is required for the eraseCell action."},
		{name: "unknown action", tool: manageTilemap, project: root, args: `{"scenePath":"scenes/level.tscn","nodePath":"root/Ground","action":"bogus"}`, want: "Unknown action: bogus."},
		{name: "invalid scene path", tool: readTilemap, project: root, args: `{"scenePath":"../escape.tscn","nodePath":"root/Ground"}`, want: "Invalid path."},
		{name: "missing scene", tool: readTilemap, project: root, args: `{"scenePath":"scenes/missing.tscn","nodePath":"root/Ground"}`, want: "Scene file does not exist"},
		{name: "not a project", tool: readTilemap, project: notAProject, args: `{"scenePath":"scenes/level.tscn","nodePath":"root/Ground"}`, want: "Not a valid Godot project"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := callToolArgs(t, tt.tool(), tt.project, tt.args)
			if !result.IsError {
				t.Fatalf("expected an error result: %+v", result.Content)
			}
			if !strings.HasPrefix(result.Content[0].Text, tt.want) {
				t.Errorf("message = %q, want prefix %q", result.Content[0].Text, tt.want)
			}
		})
	}
}

func TestManageTilemapScriptFailure(t *testing.T) {
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
