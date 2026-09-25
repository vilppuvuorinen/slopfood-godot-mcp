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
		{name: "empty defaults to project", raw: nil, want: []Category{CategoryProject}},
		{name: "blank value defaults to project", raw: []string{""}, want: []Category{CategoryProject}},
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
	if len(seen) != 21 {
		t.Errorf("registered %d tools, want 21", len(seen))
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
