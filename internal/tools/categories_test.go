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
		{name: "media category", raw: []string{"media"}, want: []Category{CategoryMedia}},
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

// registeredToolNames returns the tool names Register adds for the categories.
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
	if !projectOnly["get_project_info"] {
		t.Error("project category should register get_project_info")
	}
	if projectOnly["create_scene"] {
		t.Error("project category should not register create_scene")
	}

	sceneOnly := registeredToolNames(t, CategoryScene)
	if !sceneOnly["create_scene"] {
		t.Error("scene category should register create_scene")
	}
	if sceneOnly["get_project_info"] {
		t.Error("scene category should not register get_project_info")
	}

	all := registeredToolNames(t, AllCategories...)
	if !all["get_project_info"] || !all["create_scene"] {
		t.Errorf("all categories = %v", all)
	}
}

// TestCategoryToolsUnique verifies every constructor is reachable, no tool is
// registered twice, and the total matches the registered tool list.
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
	if len(seen) != 31 {
		t.Errorf("registered %d tools, want 31", len(seen))
	}
}

// TestCategoryBudget keeps each category small enough for a subagent to align
// on: a category is a coherent domain, so it stays at or below the top end of
// the 5-7 tool budget.
func TestCategoryBudget(t *testing.T) {
	for _, category := range AllCategories {
		if count := len(categoryTools[category]); count > 7 {
			t.Errorf("category %q has %d tools, want at most 7", category, count)
		}
	}
}

// schemaOptionalReads are read tools whose payload is self-describing, so they
// deliberately advertise no output schema: structured content plus text is more
// powerful token for token than a declared schema that can drift from what the
// engine emits. Every other tool must still advertise one.
var schemaOptionalReads = map[string]bool{
	"read_animation": true,
	"read_resource":  true,
	"read_scene":     true,
	"read_theme":     true,
	"read_tilemap":   true,
	"read_tileset":   true,
}

// TestToolsAdvertiseMetadata verifies every tool carries the metadata modern
// clients rely on: a title, behavior annotations, and (except for the
// self-describing reads) an output schema so structured results can be consumed
// programmatically.
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

// TestToolAnnotationsMatchBehavior pins every tool's behavior hints to a single
// stated rule, so a mislabeled or omitted hint cannot survive a change.
//
//   - Read-only tools advertise readOnlyHint and never claim to be destructive.
//   - A writer is destructive unless every effect is the creation of content
//     that cannot replace what is already there: create_project refuses an
//     existing project.godot and reimport_project only writes missing sidecars
//     and regenerated artifacts, so those two are additive; every other writer
//     can rewrite or delete existing content.
//   - A writer is idempotent when repeating the identical call leaves the
//     project unchanged: assignments, guarded inserts, overwrites, and
//     deletes. Tools whose actions mint new content each call (a node, a
//     duplicate, a track, a preset, a tileset source or tile) are not
//     idempotent.
//
// Every writer must declare both destructiveHint and idempotentHint, so an
// omission fails the test rather than silently falling back to a default.
func TestToolAnnotationsMatchBehavior(t *testing.T) {
	readOnly := map[string]bool{
		"get_godot_version": true,
		"get_project_info":  true,
		"read_animation":    true,
		"read_resource":     true,
		"read_scene":        true,
		"read_tilemap":      true,
		"read_tileset":      true,
		"read_theme":        true,
		"validate_project":  true,
		"validate_scripts":  true,
	}
	nonDestructive := map[string]bool{
		"create_project":   true,
		"reimport_project": true,
	}
	idempotent := map[string]bool{
		"add_scene_signal":    true,
		"attach_script":       true,
		"clear_tilemap":       true,
		"create_project":      true,
		"create_scene":        true,
		"erase_tilemap_cell":  true,
		"export_mesh_library": true,
		"export_project":      true,
		"reimport_project":    true,
		"manage_resource":     true,
		"manage_theme":        true,
		"modify_scene_node":   true,
		"move_scene_node":     true,
		"remove_scene_node":   true,
		"remove_scene_signal": true,
		"rename_scene_node":   true,
		"set_tilemap_cell":    true,
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

// TestToolsAdvertiseActionEnum verifies every tool whose schema has an "action"
// property advertises the valid actions as an enum, so clients can discover
// them without out-of-band documentation.
func TestToolsAdvertiseActionEnum(t *testing.T) {
	for _, category := range AllCategories {
		for _, newTool := range categoryTools[category] {
			tool := newTool()
			props, ok := tool.InputSchema["properties"].(map[string]any)
			if !ok {
				t.Errorf("tool %q has no properties", tool.Name)
				continue
			}
			action, ok := props["action"]
			if !ok {
				continue
			}
			actionProp, ok := action.(map[string]any)
			if !ok {
				t.Errorf("tool %q action property = %T", tool.Name, action)
				continue
			}
			enum, ok := actionProp["enum"].([]string)
			if !ok || len(enum) == 0 {
				t.Errorf("tool %q action has no enum: %v", tool.Name, actionProp)
			}
		}
	}
}

// schemaKeywords mark a map as a JSON Schema node. A node with none of them is
// a bare annotation, such as a value slot that only carries a description.
var schemaKeywords = []string{
	"type", "anyOf", "oneOf", "allOf", "not", "enum", "const",
	"$ref", "properties", "additionalProperties", "items", "required",
}

// TestToolsAdvertiseValueTypes verifies no tool describes a value slot with a
// bare description: every schema node names a type (or an equivalent keyword),
// so the tool value contract is visible to schema consumers.
func TestToolsAdvertiseValueTypes(t *testing.T) {
	for _, category := range AllCategories {
		for _, newTool := range categoryTools[category] {
			tool := newTool()
			assertTypedSchemas(t, tool.Name, "inputSchema", tool.InputSchema)
			assertTypedSchemas(t, tool.Name, "outputSchema", tool.OutputSchema)
		}
	}
}

// assertTypedSchemas walks a schema tree and fails on any map that carries a
// description without a schema keyword to constrain the value.
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

// TestActionInputSchemaBuildsVariants verifies the discriminated action schema
// pins each action and requires the common fields plus that action's own.
func TestActionInputSchemaBuildsVariants(t *testing.T) {
	schema := actionInputSchema(
		map[string]any{
			"action": map[string]any{"type": "string", "enum": []string{"list", "add"}},
			"path":   map[string]any{"type": "string"},
			"name":   map[string]any{"type": "string"},
			"value":  map[string]any{"type": "string"},
		},
		[]string{"path"},
		[]actionVariant{
			{Action: "list"},
			{Action: "add", Required: []string{"name"}, AnyOf: []string{"value"}},
		},
	)

	if !hasRequired(schema, "path") {
		t.Fatalf("common required = %v", schema["required"])
	}
	if !hasRequired(schema, "action") {
		t.Fatalf("common required = %v, want action", schema["required"])
	}
	branches, ok := schema["oneOf"].([]any)
	if !ok || len(branches) != 2 {
		t.Fatalf("oneOf = %#v, want two branches", schema["oneOf"])
	}

	list, _ := branches[0].(map[string]any)
	listProps, _ := list["properties"].(map[string]any)
	listAction, _ := listProps["action"].(map[string]any)
	if listAction["const"] != "list" {
		t.Errorf("branch 0 action = %#v, want const list", listProps["action"])
	}
	if !hasRequired(list, "path") || hasRequired(list, "name") {
		t.Errorf("list required = %v, want path only", list["required"])
	}

	add, _ := branches[1].(map[string]any)
	addProps, _ := add["properties"].(map[string]any)
	addAction, _ := addProps["action"].(map[string]any)
	if addAction["const"] != "add" {
		t.Errorf("branch 1 action = %#v, want const add", addProps["action"])
	}
	if !hasRequired(add, "path", "name") {
		t.Errorf("add required = %v, want path and name", add["required"])
	}
	addAnyOf, ok := add["anyOf"].([]any)
	if !ok || len(addAnyOf) != 1 {
		t.Fatalf("add anyOf = %#v, want one branch", add["anyOf"])
	}
	valueBranch, _ := addAnyOf[0].(map[string]any)
	if !hasRequired(valueBranch, "value") {
		t.Errorf("add anyOf branch = %v, want value", addAnyOf[0])
	}
}

// TestActionToolsAdvertiseVariants verifies every action-routed tool constrains
// each action's required fields with a discriminated oneOf, so the schema
// reveals the per-action contract instead of only the common fields. anyOf
// lists the effectful fields an action must name at least one of, so a modify
// cannot be advertised as a no-op.
func TestActionToolsAdvertiseVariants(t *testing.T) {
	want := map[string]struct {
		common  []string
		actions map[string][]string
		anyOf   map[string][]string
	}{
		"manage_resource": {
			common: []string{"resourcePath", "action"},
			actions: map[string][]string{
				"create": {"resourceType"},
				"modify": {"properties"},
			},
		},
		"manage_animation": {
			common: []string{"resourcePath", "action"},
			actions: map[string][]string{
				"create": {},
				"modify": {},
				"add":    {"trackType"},
				"set":    {"trackIndex"},
				"remove": {"trackIndex"},
			},
			anyOf: map[string][]string{
				"modify": {"length", "loopMode", "step"},
			},
		},
		"manage_tileset": {
			common: []string{"resourcePath", "action"},
			actions: map[string][]string{
				"create":            {},
				"modify":            {},
				"addSource":         {"sourceType"},
				"removeSource":      {"sourceId"},
				"addTile":           {"sourceId"},
				"removeTile":        {"sourceId"},
				"addAlternative":    {"sourceId", "atlasCoords"},
				"removeAlternative": {"sourceId", "atlasCoords", "alternativeId"},
				"setTileData":       {"sourceId", "atlasCoords"},
			},
			anyOf: map[string][]string{
				"modify": {"tileSize", "tileShape", "tileLayout", "tileOffsetAxis", "uvClipping"},
			},
		},
		"manage_theme": {
			common: []string{"resourcePath", "action"},
			actions: map[string][]string{
				"create":             {},
				"modify":             {},
				"removeType":         {"themeType"},
				"setTypeVariation":   {"themeType", "baseType"},
				"clearTypeVariation": {"themeType"},
				"setItem":            {"themeType", "itemType", "itemName", "value"},
				"clearItem":          {"themeType", "itemType", "itemName"},
			},
			anyOf: map[string][]string{
				"modify": {"defaultFont", "defaultFontSize", "defaultBaseScale"},
			},
		},
	}

	seen := make(map[string]bool)
	for _, category := range AllCategories {
		for _, newTool := range categoryTools[category] {
			tool := newTool()
			spec, ok := want[tool.Name]
			if !ok {
				continue
			}
			seen[tool.Name] = true

			common, _ := tool.InputSchema["required"].([]string)
			if strings.Join(common, ",") != strings.Join(spec.common, ",") {
				t.Errorf("tool %q required = %v, want %v", tool.Name, common, spec.common)
			}

			branches, ok := tool.InputSchema["oneOf"].([]any)
			if !ok {
				t.Errorf("tool %q has no oneOf", tool.Name)
				continue
			}
			if len(branches) != len(spec.actions) {
				t.Errorf("tool %q oneOf has %d branches, want %d", tool.Name, len(branches), len(spec.actions))
			}
			for _, branch := range branches {
				b, ok := branch.(map[string]any)
				if !ok {
					t.Errorf("tool %q branch = %T", tool.Name, branch)
					continue
				}
				props, _ := b["properties"].(map[string]any)
				actionProp, _ := props["action"].(map[string]any)
				action, _ := actionProp["const"].(string)
				extras, known := spec.actions[action]
				if !known {
					t.Errorf("tool %q has a branch for unknown action %q", tool.Name, action)
					continue
				}
				required, _ := b["required"].([]string)
				expected := append(append([]string{}, spec.common...), extras...)
				if strings.Join(required, ",") != strings.Join(expected, ",") {
					t.Errorf("tool %q action %q required = %v, want %v", tool.Name, action, required, expected)
				}

				fields, wantAnyOf := spec.anyOf[action]
				branchAnyOf, hasAnyOf := b["anyOf"].([]any)
				switch {
				case !wantAnyOf && hasAnyOf:
					t.Errorf("tool %q action %q has an unexpected anyOf: %v", tool.Name, action, b["anyOf"])
				case wantAnyOf && (!hasAnyOf || len(branchAnyOf) != len(fields)):
					t.Errorf("tool %q action %q anyOf = %v, want %v", tool.Name, action, b["anyOf"], fields)
				case wantAnyOf:
					for i, field := range fields {
						branch, _ := branchAnyOf[i].(map[string]any)
						if !hasRequired(branch, field) {
							t.Errorf("tool %q action %q anyOf branch %d = %v, want %q", tool.Name, action, i, branchAnyOf[i], field)
						}
					}
				}
			}
		}
	}
	for name := range want {
		if !seen[name] {
			t.Errorf("tool %q was not registered", name)
		}
	}
}

// TestToolsConstrainAdvertisedStatus verifies no tool advertises the success and
// message envelope loosely. A mutation requires both fields, while an
// action-routed tool separates them into an anyOf branch so a read or list
// result is never described as returning the envelope.
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
