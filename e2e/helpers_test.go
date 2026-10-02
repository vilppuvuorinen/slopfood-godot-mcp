package e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// binaryEnv is the path to the server binary under test; build it before running these tests.
const binaryEnv = "GODOT_MCP_BIN"

// connect binds the server to the working directory; project-scoped tests use connectProject.
func connect(t *testing.T) *mcp.ClientSession {
	t.Helper()
	return connectSession(t, nil)
}

func connectProject(t *testing.T, root string) *mcp.ClientSession {
	t.Helper()
	return connectSession(t, nil, "--project", root)
}

func connectProjectWithEnv(t *testing.T, root string, env ...string) *mcp.ClientSession {
	t.Helper()
	return connectSession(t, env, "--project", root)
}

// newRuntimeDir builds directly under /tmp rather than os.TempDir: Unix socket
// paths are capped near 104 bytes on macOS and its per-user TMPDIR under
// /var/folders is long enough that the derived socket overflows the cap. On
// Windows /tmp is absent and the empty base falls back to os.TempDir.
func newRuntimeDir(t *testing.T) string {
	t.Helper()
	base := ""
	if info, err := os.Stat("/tmp"); err == nil && info.IsDir() {
		base = "/tmp"
	}
	runtimeDir, err := os.MkdirTemp(base, "slopfood-rt-")
	if err != nil {
		t.Fatalf("create runtime dir: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(runtimeDir) })
	return runtimeDir
}

// A private runtime dir keeps sessions from sharing a reused daemon across runs; a short idle timeout lets the spawned daemon exit soon.
func sessionEnv(t *testing.T, runtimeDir string, env, args []string) ([]string, []string) {
	t.Helper()
	childEnv := slices.DeleteFunc(slices.Clone(os.Environ()), func(entry string) bool {
		return strings.HasPrefix(entry, "XDG_RUNTIME_DIR=")
	})
	childEnv = append(childEnv, env...)
	childEnv = append(childEnv, "XDG_RUNTIME_DIR="+runtimeDir)

	childArgs := slices.Clone(args)
	childArgs = append(childArgs, "--idle-timeout", "2s")
	return childEnv, childArgs
}

func connectSession(t *testing.T, env []string, args ...string) *mcp.ClientSession {
	t.Helper()

	binary := os.Getenv(binaryEnv)
	if binary == "" {
		t.Fatalf("%s is not set; build the server and point it at the binary", binaryEnv)
	}

	cmdEnv, cmdArgs := sessionEnv(t, newRuntimeDir(t), env, args)
	registerDaemonExitWait(t, projectArg(cmdArgs))
	cmd := exec.CommandContext(context.Background(), binary, cmdArgs...) //nolint:gosec // the binary path is supplied by the test harness.
	cmd.Env = cmdEnv
	client := mcp.NewClient(&mcp.Implementation{Name: "slopfood-godot-mcp-e2e", Version: "0.0.1"}, nil)
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

func connectWithProtocolVersion(t *testing.T, protocolVersion string) *mcp.ClientSession {
	t.Helper()

	binary := os.Getenv(binaryEnv)
	if binary == "" {
		t.Fatalf("%s is not set; build the server and point it at the binary", binaryEnv)
	}

	cmdEnv, cmdArgs := sessionEnv(t, newRuntimeDir(t), nil, nil)
	cmd := exec.CommandContext(context.Background(), binary, cmdArgs...) //nolint:gosec // the binary path is supplied by the test harness.
	cmd.Env = cmdEnv
	client := mcp.NewClient(&mcp.Implementation{Name: "slopfood-godot-mcp-e2e", Version: "0.0.1"}, nil)
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

const defaultInRuntimeIdleTimeout = "2s"

// connectInRuntime returns an error instead of failing so a test can start several sessions concurrently; the ignored close error covers a test that kills the daemon.
func connectInRuntime(t *testing.T, runtimeDir string, args ...string) (*mcp.ClientSession, error) {
	t.Helper()

	binary := os.Getenv(binaryEnv)
	if binary == "" {
		return nil, fmt.Errorf("%s is not set; build the server and point it at the binary", binaryEnv)
	}

	childEnv := slices.DeleteFunc(slices.Clone(os.Environ()), func(entry string) bool {
		return strings.HasPrefix(entry, "XDG_RUNTIME_DIR=")
	})
	childEnv = append(childEnv, "XDG_RUNTIME_DIR="+runtimeDir)

	childArgs := slices.Clone(args)
	if !hasIdleTimeout(childArgs) {
		childArgs = append(childArgs, "--idle-timeout", defaultInRuntimeIdleTimeout)
	}
	registerDaemonExitWait(t, projectArg(childArgs))

	cmd := exec.CommandContext(context.Background(), binary, childArgs...) //nolint:gosec // the binary path is supplied by the test harness.
	cmd.Env = childEnv
	client := mcp.NewClient(&mcp.Implementation{Name: "slopfood-godot-mcp-e2e", Version: "0.0.1"}, nil)
	session, err := client.Connect(context.Background(), &mcp.CommandTransport{Command: cmd}, nil)
	if err != nil {
		return nil, fmt.Errorf("connect in runtime: %w", err)
	}
	t.Cleanup(func() { _ = session.Close() })
	return session, nil
}

func hasIdleTimeout(args []string) bool {
	for _, arg := range args {
		if arg == "--idle-timeout" || arg == "-idle-timeout" ||
			strings.HasPrefix(arg, "--idle-timeout=") || strings.HasPrefix(arg, "-idle-timeout=") {
			return true
		}
	}
	return false
}

const daemonWaitTimeout = 15 * time.Second

var daemonPollInterval = 100 * time.Millisecond

func waitForDaemon(t *testing.T, project string, want int) {
	t.Helper()
	deadline := time.Now().Add(daemonWaitTimeout)
	var got []int
	for time.Now().Before(deadline) {
		got = daemonPIDs(t, project)
		if len(got) == want {
			return
		}
		time.Sleep(daemonPollInterval)
	}
	t.Fatalf("daemon count = %d %v, want %d for project %s", len(got), got, want, project)
}

func waitForNoDaemon(t *testing.T, project string) {
	t.Helper()
	waitForDaemon(t, project, 0)
}

var (
	daemonExitMu   sync.Mutex
	daemonExitSeen = map[*testing.T]map[string]struct{}{}
)

// registerDaemonExitWait waits for detached daemons to exit before removing the project dir: the engine rewrites .godot/editor while running, so RemoveAll can hit ENOTEMPTY.
func registerDaemonExitWait(t *testing.T, project string) {
	t.Helper()
	if project == "" {
		return
	}
	daemonExitMu.Lock()
	projects := daemonExitSeen[t]
	if projects == nil {
		projects = map[string]struct{}{}
		daemonExitSeen[t] = projects
	}
	if _, seen := projects[project]; seen {
		daemonExitMu.Unlock()
		return
	}
	projects[project] = struct{}{}
	daemonExitMu.Unlock()

	t.Cleanup(func() {
		daemonExitMu.Lock()
		delete(daemonExitSeen[t], project)
		if len(daemonExitSeen[t]) == 0 {
			delete(daemonExitSeen, t)
		}
		daemonExitMu.Unlock()
		waitForNoDaemon(t, project)
	})
}

func projectArg(args []string) string {
	for i, arg := range args {
		if arg == "--project" && i+1 < len(args) {
			return args[i+1]
		}
		if value, ok := strings.CutPrefix(arg, "--project="); ok {
			return value
		}
	}
	return ""
}

func listToolsOnce(t *testing.T, session *mcp.ClientSession) map[string]*mcp.Tool {
	t.Helper()
	res, err := session.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatalf("list tools: %v", err)
	}
	tools := make(map[string]*mcp.Tool, len(res.Tools))
	for _, tool := range res.Tools {
		tools[tool.Name] = tool
	}
	return tools
}

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

func callTool(t *testing.T, session *mcp.ClientSession, name string, args map[string]any) *mcp.CallToolResult {
	t.Helper()
	res, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		t.Fatalf("call %s: %v", name, err)
	}
	return res
}

func requireToolText(t *testing.T, session *mcp.ClientSession, name string, args map[string]any) string {
	t.Helper()
	res := callTool(t, session, name, args)
	if res.IsError {
		t.Fatalf("%s: unexpected error: %s", name, firstText(t, res))
	}
	return firstText(t, res)
}

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

func requirePrefix(t *testing.T, got, want string) {
	t.Helper()
	if !strings.HasPrefix(got, want) {
		t.Errorf("text = %q, want prefix %q", got, want)
	}
}

func requireContains(t *testing.T, s string, wants ...string) {
	t.Helper()
	for _, want := range wants {
		if !strings.Contains(s, want) {
			t.Errorf("missing %q in:\n%s", want, s)
		}
	}
}

func requireNotContains(t *testing.T, s, unwanted string) {
	t.Helper()
	if strings.Contains(s, unwanted) {
		t.Errorf("unexpected %q in:\n%s", unwanted, s)
	}
}

type toolSchema struct {
	Type       string                    `json:"type"`
	Required   []string                  `json:"required"`
	Properties map[string]map[string]any `json:"properties"`
}

func requireSchema(t *testing.T, tools map[string]*mcp.Tool, name, description string) toolSchema {
	t.Helper()
	tool, ok := tools[name]
	if !ok {
		t.Fatalf("tool %q not listed", name)
	}
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

func (s toolSchema) requireRequired(t *testing.T, want ...string) {
	t.Helper()
	if got := strings.Join(s.Required, ","); got != strings.Join(want, ",") {
		t.Errorf("required = %v, want %v", s.Required, want)
	}
}

func (s toolSchema) requireProps(t *testing.T, keys ...string) {
	t.Helper()
	for _, key := range keys {
		if _, ok := s.Properties[key]; !ok {
			t.Errorf("missing property %q", key)
		}
	}
}

func (s toolSchema) requireStringProps(t *testing.T, keys ...string) {
	t.Helper()
	for _, key := range keys {
		s.requireType(t, key, "string")
	}
}

func (s toolSchema) requireType(t *testing.T, key, want string) {
	t.Helper()
	if got := s.Properties[key]["type"]; got != want {
		t.Errorf("%s type = %v, want %q", key, got, want)
	}
}

func (s toolSchema) requirePropCount(t *testing.T, n int) {
	t.Helper()
	if len(s.Properties) != n {
		t.Errorf("properties = %v, want %d", s.Properties, n)
	}
}

func seedProject(t *testing.T, dir string) {
	t.Helper()
	writeE2EFile(t, filepath.Join(dir, "project.godot"), "config_version=5\n")
}

func newProject(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	seedProject(t, root)
	return root
}

func seedScene(t *testing.T, scenePath string, rootNodeType ...string) (string, *mcp.ClientSession) {
	t.Helper()
	root := newProject(t)
	session := connectProject(t, root)
	createScene(t, session, scenePath, rootNodeType...)
	return root, session
}

func createScene(t *testing.T, session *mcp.ClientSession, scenePath string, rootNodeType ...string) {
	t.Helper()
	args := map[string]any{"scenePath": scenePath}
	if len(rootNodeType) > 0 {
		args["rootNodeType"] = rootNodeType[0]
	}
	res := callTool(t, session, "create_scene", args)
	if res.IsError {
		t.Fatalf("create_scene failed: %s", firstText(t, res))
	}
	requirePrefix(t, firstText(t, res), "Scene created successfully at: "+scenePath)
}

func assertNonEmptyFile(t *testing.T, path string) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat artifact: %v", err)
	}
	if !info.Mode().IsRegular() || info.Size() == 0 {
		t.Errorf("artifact %s = mode %v size %d, want a non-empty regular file", path, info.Mode(), info.Size())
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(data)
}

func writeE2EFile(t *testing.T, path, content string) {
	t.Helper()
	writeFileMode(t, path, content, 0o644)
}

func writeFileMode(t *testing.T, path, content string, mode os.FileMode) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), mode); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
