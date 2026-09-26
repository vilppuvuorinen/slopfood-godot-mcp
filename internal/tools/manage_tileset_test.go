package tools

import (
	"strings"
	"testing"

	"github.com/vilppuvuorinen/go-dot-mcp/internal/godot"
	"github.com/vilppuvuorinen/go-dot-mcp/internal/mcp"
)

func TestManageTilesetSchema(t *testing.T) {
	tool := manageTileset()
	if tool.Name != "manage_tileset" {
		t.Errorf("name = %q", tool.Name)
	}
	required, ok := tool.InputSchema["required"].([]string)
	if !ok || strings.Join(required, ",") != "resourcePath,action" {
		t.Errorf("required = %v", tool.InputSchema["required"])
	}
	props, ok := tool.InputSchema["properties"].(map[string]any)
	if !ok {
		t.Fatalf("properties = %T", tool.InputSchema["properties"])
	}
	wantTypes := map[string]string{
		"resourcePath":       "string",
		"action":             "string",
		"tileShape":          "string",
		"tileLayout":         "string",
		"tileOffsetAxis":     "string",
		"uvClipping":         "boolean",
		"sourceType":         "string",
		"sourceId":           "integer",
		"useTexturePadding":  "boolean",
		"scenePath":          "string",
		"sceneTileId":        "integer",
		"alternativeId":      "integer",
		"displayPlaceholder": "boolean",
	}
	for key, want := range wantTypes {
		prop, propOK := props[key].(map[string]any)
		if !propOK || prop["type"] != want {
			t.Errorf("property %q = %v, want %s", key, props[key], want)
		}
	}
	for _, key := range []string{"tileSize", "texture", "margins", "separation", "textureRegionSize", "atlasCoords", "sizeInAtlas"} {
		prop, propOK := props[key].(map[string]any)
		if !propOK || prop["anyOf"] == nil {
			t.Errorf("property %q = %v, want a value-contract schema", key, props[key])
		}
	}
	action, _ := props["action"].(map[string]any)
	enum, _ := action["enum"].([]string)
	if strings.Join(enum, ",") != "create,modify,addSource,removeSource,addTile,removeTile,addAlternative,removeAlternative,setTileData" {
		t.Errorf("action enum = %v", action["enum"])
	}
}

func TestReadTilesetSchema(t *testing.T) {
	tool := readTileset()
	if tool.Name != "read_tileset" {
		t.Errorf("name = %q", tool.Name)
	}
	if !hasRequired(tool.InputSchema, "resourcePath") {
		t.Errorf("required = %v", tool.InputSchema["required"])
	}
}

func TestManageTilesetCreateForwardsProperties(t *testing.T) {
	t.Setenv(godot.PathEnv, fakeGodotOps(t, `echo "$@"`))
	root := t.TempDir()
	makeProject(t, root)

	result := callToolArgs(t, manageTileset(), root, `{"resourcePath":"tiles/set.tres","action":"create","tileSize":{"$type":"Vector2i","x":16,"y":16},"tileShape":"isometric","tileLayout":"diamond_down","tileOffsetAxis":"vertical","uvClipping":true}`)
	if result.IsError {
		t.Fatalf("unexpected error: %+v", result.Content)
	}
	text := result.Content[0].Text
	for _, want := range []string{`"action":"create"`, `"tileSize":{"$type":"Vector2i","x":16,"y":16}`, `"tileShape":"isometric"`, `"tileLayout":"diamond_down"`, `"tileOffsetAxis":"vertical"`, `"uvClipping":true`} {
		if !strings.Contains(text, want) {
			t.Errorf("text = %q, want it to contain %q", text, want)
		}
	}
}

func TestManageTilesetAddSourceForwardsFields(t *testing.T) {
	t.Setenv(godot.PathEnv, fakeGodotOps(t, `echo "$@"`))
	root := t.TempDir()
	makeProject(t, root)

	result := callToolArgs(t, manageTileset(), root, `{"resourcePath":"tiles/set.tres","action":"addSource","sourceType":"atlas","sourceId":3,"texture":"tiles/atlas.png","margins":{"$type":"Vector2i","x":1,"y":1},"textureRegionSize":{"$type":"Vector2i","x":16,"y":16}}`)
	if result.IsError {
		t.Fatalf("unexpected error: %+v", result.Content)
	}
	text := result.Content[0].Text
	for _, want := range []string{`"action":"addSource"`, `"sourceType":"atlas"`, `"sourceId":3`, `"texture":"tiles/atlas.png"`, `"margins":{"$type":"Vector2i","x":1,"y":1}`} {
		if !strings.Contains(text, want) {
			t.Errorf("text = %q, want it to contain %q", text, want)
		}
	}
}

func TestManageTilesetAddTileAndDataForwardFields(t *testing.T) {
	t.Setenv(godot.PathEnv, fakeGodotOps(t, `echo "$@"`))
	root := t.TempDir()
	makeProject(t, root)

	add := callToolArgs(t, manageTileset(), root, `{"resourcePath":"tiles/set.tres","action":"addTile","sourceId":0,"atlasCoords":{"$type":"Vector2i","x":2,"y":3},"sizeInAtlas":{"$type":"Vector2i","x":2,"y":1}}`)
	if add.IsError {
		t.Fatalf("unexpected add error: %+v", add.Content)
	}
	for _, want := range []string{`"sourceId":0`, `"atlasCoords":{"$type":"Vector2i","x":2,"y":3}`, `"sizeInAtlas":{"$type":"Vector2i","x":2,"y":1}`} {
		if !strings.Contains(add.Content[0].Text, want) {
			t.Errorf("add text = %q, want it to contain %q", add.Content[0].Text, want)
		}
	}

	data := callToolArgs(t, manageTileset(), root, `{"resourcePath":"tiles/set.tres","action":"setTileData","sourceId":0,"atlasCoords":{"$type":"Vector2i","x":2,"y":3},"alternativeId":1,"properties":{"z_index":4}}`)
	if data.IsError {
		t.Fatalf("unexpected setTileData error: %+v", data.Content)
	}
	for _, want := range []string{`"alternativeId":1`, `"z_index":4`} {
		if !strings.Contains(data.Content[0].Text, want) {
			t.Errorf("setTileData text = %q, want it to contain %q", data.Content[0].Text, want)
		}
	}
}

func TestReadTilesetStructured(t *testing.T) {
	t.Setenv(godot.PathEnv, fakeGodotOps(t, `printf '<<<GO_DOT_MCP_JSON_START>>>\n{"path":"tiles/set.tres","tileSize":{"$type":"Vector2i","x":16,"y":16},"tileShape":"square","tileLayout":"stacked","tileOffsetAxis":"horizontal","uvClipping":false,"sources":[]}\n<<<GO_DOT_MCP_JSON_END>>>\n'`))
	root := t.TempDir()
	makeProject(t, root)

	result := callToolArgs(t, readTileset(), root, `{"resourcePath":"tiles/set.tres"}`)
	if result.IsError {
		t.Fatalf("unexpected error: %+v", result.Content)
	}
	text := result.Content[0].Text
	for _, want := range []string{`"path": "tiles/set.tres"`, `"tileShape": "square"`, `"sources": []`} {
		if !strings.Contains(text, want) {
			t.Errorf("text = %q, want it to contain %q", text, want)
		}
	}
	if strings.Contains(text, "succeeded.\n\nOutput:") {
		t.Errorf("text = %q, want the structured payload without the legacy wrapper", text)
	}
}

func TestManageTilesetValidation(t *testing.T) {
	root := t.TempDir()
	makeProject(t, root)
	notAProject := t.TempDir()

	tests := []struct {
		name    string
		tool    func() mcp.Tool
		project string
		args    string
		want    string
	}{
		{name: "missing params", tool: manageTileset, args: `{}`, want: "resourcePath and action are required."},
		{name: "missing resource path on read", tool: readTileset, project: root, args: `{}`, want: "resourcePath is required."},
		{name: "modify without properties", tool: manageTileset, project: root, args: `{"resourcePath":"set.tres","action":"modify"}`, want: "The modify action requires at least one of: tileSize, tileShape, tileLayout, tileOffsetAxis, uvClipping."},
		{name: "addSource without sourceType", tool: manageTileset, project: root, args: `{"resourcePath":"set.tres","action":"addSource"}`, want: "sourceType is required for the addSource action."},
		{name: "removeSource without sourceId", tool: manageTileset, project: root, args: `{"resourcePath":"set.tres","action":"removeSource"}`, want: "sourceId is required for the removeSource action."},
		{name: "addTile without sourceId", tool: manageTileset, project: root, args: `{"resourcePath":"set.tres","action":"addTile"}`, want: "sourceId is required for the addTile action."},
		{name: "addAlternative without atlasCoords", tool: manageTileset, project: root, args: `{"resourcePath":"set.tres","action":"addAlternative","sourceId":0}`, want: "atlasCoords is required for the addAlternative action."},
		{name: "removeAlternative without alternativeId", tool: manageTileset, project: root, args: `{"resourcePath":"set.tres","action":"removeAlternative","sourceId":0,"atlasCoords":{"x":0,"y":0}}`, want: "alternativeId is required for the removeAlternative action."},
		{name: "setTileData without atlasCoords", tool: manageTileset, project: root, args: `{"resourcePath":"set.tres","action":"setTileData","sourceId":0}`, want: "atlasCoords is required for the setTileData action."},
		{name: "unknown action", tool: manageTileset, project: root, args: `{"resourcePath":"set.tres","action":"bogus"}`, want: "Unknown action: bogus."},
		{name: "invalid resource path", tool: readTileset, project: root, args: `{"resourcePath":"../escape.tres"}`, want: "Invalid path."},
		{name: "not a project", tool: readTileset, project: notAProject, args: `{"resourcePath":"set.tres"}`, want: "Not a valid Godot project"},
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

func TestManageTilesetScriptFailure(t *testing.T) {
	t.Setenv(godot.PathEnv, fakeGodotOps(t, `echo 'TileSet resource not found: boom' >&2; exit 1`))
	root := t.TempDir()
	makeProject(t, root)

	result := callToolArgs(t, readTileset(), root, `{"resourcePath":"set.tres"}`)
	if !result.IsError {
		t.Fatalf("expected an error result: %+v", result.Content)
	}
	if !strings.HasPrefix(result.Content[0].Text, "manage_tileset failed: ") {
		t.Errorf("message = %q", result.Content[0].Text)
	}
	if len(result.Content) != 2 || !strings.HasPrefix(result.Content[1].Text, "Possible solutions:") {
		t.Errorf("solutions = %+v", result.Content)
	}
}
