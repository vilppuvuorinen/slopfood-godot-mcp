package tools

import (
	"strings"
	"testing"

	"github.com/vilppuvuorinen/slopfood-godot-mcp/internal/godot"
	"github.com/vilppuvuorinen/slopfood-godot-mcp/internal/mcp"
)

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
	if !strings.HasPrefix(result.Content[0].Text, "read_tileset failed: ") {
		t.Errorf("message = %q", result.Content[0].Text)
	}
	if len(result.Content) != 2 || !strings.HasPrefix(result.Content[1].Text, "Possible solutions:") {
		t.Errorf("solutions = %+v", result.Content)
	}
}
