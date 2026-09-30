package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/vilppuvuorinen/slopfood-godot-mcp/internal/mcp"
)

func TestResolveCategories(t *testing.T) {
	tests := []struct {
		name string
		raw  []string
		want []Category
	}{
		{name: "empty defaults to all", raw: nil, want: AllCategories},
		{name: "blank value defaults to all", raw: []string{""}, want: AllCategories},
		{name: "all expands", raw: []string{"all"}, want: AllCategories},
		{name: "comma separated", raw: []string{"scene,script"}, want: []Category{CategoryScene, CategoryScript}},
		{name: "assets category", raw: []string{"assets"}, want: []Category{CategoryAssets}},
		{name: "signals category", raw: []string{"signals"}, want: []Category{CategorySignals}},
		{name: "tilemap category", raw: []string{"tilemap"}, want: []Category{CategoryTilemap}},
		{name: "animation category", raw: []string{"animation"}, want: []Category{CategoryAnim}},
		{name: "tileset category", raw: []string{"tileset"}, want: []Category{CategoryTileset}},
		{name: "theme category", raw: []string{"theme"}, want: []Category{CategoryTheme}},
		{name: "repeated flags", raw: []string{"project", "node"}, want: []Category{CategoryProject, CategoryNode}},
		{name: "deduplicates preserving order", raw: []string{"node,project,node"}, want: []Category{CategoryNode, CategoryProject}},
		{name: "trims whitespace", raw: []string{" scene , node "}, want: []Category{CategoryScene, CategoryNode}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ResolveCategories(tt.raw)
			if err != nil {
				t.Fatalf("ResolveCategories(%v): %v", tt.raw, err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ResolveCategories(%v) = %v, want %v", tt.raw, got, tt.want)
			}
		})
	}
}

func TestResolveCategoriesUnknown(t *testing.T) {
	// "resources" was renamed to "assets"; it is no longer a valid name.
	for _, raw := range [][]string{{"bogus"}, {"all,bogus"}, {"project,nope"}, {"resources"}} {
		if _, err := ResolveCategories(raw); err == nil {
			t.Errorf("ResolveCategories(%v) = nil error, want an error", raw)
		}
	}
}

func registeredToolNames(t *testing.T, categories ...Category) map[string]bool {
	t.Helper()
	server := mcp.NewServer(mcp.ServerInfo{Name: "test"}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	Register(server, "", categories...)

	in := strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"tools/list"}` + "\n")
	var out bytes.Buffer
	if err := server.Serve(context.Background(), in, &out); err != nil {
		t.Fatalf("serve: %v", err)
	}
	var resp struct {
		Result struct {
			Tools []struct {
				Name string `json:"name"`
			} `json:"tools"`
		} `json:"result"`
	}
	if err := json.Unmarshal(out.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal %q: %v", out.String(), err)
	}
	names := make(map[string]bool, len(resp.Result.Tools))
	for _, tool := range resp.Result.Tools {
		names[tool.Name] = true
	}
	return names
}

func TestRegisterGating(t *testing.T) {
	projectOnly := registeredToolNames(t, CategoryProject)
	if !projectOnly["validate_project"] {
		t.Error("project category should register validate_project")
	}
	if projectOnly["create_scene"] {
		t.Error("project category should not register create_scene")
	}

	sceneOnly := registeredToolNames(t, CategoryScene)
	if !sceneOnly["create_scene"] {
		t.Error("scene category should register create_scene")
	}
	if sceneOnly["validate_project"] {
		t.Error("scene category should not register validate_project")
	}

	all := registeredToolNames(t, AllCategories...)
	if !all["validate_project"] || !all["create_scene"] {
		t.Errorf("all categories = %v", all)
	}
}

func TestCategoryToolsUnique(t *testing.T) {
	seen := make(map[string]bool)
	for _, category := range AllCategories {
		for _, newTool := range categoryTools[category] {
			tool := newTool()
			if seen[tool.Name] {
				t.Errorf("tool %q appears in more than one category", tool.Name)
			}
			seen[tool.Name] = true
		}
	}
	if len(seen) != 49 {
		t.Errorf("registered %d tools, want 49", len(seen))
	}
}

// schemaOptionalReads is the allowlist of self-describing reads with no output schema.
var schemaOptionalReads = map[string]bool{
	"read_animation":       true,
	"read_binary_resource": true,
	"read_scene":           true,
	"read_theme":           true,
	"read_tilemap":         true,
	"read_tileset":         true,
	"validate_scripts":     true,
}

func TestToolsAdvertiseMetadata(t *testing.T) {
	for _, category := range AllCategories {
		for _, newTool := range categoryTools[category] {
			tool := newTool()
			if tool.Title == "" {
				t.Errorf("tool %q has no title", tool.Name)
			}
			if tool.Annotations == nil {
				t.Errorf("tool %q has no annotations", tool.Name)
			}
			if tool.OutputSchema == nil && !schemaOptionalReads[tool.Name] {
				t.Errorf("tool %q has no output schema", tool.Name)
			}
		}
	}
}

func TestToolAnnotationsMatchBehavior(t *testing.T) {
	readOnly := map[string]bool{
		"get_godot_version":    true,
		"read_animation":       true,
		"read_binary_resource": true,
		"read_scene":           true,
		"read_tilemap":         true,
		"read_tileset":         true,
		"read_theme":           true,
		"validate_project":     true,
		"validate_scripts":     true,
	}
	nonDestructive := map[string]bool{
		"create_project":   true,
		"reimport_project": true,
	}
	idempotent := map[string]bool{
		"add_scene_signal":           true,
		"attach_script":              true,
		"clear_theme_item":           true,
		"clear_theme_type_variation": true,
		"clear_tilemap":              true,
		"create_animation":           true,
		"create_project":             true,
		"create_resource":            true,
		"create_scene":               true,
		"create_theme":               true,
		"create_tileset":             true,
		"erase_tilemap_cell":         true,
		"export_mesh_library":        true,
		"export_project":             true,
		"modify_animation":           true,
		"modify_resource":            true,
		"modify_scene_node":          true,
		"modify_theme":               true,
		"modify_tileset":             true,
		"move_scene_node":            true,
		"reimport_project":           true,
		"remove_animation_track":     true,
		"remove_scene_node":          true,
		"remove_scene_signal":        true,
		"remove_theme_type":          true,
		"remove_tileset_alternative": true,
		"remove_tileset_source":      true,
		"remove_tileset_tile":        true,
		"rename_scene_node":          true,
		"set_animation_track":        true,
		"set_theme_item":             true,
		"set_theme_type_variation":   true,
		"set_tilemap_cell":           true,
		"set_tileset_tile_data":      true,
	}
	for _, category := range AllCategories {
		for _, newTool := range categoryTools[category] {
			tool := newTool()
			if tool.Annotations == nil {
				t.Errorf("tool %q has no annotations", tool.Name)
				continue
			}
			if readOnly[tool.Name] {
				if tool.Annotations.ReadOnlyHint == nil || !*tool.Annotations.ReadOnlyHint {
					t.Errorf("read-only tool %q does not advertise readOnlyHint", tool.Name)
				}
				if tool.Annotations.DestructiveHint != nil && *tool.Annotations.DestructiveHint {
					t.Errorf("read-only tool %q advertises destructiveHint", tool.Name)
				}
				continue
			}
			if tool.Annotations.ReadOnlyHint != nil && *tool.Annotations.ReadOnlyHint {
				t.Errorf("writing tool %q advertises readOnlyHint", tool.Name)
			}
			if tool.Annotations.DestructiveHint == nil {
				t.Errorf("writing tool %q does not declare destructiveHint", tool.Name)
			} else if want := !nonDestructive[tool.Name]; *tool.Annotations.DestructiveHint != want {
				t.Errorf("writing tool %q destructiveHint = %t, want %t", tool.Name, *tool.Annotations.DestructiveHint, want)
			}
			if tool.Annotations.IdempotentHint == nil {
				t.Errorf("writing tool %q does not declare idempotentHint", tool.Name)
			} else if want := idempotent[tool.Name]; *tool.Annotations.IdempotentHint != want {
				t.Errorf("writing tool %q idempotentHint = %t, want %t", tool.Name, *tool.Annotations.IdempotentHint, want)
			}
		}
	}
}

var schemaKeywords = []string{
	"type", "anyOf", "oneOf", "allOf", "not", "enum", "const",
	"$ref", "properties", "additionalProperties", "items", "required",
}

func TestToolsAdvertiseValueTypes(t *testing.T) {
	for _, category := range AllCategories {
		for _, newTool := range categoryTools[category] {
			tool := newTool()
			assertTypedSchemas(t, tool.Name, "inputSchema", tool.InputSchema)
			assertTypedSchemas(t, tool.Name, "outputSchema", tool.OutputSchema)
		}
	}
}

func assertTypedSchemas(t *testing.T, toolName, path string, node any) {
	t.Helper()
	switch n := node.(type) {
	case map[string]any:
		typed := false
		for _, keyword := range schemaKeywords {
			if _, ok := n[keyword]; ok {
				typed = true
				break
			}
		}
		if !typed {
			if _, ok := n["description"]; ok {
				t.Errorf("tool %q %s is a bare description: %v", toolName, path, n)
			}
			return
		}
		for key, value := range n {
			assertTypedSchemas(t, toolName, path+"."+key, value)
		}
	case []any:
		for i, value := range n {
			assertTypedSchemas(t, toolName, path+"["+strconv.Itoa(i)+"]", value)
		}
	}
}

func TestToolsConstrainAdvertisedStatus(t *testing.T) {
	for _, category := range AllCategories {
		for _, newTool := range categoryTools[category] {
			tool := newTool()
			props, _ := tool.OutputSchema["properties"].(map[string]any)
			if _, ok := props["success"]; !ok {
				continue
			}

			branches, hasBranches := tool.OutputSchema["anyOf"].([]any)
			if !hasBranches {
				if !hasRequired(tool.OutputSchema, "success", "message") {
					t.Errorf("tool %q advertises success without requiring it: %v", tool.Name, tool.OutputSchema)
				}
				continue
			}

			statusBranch, payloadBranch := false, false
			for _, branch := range branches {
				schema, ok := branch.(map[string]any)
				if !ok {
					t.Errorf("tool %q anyOf branch = %T", tool.Name, branch)
					continue
				}
				if hasRequired(schema, "success", "message") {
					statusBranch = true
				} else if !hasRequired(schema, "success") {
					payloadBranch = true
				}
			}
			if !statusBranch || !payloadBranch {
				t.Errorf("tool %q anyOf must separate the status and payload shapes: %v", tool.Name, branches)
			}
		}
	}
}
