package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"strconv"
	"strings"
	"testing"

	"github.com/vilppuvuorinen/slopfood-godot-mcp/internal/mcp"
)

func registeredToolNames(t *testing.T) map[string]bool {
	t.Helper()
	server := mcp.NewServer(mcp.ServerInfo{Name: "test"}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	Register(server, "")

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

func TestRegisterAllTools(t *testing.T) {
	names := registeredToolNames(t)
	for _, name := range []string{"validate", "attach_script", "run_script", "read", "read_node"} {
		if !names[name] {
			t.Errorf("tool %q not registered", name)
		}
	}
	if len(names) != len(allTools) {
		t.Errorf("registered %d tools, want %d", len(names), len(allTools))
	}
}

func TestToolsUnique(t *testing.T) {
	seen := make(map[string]bool)
	for _, newTool := range allTools {
		tool := newTool()
		if seen[tool.Name] {
			t.Errorf("tool %q registered more than once", tool.Name)
		}
		seen[tool.Name] = true
	}
	if len(seen) != 8 {
		t.Errorf("registered %d tools, want 8", len(seen))
	}
}

func TestToolsAdvertiseMetadata(t *testing.T) {
	for _, newTool := range allTools {
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

func TestToolAnnotationsMatchBehavior(t *testing.T) {
	readOnly := map[string]bool{
		"read":      true,
		"read_node": true,
		"validate":  true,
	}
	nonDestructive := map[string]bool{
		"create_project": true,
	}
	idempotent := map[string]bool{
		"attach_script":       true,
		"export_mesh_library": true,
		"export_project":      true,
	}
	for _, newTool := range allTools {
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

var schemaKeywords = []string{
	"type", "anyOf", "oneOf", "allOf", "not", "enum", "const",
	"$ref", "properties", "additionalProperties", "items", "required",
}

func TestToolsAdvertiseValueTypes(t *testing.T) {
	for _, newTool := range allTools {
		tool := newTool()
		assertTypedSchemas(t, tool.Name, "inputSchema", tool.InputSchema)
		assertTypedSchemas(t, tool.Name, "outputSchema", tool.OutputSchema)
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
	for _, newTool := range allTools {
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
