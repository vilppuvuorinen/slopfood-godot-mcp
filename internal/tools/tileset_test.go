package tools

import (
	"strings"
	"testing"

	"github.com/vilppuvuorinen/slopfood-godot-mcp/internal/godot"
	"github.com/vilppuvuorinen/slopfood-godot-mcp/internal/mcp"
)

func TestCreateTilesetSchema(t *testing.T) {
	tool := createTileset()
	if tool.Name != "create_tileset" {
		t.Errorf("name = %q", tool.Name)
	}
	if tool.Description != "Create a TileSet resource" {
		t.Errorf("description = %q", tool.Description)
	}
	if !hasRequired(tool.InputSchema, "resourcePath") {
		t.Errorf("required = %v", tool.InputSchema["required"])
	}
	props, ok := tool.InputSchema["properties"].(map[string]any)
	if !ok {
		t.Fatalf("properties = %T", tool.InputSchema["properties"])
	}
	wantTypes := map[string]string{
		"resourcePath":   "string",
		"tileShape":      "string",
		"tileLayout":     "string",
		"tileOffsetAxis": "string",
		"uvClipping":     "boolean",
	}
	for key, want := range wantTypes {
		prop, propOK := props[key].(map[string]any)
		if !propOK || prop["type"] != want {
			t.Errorf("property %q = %v, want %s", key, props[key], want)
		}
	}
	if prop, propOK := props["tileSize"].(map[string]any); !propOK || prop["anyOf"] == nil {
		t.Errorf("tileSize = %v, want a value-contract schema", props["tileSize"])
	}
	tileShape, _ := props["tileShape"].(map[string]any)
	enum, _ := tileShape["enum"].([]string)
	if strings.Join(enum, ",") != "square,isometric,half_offset_square,hexagon" {
		t.Errorf("tileShape enum = %v", tileShape["enum"])
	}
	tileLayout, _ := props["tileLayout"].(map[string]any)
	enum, _ = tileLayout["enum"].([]string)
	if strings.Join(enum, ",") != "stacked,stacked_offset,stairs_right,stairs_down,diamond_right,diamond_down" {
		t.Errorf("tileLayout enum = %v", tileLayout["enum"])
	}
	tileOffsetAxis, _ := props["tileOffsetAxis"].(map[string]any)
	enum, _ = tileOffsetAxis["enum"].([]string)
	if strings.Join(enum, ",") != "horizontal,vertical" {
		t.Errorf("tileOffsetAxis enum = %v", tileOffsetAxis["enum"])
	}
}

func TestModifyTilesetSchema(t *testing.T) {
	tool := modifyTileset()
	if tool.Name != "modify_tileset" {
		t.Errorf("name = %q", tool.Name)
	}
	if tool.Description != "Set a TileSet resource's tile size, shape, layout, and UV clipping" {
		t.Errorf("description = %q", tool.Description)
	}
	if !hasRequired(tool.InputSchema, "resourcePath") {
		t.Errorf("required = %v", tool.InputSchema["required"])
	}
	props, ok := tool.InputSchema["properties"].(map[string]any)
	if !ok {
		t.Fatalf("properties = %T", tool.InputSchema["properties"])
	}
	for _, key := range []string{"resourcePath", "tileSize", "tileShape", "tileLayout", "tileOffsetAxis", "uvClipping"} {
		if _, propOK := props[key].(map[string]any); !propOK {
			t.Errorf("missing property %q", key)
		}
	}
}

func TestAddTilesetSourceSchema(t *testing.T) {
	tool := addTilesetSource()
	if tool.Name != "add_tileset_source" {
		t.Errorf("name = %q", tool.Name)
	}
	if tool.Description != "Add an atlas or scenes-collection source to a TileSet" {
		t.Errorf("description = %q", tool.Description)
	}
	if !hasRequired(tool.InputSchema, "resourcePath", "sourceType") {
		t.Errorf("required = %v", tool.InputSchema["required"])
	}
	props, ok := tool.InputSchema["properties"].(map[string]any)
	if !ok {
		t.Fatalf("properties = %T", tool.InputSchema["properties"])
	}
	wantTypes := map[string]string{
		"resourcePath":      "string",
		"sourceType":        "string",
		"sourceId":          "integer",
		"useTexturePadding": "boolean",
	}
	for key, want := range wantTypes {
		prop, propOK := props[key].(map[string]any)
		if !propOK || prop["type"] != want {
			t.Errorf("property %q = %v, want %s", key, props[key], want)
		}
	}
	sourceType, _ := props["sourceType"].(map[string]any)
	enum, _ := sourceType["enum"].([]string)
	if strings.Join(enum, ",") != "atlas,scenes" {
		t.Errorf("sourceType enum = %v", sourceType["enum"])
	}
	for _, key := range []string{"texture", "margins", "separation", "textureRegionSize"} {
		prop, propOK := props[key].(map[string]any)
		if !propOK || prop["anyOf"] == nil {
			t.Errorf("property %q = %v, want a value-contract schema", key, props[key])
		}
	}
}

func TestRemoveTilesetSourceSchema(t *testing.T) {
	tool := removeTilesetSource()
	if tool.Name != "remove_tileset_source" {
		t.Errorf("name = %q", tool.Name)
	}
	if tool.Description != "Remove a source from a TileSet" {
		t.Errorf("description = %q", tool.Description)
	}
	if !hasRequired(tool.InputSchema, "resourcePath", "sourceId") {
		t.Errorf("required = %v", tool.InputSchema["required"])
	}
	props, ok := tool.InputSchema["properties"].(map[string]any)
	if !ok {
		t.Fatalf("properties = %T", tool.InputSchema["properties"])
	}
	prop, propOK := props["sourceId"].(map[string]any)
	if !propOK || prop["type"] != "integer" {
		t.Errorf("sourceId = %v, want integer", props["sourceId"])
	}
}

func TestAddTilesetTileSchema(t *testing.T) {
	tool := addTilesetTile()
	if tool.Name != "add_tileset_tile" {
		t.Errorf("name = %q", tool.Name)
	}
	if tool.Description != "Add a tile to a TileSet source" {
		t.Errorf("description = %q", tool.Description)
	}
	if !hasRequired(tool.InputSchema, "resourcePath", "sourceId") {
		t.Errorf("required = %v", tool.InputSchema["required"])
	}
	props, ok := tool.InputSchema["properties"].(map[string]any)
	if !ok {
		t.Fatalf("properties = %T", tool.InputSchema["properties"])
	}
	wantTypes := map[string]string{
		"resourcePath":       "string",
		"sourceId":           "integer",
		"scenePath":          "string",
		"sceneTileId":        "integer",
		"displayPlaceholder": "boolean",
	}
	for key, want := range wantTypes {
		prop, propOK := props[key].(map[string]any)
		if !propOK || prop["type"] != want {
			t.Errorf("property %q = %v, want %s", key, props[key], want)
		}
	}
	for _, key := range []string{"atlasCoords", "sizeInAtlas"} {
		prop, propOK := props[key].(map[string]any)
		if !propOK || prop["anyOf"] == nil {
			t.Errorf("property %q = %v, want a value-contract schema", key, props[key])
		}
	}
}

func TestRemoveTilesetTileSchema(t *testing.T) {
	tool := removeTilesetTile()
	if tool.Name != "remove_tileset_tile" {
		t.Errorf("name = %q", tool.Name)
	}
	if tool.Description != "Remove a tile from a TileSet source" {
		t.Errorf("description = %q", tool.Description)
	}
	if !hasRequired(tool.InputSchema, "resourcePath", "sourceId") {
		t.Errorf("required = %v", tool.InputSchema["required"])
	}
	props, ok := tool.InputSchema["properties"].(map[string]any)
	if !ok {
		t.Fatalf("properties = %T", tool.InputSchema["properties"])
	}
	if prop, propOK := props["atlasCoords"].(map[string]any); !propOK || prop["anyOf"] == nil {
		t.Errorf("atlasCoords = %v, want a value-contract schema", props["atlasCoords"])
	}
	prop, propOK := props["sceneTileId"].(map[string]any)
	if !propOK || prop["type"] != "integer" {
		t.Errorf("sceneTileId = %v, want integer", props["sceneTileId"])
	}
}

func TestAddTilesetAlternativeSchema(t *testing.T) {
	tool := addTilesetAlternative()
	if tool.Name != "add_tileset_alternative" {
		t.Errorf("name = %q", tool.Name)
	}
	if tool.Description != "Add an alternative tile to a TileSet atlas tile" {
		t.Errorf("description = %q", tool.Description)
	}
	if !hasRequired(tool.InputSchema, "resourcePath", "sourceId", "atlasCoords") {
		t.Errorf("required = %v", tool.InputSchema["required"])
	}
	props, ok := tool.InputSchema["properties"].(map[string]any)
	if !ok {
		t.Fatalf("properties = %T", tool.InputSchema["properties"])
	}
	prop, propOK := props["alternativeId"].(map[string]any)
	if !propOK || prop["type"] != "integer" {
		t.Errorf("alternativeId = %v, want integer", props["alternativeId"])
	}
}

func TestRemoveTilesetAlternativeSchema(t *testing.T) {
	tool := removeTilesetAlternative()
	if tool.Name != "remove_tileset_alternative" {
		t.Errorf("name = %q", tool.Name)
	}
	if tool.Description != "Remove an alternative tile from a TileSet atlas tile" {
		t.Errorf("description = %q", tool.Description)
	}
	if !hasRequired(tool.InputSchema, "resourcePath", "sourceId", "atlasCoords", "alternativeId") {
		t.Errorf("required = %v", tool.InputSchema["required"])
	}
}

func TestSetTilesetTileDataSchema(t *testing.T) {
	tool := setTilesetTileData()
	if tool.Name != "set_tileset_tile_data" {
		t.Errorf("name = %q", tool.Name)
	}
	if tool.Description != "Set properties on a TileSet atlas tile's data" {
		t.Errorf("description = %q", tool.Description)
	}
	if !hasRequired(tool.InputSchema, "resourcePath", "sourceId", "atlasCoords", "properties") {
		t.Errorf("required = %v", tool.InputSchema["required"])
	}
	props, ok := tool.InputSchema["properties"].(map[string]any)
	if !ok {
		t.Fatalf("properties = %T", tool.InputSchema["properties"])
	}
	properties, ok := props["properties"].(map[string]any)
	if !ok || properties["type"] != "object" {
		t.Errorf("properties property = %v, want object", props["properties"])
	}
	prop, propOK := props["alternativeId"].(map[string]any)
	if !propOK || prop["type"] != "integer" {
		t.Errorf("alternativeId = %v, want integer", props["alternativeId"])
	}
}

func TestReadTilesetSchema(t *testing.T) {
	tool := readTileset()
	if tool.Name != "read_tileset" {
		t.Errorf("name = %q", tool.Name)
	}
	if tool.Description != "Read TileSet resources and their sources and tiles" {
		t.Errorf("description = %q", tool.Description)
	}
	if !hasRequired(tool.InputSchema, "resourcePath") {
		t.Errorf("required = %v", tool.InputSchema["required"])
	}
}

func TestCreateTilesetForwardsProperties(t *testing.T) {
	t.Setenv(godot.PathEnv, fakeGodotOps(t, `echo "$@"`))
	root := t.TempDir()
	makeProject(t, root)

	result := callToolArgs(t, createTileset(), root, `{"resourcePath":"tiles/set.tres","tileSize":{"$type":"Vector2i","x":16,"y":16},"tileShape":"isometric","tileLayout":"diamond_down","tileOffsetAxis":"vertical","uvClipping":true}`)
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

func TestAddTilesetSourceForwardsFields(t *testing.T) {
	t.Setenv(godot.PathEnv, fakeGodotOps(t, `echo "$@"`))
	root := t.TempDir()
	makeProject(t, root)

	result := callToolArgs(t, addTilesetSource(), root, `{"resourcePath":"tiles/set.tres","sourceType":"atlas","sourceId":3,"texture":"tiles/atlas.png","margins":{"$type":"Vector2i","x":1,"y":1},"textureRegionSize":{"$type":"Vector2i","x":16,"y":16}}`)
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

func TestTilesetTileForwardsFields(t *testing.T) {
	t.Setenv(godot.PathEnv, fakeGodotOps(t, `echo "$@"`))
	root := t.TempDir()
	makeProject(t, root)

	add := callToolArgs(t, addTilesetTile(), root, `{"resourcePath":"tiles/set.tres","sourceId":0,"atlasCoords":{"$type":"Vector2i","x":2,"y":3},"sizeInAtlas":{"$type":"Vector2i","x":2,"y":1}}`)
	if add.IsError {
		t.Fatalf("unexpected add error: %+v", add.Content)
	}
	for _, want := range []string{`"action":"addTile"`, `"sourceId":0`, `"atlasCoords":{"$type":"Vector2i","x":2,"y":3}`, `"sizeInAtlas":{"$type":"Vector2i","x":2,"y":1}`} {
		if !strings.Contains(add.Content[0].Text, want) {
			t.Errorf("add text = %q, want it to contain %q", add.Content[0].Text, want)
		}
	}

	remove := callToolArgs(t, removeTilesetTile(), root, `{"resourcePath":"tiles/set.tres","sourceId":0,"atlasCoords":{"$type":"Vector2i","x":2,"y":3}}`)
	if remove.IsError {
		t.Fatalf("unexpected remove error: %+v", remove.Content)
	}
	for _, want := range []string{`"action":"removeTile"`, `"sourceId":0`, `"atlasCoords":{"$type":"Vector2i","x":2,"y":3}`} {
		if !strings.Contains(remove.Content[0].Text, want) {
			t.Errorf("remove text = %q, want it to contain %q", remove.Content[0].Text, want)
		}
	}
}

func TestTilesetAlternativeForwardsFields(t *testing.T) {
	t.Setenv(godot.PathEnv, fakeGodotOps(t, `echo "$@"`))
	root := t.TempDir()
	makeProject(t, root)

	add := callToolArgs(t, addTilesetAlternative(), root, `{"resourcePath":"tiles/set.tres","sourceId":0,"atlasCoords":{"$type":"Vector2i","x":2,"y":3},"alternativeId":1}`)
	if add.IsError {
		t.Fatalf("unexpected add error: %+v", add.Content)
	}
	for _, want := range []string{`"action":"addAlternative"`, `"sourceId":0`, `"atlasCoords":{"$type":"Vector2i","x":2,"y":3}`, `"alternativeId":1`} {
		if !strings.Contains(add.Content[0].Text, want) {
			t.Errorf("add text = %q, want it to contain %q", add.Content[0].Text, want)
		}
	}

	remove := callToolArgs(t, removeTilesetAlternative(), root, `{"resourcePath":"tiles/set.tres","sourceId":0,"atlasCoords":{"$type":"Vector2i","x":2,"y":3},"alternativeId":1}`)
	if remove.IsError {
		t.Fatalf("unexpected remove error: %+v", remove.Content)
	}
	for _, want := range []string{`"action":"removeAlternative"`, `"alternativeId":1`} {
		if !strings.Contains(remove.Content[0].Text, want) {
			t.Errorf("remove text = %q, want it to contain %q", remove.Content[0].Text, want)
		}
	}
}

func TestSetTilesetTileDataForwardsFields(t *testing.T) {
	t.Setenv(godot.PathEnv, fakeGodotOps(t, `echo "$@"`))
	root := t.TempDir()
	makeProject(t, root)

	result := callToolArgs(t, setTilesetTileData(), root, `{"resourcePath":"tiles/set.tres","sourceId":0,"atlasCoords":{"$type":"Vector2i","x":2,"y":3},"alternativeId":1,"properties":{"z_index":4}}`)
	if result.IsError {
		t.Fatalf("unexpected error: %+v", result.Content)
	}
	text := result.Content[0].Text
	for _, want := range []string{`"action":"setTileData"`, `"alternativeId":1`, `"z_index":4`} {
		if !strings.Contains(text, want) {
			t.Errorf("text = %q, want it to contain %q", text, want)
		}
	}
}

func TestReadTilesetStructured(t *testing.T) {
	t.Setenv(godot.PathEnv, fakeGodotOps(t, `printf '<<<GODOT_MCP_JSON_START>>>\n{"path":"tiles/set.tres","tileSize":{"$type":"Vector2i","x":16,"y":16},"tileShape":"square","tileLayout":"stacked","tileOffsetAxis":"horizontal","uvClipping":false,"sources":[]}\n<<<GODOT_MCP_JSON_END>>>\n'`))
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

func TestTilesetValidation(t *testing.T) {
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
		{name: "create missing resource path", tool: createTileset, args: `{}`, want: "resourcePath is required."},
		{name: "modify without properties", tool: modifyTileset, project: root, args: `{"resourcePath":"set.tres"}`, want: "The modify_tileset tool requires at least one of: tileSize, tileShape, tileLayout, tileOffsetAxis, uvClipping."},
		{name: "addSource missing source type", tool: addTilesetSource, project: root, args: `{"resourcePath":"set.tres"}`, want: "resourcePath and sourceType are required."},
		{name: "removeSource missing source id", tool: removeTilesetSource, project: root, args: `{"resourcePath":"set.tres"}`, want: "resourcePath and sourceId are required."},
		{name: "addTile missing source id", tool: addTilesetTile, project: root, args: `{"resourcePath":"set.tres"}`, want: "resourcePath and sourceId are required."},
		{name: "removeTile missing source id", tool: removeTilesetTile, project: root, args: `{"resourcePath":"set.tres"}`, want: "resourcePath and sourceId are required."},
		{name: "addAlternative missing atlas coords", tool: addTilesetAlternative, project: root, args: `{"resourcePath":"set.tres","sourceId":0}`, want: "resourcePath, sourceId, and atlasCoords are required."},
		{name: "removeAlternative missing alternative id", tool: removeTilesetAlternative, project: root, args: `{"resourcePath":"set.tres","sourceId":0,"atlasCoords":{"x":0,"y":0}}`, want: "resourcePath, sourceId, atlasCoords, and alternativeId are required."},
		{name: "setTileData missing properties", tool: setTilesetTileData, project: root, args: `{"resourcePath":"set.tres","sourceId":0,"atlasCoords":{"x":0,"y":0}}`, want: "resourcePath, sourceId, atlasCoords, and properties are required."},
		{name: "read missing resource path", tool: readTileset, project: root, args: `{}`, want: "resourcePath is required."},
		{name: "invalid project path", tool: readTileset, project: "../escape", args: `{"resourcePath":"set.tres"}`, want: "Invalid path."},
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

func TestTilesetScriptFailure(t *testing.T) {
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
