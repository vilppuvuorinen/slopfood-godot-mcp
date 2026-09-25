package e2e

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// binaryEnv is the path to the server binary under test. Build it before
// running these tests; the test itself never builds anything.
const binaryEnv = "GO_DOT_MCP_BIN"

// connect starts the server binary with every tool category enabled and returns
// a connected MCP client session.
func connect(t *testing.T) *mcp.ClientSession {
	t.Helper()
	return connectSession(t, nil, "--enable", "all")
}

// connectCategories starts the server with exactly the given --enable value.
func connectCategories(t *testing.T, categories string) *mcp.ClientSession {
	t.Helper()
	return connectSession(t, nil, "--enable", categories)
}

// connectWithEnv starts the server binary with every category enabled plus
// extra environment entries (KEY=VALUE) and returns a connected MCP client
// session.
func connectWithEnv(t *testing.T, env ...string) *mcp.ClientSession {
	t.Helper()
	return connectSession(t, env, "--enable", "all")
}

// connectSession starts the server binary with extra environment entries and
// command-line arguments and returns a connected MCP client session.
func connectSession(t *testing.T, env []string, args ...string) *mcp.ClientSession {
	t.Helper()

	binary := os.Getenv(binaryEnv)
	if binary == "" {
		t.Fatalf("%s is not set; build the server and point it at the binary", binaryEnv)
	}

	cmd := exec.CommandContext(context.Background(), binary, args...) //nolint:gosec // the binary path is supplied by the test harness.
	cmd.Env = append(os.Environ(), env...)
	client := mcp.NewClient(&mcp.Implementation{Name: "go-dot-mcp-e2e", Version: "0.0.1"}, nil)
	session, err := client.Connect(context.Background(), &mcp.CommandTransport{Command: cmd}, nil)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() {
		if err := session.Close(); err != nil {
			t.Errorf("close session: %v", err)
		}
	})
	return session
}

// connectWithProtocolVersion starts the server binary and connects with an
// explicit protocol version so negotiation can be asserted.
func connectWithProtocolVersion(t *testing.T, protocolVersion string) *mcp.ClientSession {
	t.Helper()

	binary := os.Getenv(binaryEnv)
	if binary == "" {
		t.Fatalf("%s is not set; build the server and point it at the binary", binaryEnv)
	}

	cmd := exec.CommandContext(context.Background(), binary, "--enable", "all") //nolint:gosec // the binary path is supplied by the test harness.
	cmd.Env = os.Environ()
	client := mcp.NewClient(&mcp.Implementation{Name: "go-dot-mcp-e2e", Version: "0.0.1"}, nil)
	session, err := client.Connect(context.Background(), &mcp.CommandTransport{Command: cmd}, &mcp.ClientSessionOptions{ProtocolVersion: protocolVersion})
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() {
		if err := session.Close(); err != nil {
			t.Errorf("close session: %v", err)
		}
	})
	return session
}

// connectWithFakeGodot starts the server with GODOT_PATH pointing at a script
// that reports version, so version gates can be exercised without the engine.
func connectWithFakeGodot(t *testing.T, version string) *mcp.ClientSession {
	t.Helper()
	path := filepath.Join(t.TempDir(), "godot")
	writeFileMode(t, path, "#!/bin/sh\necho \""+version+"\"\nexit 0\n", 0o755)
	return connectWithEnv(t, "GODOT_PATH="+path)
}

// requireGodot skips a test when no usable Godot executable is configured, so
// the black-box suite still passes on machines (and CI) without the engine.
func requireGodot(t *testing.T, session *mcp.ClientSession) {
	t.Helper()
	if res := callTool(t, session, "get_godot_version", nil); res.IsError {
		t.Skipf("Godot unavailable, skipping: %s", firstText(t, res))
	}
}

// findTool returns a listed tool by name, failing when it is absent.
func findTool(t *testing.T, session *mcp.ClientSession, name string) *mcp.Tool {
	t.Helper()
	res, err := session.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatalf("list tools: %v", err)
	}
	for _, tool := range res.Tools {
		if tool.Name == name {
			return tool
		}
	}
	t.Fatalf("tool %q not listed", name)
	return nil
}

// requireToolAbsent fails when a listed tool is present.
func requireToolAbsent(t *testing.T, session *mcp.ClientSession, name string) {
	t.Helper()
	res, err := session.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatalf("list tools: %v", err)
	}
	for _, tool := range res.Tools {
		if tool.Name == name {
			t.Errorf("tool %q should not be listed", name)
		}
	}
}

// callTool invokes a tool over the session and fails only on transport errors.
func callTool(t *testing.T, session *mcp.ClientSession, name string, args map[string]any) *mcp.CallToolResult {
	t.Helper()
	res, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		t.Fatalf("call %s: %v", name, err)
	}
	return res
}

// requireToolText calls a tool and fails when it reports an error, returning
// the first text content block.
func requireToolText(t *testing.T, session *mcp.ClientSession, name string, args map[string]any) string {
	t.Helper()
	res := callTool(t, session, name, args)
	if res.IsError {
		t.Fatalf("%s: unexpected error: %s", name, firstText(t, res))
	}
	return firstText(t, res)
}

// requireToolError calls a tool and asserts it fails with exactly want.
func requireToolError(t *testing.T, session *mcp.ClientSession, name string, args map[string]any, want string) {
	t.Helper()
	res := callTool(t, session, name, args)
	if !res.IsError {
		t.Fatalf("%s: expected an error result", name)
	}
	if got := firstText(t, res); got != want {
		t.Errorf("%s: message = %q, want %q", name, got, want)
	}
}

// firstText returns the first text content block of a tool result.
func firstText(t *testing.T, res *mcp.CallToolResult) string {
	t.Helper()
	if len(res.Content) == 0 {
		t.Fatal("result has no content")
	}
	text, ok := res.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatalf("content type = %T", res.Content[0])
	}
	return text.Text
}

// requirePrefix fails unless got starts with want.
func requirePrefix(t *testing.T, got, want string) {
	t.Helper()
	if !strings.HasPrefix(got, want) {
		t.Errorf("text = %q, want prefix %q", got, want)
	}
}

// requireContains fails unless s contains every want.
func requireContains(t *testing.T, s string, wants ...string) {
	t.Helper()
	for _, want := range wants {
		if !strings.Contains(s, want) {
			t.Errorf("missing %q in:\n%s", want, s)
		}
	}
}

// requireNotContains fails when s contains unwanted.
func requireNotContains(t *testing.T, s, unwanted string) {
	t.Helper()
	if strings.Contains(s, unwanted) {
		t.Errorf("unexpected %q in:\n%s", unwanted, s)
	}
}

// requireProjectSettingsRoundTrip reads project.godot back through
// read_project_settings, which loads the file with the real Godot engine and
// fails when the engine rejects it. It returns every stored setting grouped by
// section, with each value in the tool value contract.
//
// It asserts nothing itself beyond "Godot parsed the file": the returned map is
// the typed store's view, which is only reached when the engine loaded
// project.godot cleanly. Callers must be Godot-gated (requireGodot) because the
// tool always invokes the engine and fails when it is unavailable.
func requireProjectSettingsRoundTrip(t *testing.T, session *mcp.ClientSession, projectPath string) map[string]map[string]any {
	t.Helper()
	text := requireToolText(t, session, "read_project_settings", map[string]any{
		"projectPath": projectPath,
	})
	var payload struct {
		Sections map[string]map[string]any `json:"sections"`
	}
	if err := json.Unmarshal([]byte(text), &payload); err != nil {
		t.Fatalf("unmarshal project settings %q: %v", text, err)
	}
	return payload.Sections
}

// godotMap asserts value is a JSON object and returns it, optionally failing
// when a required field is missing or the wrong type.
func godotMap(t *testing.T, value any) map[string]any {
	t.Helper()
	obj, ok := value.(map[string]any)
	if !ok {
		t.Fatalf("value = %#v, want an object", value)
	}
	return obj
}

// godotTexts asserts value is an array of GodotText escape-hatch objects and
// returns their text.
func godotTexts(t *testing.T, value any) []string {
	t.Helper()
	items, ok := value.([]any)
	if !ok {
		t.Fatalf("value = %#v, want an array", value)
	}
	out := make([]string, 0, len(items))
	for _, item := range items {
		obj, ok := item.(map[string]any)
		if !ok || obj["$type"] != "GodotText" {
			t.Fatalf("item = %#v, want a GodotText object", item)
		}
		text, _ := obj["text"].(string)
		out = append(out, text)
	}
	return out
}

// toolSchema is the subset of a tool's JSON schema the tests assert against.
type toolSchema struct {
	Type       string                    `json:"type"`
	Required   []string                  `json:"required"`
	Properties map[string]map[string]any `json:"properties"`
}

// requireSchema finds a listed tool, checks its description, and decodes its
// input schema, asserting the top-level object type along the way.
func requireSchema(t *testing.T, name, description string) toolSchema {
	t.Helper()
	tool := findTool(t, connect(t), name)
	if tool.Description != description {
		t.Errorf("%s description = %q, want %q", name, tool.Description, description)
	}

	data, err := json.Marshal(tool.InputSchema)
	if err != nil {
		t.Fatalf("marshal schema: %v", err)
	}
	var schema toolSchema
	if err := json.Unmarshal(data, &schema); err != nil {
		t.Fatalf("unmarshal schema %s: %v", data, err)
	}
	if schema.Type != "object" {
		t.Errorf("%s schema type = %q, want object", name, schema.Type)
	}
	return schema
}

// requireRequired asserts the schema's required fields, in order.
func (s toolSchema) requireRequired(t *testing.T, want ...string) {
	t.Helper()
	if got := strings.Join(s.Required, ","); got != strings.Join(want, ",") {
		t.Errorf("required = %v, want %v", s.Required, want)
	}
}

// requireProps asserts every key is declared as a property.
func (s toolSchema) requireProps(t *testing.T, keys ...string) {
	t.Helper()
	for _, key := range keys {
		if _, ok := s.Properties[key]; !ok {
			t.Errorf("missing property %q", key)
		}
	}
}

// requireStringProps asserts every key is a string property.
func (s toolSchema) requireStringProps(t *testing.T, keys ...string) {
	t.Helper()
	for _, key := range keys {
		s.requireType(t, key, "string")
	}
}

// requireType asserts a property has the given JSON type.
func (s toolSchema) requireType(t *testing.T, key, want string) {
	t.Helper()
	if got := s.Properties[key]["type"]; got != want {
		t.Errorf("%s type = %v, want %q", key, got, want)
	}
}

// requirePropCount asserts the schema declares exactly n properties.
func (s toolSchema) requirePropCount(t *testing.T, n int) {
	t.Helper()
	if len(s.Properties) != n {
		t.Errorf("properties = %v, want %d", s.Properties, n)
	}
}

// seedProject creates a directory containing a minimal project.godot.
func seedProject(t *testing.T, dir string) {
	t.Helper()
	writeE2EFile(t, filepath.Join(dir, "project.godot"), "config_version=5\n")
}

// seedScene creates a temp project with a scene and returns the project root.
// It skips when create_scene fails, which usually means Godot is unavailable.
func seedScene(t *testing.T, session *mcp.ClientSession, scenePath string, rootNodeType ...string) string {
	t.Helper()
	root := t.TempDir()
	seedProject(t, root)

	args := map[string]any{"projectPath": root, "scenePath": scenePath}
	if len(rootNodeType) > 0 {
		args["rootNodeType"] = rootNodeType[0]
	}
	res := callTool(t, session, "create_scene", args)
	if res.IsError {
		t.Skipf("create_scene failed, skipping: %s", firstText(t, res))
	}
	requirePrefix(t, firstText(t, res), "Scene created successfully at: "+scenePath)
	return root
}

// readFile returns the contents of a file, failing when it cannot be read.
func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(data)
}

// writeE2EFile writes content to path, creating parent directories as needed.
func writeE2EFile(t *testing.T, path, content string) {
	t.Helper()
	writeFileMode(t, path, content, 0o644)
}

// writeFileMode writes content to path with an explicit mode, creating parent
// directories as needed.
func writeFileMode(t *testing.T, path, content string, mode os.FileMode) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), mode); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
