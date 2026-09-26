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

	"github.com/vilppuvuorinen/go-dot-mcp/internal/mcp"
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
	for _, raw := range [][]string{{"bogus"}, {"all,bogus"}, {"project,nope"}} {
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
	if !all["get_project_info"] || !all["create_scene"] || !all["get_uid"] {
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
	if len(seen) != 25 {
		t.Errorf("registered %d tools, want 25", len(seen))
	}
}

// TestToolsAdvertiseMetadata verifies every tool carries the metadata modern
// clients rely on: a title, behavior annotations, and an output schema so
// structured results can be consumed programmatically.
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
			if tool.OutputSchema == nil {
				t.Errorf("tool %q has no output schema", tool.Name)
			}
		}
	}
}

// TestToolAnnotationsMatchBehavior pins the read-only/destructive hints to what
// each tool actually does, so a mislabeled hint cannot survive a change. A
// read-only tool must say so and never claim to be destructive. Every other
// tool may write, so it must not claim to be read-only; tools that only add
// data (create_project refuses to overwrite, add_node appends and never deletes
// a sibling, update_project_uids only writes missing UID sidecars) declare
// themselves non-destructive, and every remaining writer is destructive.
func TestToolAnnotationsMatchBehavior(t *testing.T) {
	readOnly := map[string]bool{
		"get_godot_version":     true,
		"get_project_info":      true,
		"get_uid":               true,
		"read_export_presets":   true,
		"read_input_map":        true,
		"read_project_settings": true,
		"read_resource":         true,
		"read_scene":            true,
		"read_scene_signals":    true,
		"validate_scripts":      true,
	}
	additive := map[string]bool{
		"add_node":            true,
		"create_project":      true,
		"update_project_uids": true,
	}
	for _, category := range AllCategories {
		for _, newTool := range categoryTools[category] {
			tool := newTool()
			if tool.Annotations == nil {
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
			wantDestructive := !additive[tool.Name]
			if wantDestructive && (tool.Annotations.DestructiveHint == nil || !*tool.Annotations.DestructiveHint) {
				t.Errorf("destructive tool %q does not advertise destructiveHint", tool.Name)
			}
			if !wantDestructive && tool.Annotations.DestructiveHint != nil && *tool.Annotations.DestructiveHint {
				t.Errorf("additive tool %q advertises destructiveHint", tool.Name)
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
		},
		[]string{"path"},
		[]actionVariant{
			{Action: "list"},
			{Action: "add", Required: []string{"name"}},
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
}

// TestActionToolsAdvertiseVariants verifies every action-routed tool constrains
// each action's required fields with a discriminated oneOf, so the schema
// reveals the per-action contract instead of only the common fields.
func TestActionToolsAdvertiseVariants(t *testing.T) {
	want := map[string]struct {
		common  []string
		actions map[string][]string
	}{
		"manage_scene_structure": {
			common: []string{"scenePath", "action", "nodePath"},
			actions: map[string][]string{
				"rename":    {"newName"},
				"duplicate": {},
				"move":      {"newParentPath"},
			},
		},
		"manage_scene_signals": {
			common: []string{"scenePath", "action"},
			actions: map[string][]string{
				"add":    {"signalName", "sourcePath", "targetPath", "method"},
				"remove": {"signalName", "sourcePath", "targetPath"},
			},
		},
		"manage_resource": {
			common: []string{"resourcePath", "action"},
			actions: map[string][]string{
				"create": {"resourceType"},
				"modify": {},
			},
		},
		"manage_input_map": {
			common: []string{"action"},
			actions: map[string][]string{
				"add":    {"actionName"},
				"remove": {"actionName"},
			},
		},
		"manage_export_presets": {
			common: []string{"action"},
			actions: map[string][]string{
				"add":    {"name", "platform"},
				"remove": {"name"},
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
