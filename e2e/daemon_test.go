package e2e

import (
	"context"
	"encoding/json"
	"os"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func daemonProjectArgs(project string, extra ...string) []string {
	args := make([]string, 0, 4+len(extra))
	args = append(args, "--project", project)
	return append(args, extra...)
}

func TestDaemonSimultaneousSpawnsYieldOneDaemon(t *testing.T) {
	t.Parallel()
	project := newProject(t)
	runtimeDir := newRuntimeDir(t)

	const clients = 4
	type result struct {
		session *mcp.ClientSession
		err     error
	}
	results := make(chan result, clients)
	for range clients {
		go func() {
			session, err := connectInRuntime(t, runtimeDir, daemonProjectArgs(project, "--idle-timeout", "1s")...)
			results <- result{session: session, err: err}
		}()
	}

	sessions := make([]*mcp.ClientSession, 0, clients)
	for range clients {
		got := <-results
		if got.err != nil {
			t.Errorf("connect: %v", got.err)
			continue
		}
		sessions = append(sessions, got.session)
	}
	if len(sessions) != clients {
		t.Fatalf("connected %d sessions, want %d", len(sessions), clients)
	}

	for _, session := range sessions {
		if _, err := session.ListTools(context.Background(), nil); err != nil {
			t.Errorf("tools/list: %v", err)
		}
	}
	waitForDaemon(t, project, 1)

	for _, session := range sessions {
		if err := session.Close(); err != nil {
			t.Errorf("close session: %v", err)
		}
	}
	waitForNoDaemon(t, project)
}

func TestDaemonStaleSocketReclaimed(t *testing.T) {
	t.Parallel()
	project := newProject(t)
	runtimeDir := newRuntimeDir(t)

	session, err := connectInRuntime(t, runtimeDir, daemonProjectArgs(project, "--idle-timeout", "1s")...)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	if _, listErr := session.ListTools(context.Background(), nil); listErr != nil {
		t.Fatalf("tools/list: %v", listErr)
	}
	waitForDaemon(t, project, 1)

	pids := daemonPIDs(t, project)
	if len(pids) != 1 {
		t.Fatalf("daemon pids = %v, want exactly one", pids)
	}
	daemon, findErr := os.FindProcess(pids[0])
	if findErr != nil {
		t.Fatalf("find process %d: %v", pids[0], findErr)
	}
	// SIGKILL skips cleanup, leaving the socket and lock file a reclaim must recover from.
	if killErr := daemon.Kill(); killErr != nil {
		t.Fatalf("kill daemon %d: %v", pids[0], killErr)
	}
	waitForNoDaemon(t, project)
	_ = session.Close()

	revived, reviveErr := connectInRuntime(t, runtimeDir, daemonProjectArgs(project, "--idle-timeout", "1s")...)
	if reviveErr != nil {
		t.Fatalf("reconnect after SIGKILL: %v", reviveErr)
	}
	if _, listErr := revived.ListTools(context.Background(), nil); listErr != nil {
		t.Fatalf("tools/list after reclaim: %v", listErr)
	}
}

func TestDaemonIdleExitAndRespawn(t *testing.T) {
	t.Parallel()
	project := newProject(t)
	runtimeDir := newRuntimeDir(t)

	session, err := connectInRuntime(t, runtimeDir, daemonProjectArgs(project, "--idle-timeout", "500ms")...)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	if _, listErr := session.ListTools(context.Background(), nil); listErr != nil {
		t.Fatalf("tools/list: %v", listErr)
	}
	if closeErr := session.Close(); closeErr != nil {
		t.Errorf("close session: %v", closeErr)
	}
	waitForNoDaemon(t, project)

	respawned, respawnErr := connectInRuntime(t, runtimeDir, daemonProjectArgs(project, "--idle-timeout", "500ms")...)
	if respawnErr != nil {
		t.Fatalf("respawn: %v", respawnErr)
	}
	if _, listErr := respawned.ListTools(context.Background(), nil); listErr != nil {
		t.Fatalf("tools/list after respawn: %v", listErr)
	}
}

// The daemon's one queue serializes add_node's read-modify-write, so concurrent editors do not lose updates.
func TestDaemonConcurrentMutationsNoLostUpdate(t *testing.T) {
	t.Parallel()
	project := newProject(t)
	runtimeDir := newRuntimeDir(t)

	first, err := connectInRuntime(t, runtimeDir, daemonProjectArgs(project, "--idle-timeout", "2s")...)
	if err != nil {
		t.Fatalf("connect first: %v", err)
	}
	requireToolText(t, first, "create_scene", map[string]any{"scenePath": "scenes/main.tscn"})

	second, err := connectInRuntime(t, runtimeDir, daemonProjectArgs(project, "--idle-timeout", "2s")...)
	if err != nil {
		t.Fatalf("connect second: %v", err)
	}
	waitForDaemon(t, project, 1)

	type result struct {
		res *mcp.CallToolResult
		err error
	}
	resultCh := make(chan result, 2)
	addNode := func(session *mcp.ClientSession, name string) {
		res, err := session.CallTool(context.Background(), &mcp.CallToolParams{
			Name: "add_node",
			Arguments: map[string]any{
				"scenePath": "scenes/main.tscn",
				"nodeType":  "Sprite2D",
				"nodeName":  name,
			},
		})
		resultCh <- result{res: res, err: err}
	}
	go addNode(first, "Alpha")
	go addNode(second, "Beta")

	for range 2 {
		got := <-resultCh
		if got.err != nil {
			t.Errorf("add_node: %v", got.err)
			continue
		}
		if got.res.IsError {
			t.Errorf("add_node: %s", firstText(t, got.res))
		}
	}

	text := requireToolText(t, first, "read", map[string]any{"type": "scene", "path": "scenes/main.tscn"})
	var tree sceneNode
	if err := json.Unmarshal([]byte(text), &tree); err != nil {
		t.Fatalf("unmarshal %q: %v", text, err)
	}
	names := make(map[string]bool, len(tree.Children))
	for _, child := range tree.Children {
		names[child.Name] = true
	}
	for _, want := range []string{"Alpha", "Beta"} {
		if !names[want] {
			t.Errorf("node %q missing after concurrent add_node; children = %+v", want, tree.Children)
		}
	}
}
