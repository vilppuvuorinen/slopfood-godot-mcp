package e2e

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// daemonProjectArgs is the shared --enable/--project prefix the daemon tests
// pass to connectInRuntime, plus any per-test flags.
func daemonProjectArgs(project string, extra ...string) []string {
	args := make([]string, 0, 4+len(extra))
	args = append(args, "--enable", "all", "--project", project)
	return append(args, extra...)
}

// TestDaemonSimultaneousSpawnsYieldOneDaemon starts several thin clients at
// once against one project and runtime dir, so several race to spawn a daemon.
// Exactly one must win the lock; the rest connect to it. Closing the sessions
// lets the daemon idle out.
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

	// Every client reached the one daemon, so serving tools/list on all of them
	// then finding exactly one daemon proves the losers exited.
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

// TestDaemonStaleSocketReclaimed SIGKILLs the daemon behind a live session,
// leaving its socket and lock file behind, then starts a new client that must
// reclaim the stale path and serve normally.
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
	// SIGKILL skips the daemon's cleanup, leaving the socket and lock file, the
	// crash a reclaim has to recover from.
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

// TestDaemonIdleExitAndRespawn asserts a daemon exits after its idle grace
// period once the last client leaves, and that a new client respawns one on the
// same path.
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

// TestDaemonSocketPermissions asserts the socket is 0600 and its directory 0700,
// the filesystem gate that keeps another user out alongside the daemon's peer
// UID check.
func TestDaemonSocketPermissions(t *testing.T) {
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

	socket := socketPath(t, runtimeDir)
	info, statErr := os.Stat(socket)
	if statErr != nil {
		t.Fatalf("stat socket: %v", statErr)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Errorf("socket mode = %o, want 600", got)
	}
	dirInfo, dirErr := os.Stat(filepath.Dir(socket))
	if dirErr != nil {
		t.Fatalf("stat socket dir: %v", dirErr)
	}
	if got := dirInfo.Mode().Perm(); got != 0o700 {
		t.Errorf("socket dir mode = %o, want 700", got)
	}
}

// TestDaemonConcurrentMutationsNoLostUpdate is the end-to-end lost-update proof:
// two agents share one daemon and edit the same scene at once. The daemon's one
// queue serializes the read-modify-write in add_node, so both nodes survive; two
// independent daemons would each load the original scene and the second save
// would clobber the first.
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
	// Both sessions must hit the same daemon for the shared queue to be the one
	// serialization point.
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

	text := requireToolText(t, first, "read_scene", map[string]any{"scenePath": "scenes/main.tscn"})
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
