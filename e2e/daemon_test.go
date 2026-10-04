package e2e

import (
	"context"
	"os"
	"path/filepath"
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

// The daemon's one queue serializes a scene's read-modify-write, so concurrent editors do not lose updates.
func TestDaemonConcurrentMutationsNoLostUpdate(t *testing.T) {
	t.Parallel()
	project := newProject(t)
	runtimeDir := newRuntimeDir(t)

	writeE2EFile(t, filepath.Join(project, "scenes", "main.tscn"), `[gd_scene format=3]

[node name="root" type="Node2D"]

[node name="Alpha" type="Sprite2D" parent="."]

[node name="Beta" type="Sprite2D" parent="."]
`)
	writeE2EFile(t, filepath.Join(project, "scripts", "alpha.gd"), "extends Sprite2D\n")
	writeE2EFile(t, filepath.Join(project, "scripts", "beta.gd"), "extends Sprite2D\n")

	first, err := connectInRuntime(t, runtimeDir, daemonProjectArgs(project, "--idle-timeout", "2s")...)
	if err != nil {
		t.Fatalf("connect first: %v", err)
	}

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
	attachScript := func(session *mcp.ClientSession, nodePath, scriptPath string) {
		res, err := session.CallTool(context.Background(), &mcp.CallToolParams{
			Name: "attach_script",
			Arguments: map[string]any{
				"scenePath":  "scenes/main.tscn",
				"nodePath":   nodePath,
				"scriptPath": scriptPath,
			},
		})
		resultCh <- result{res: res, err: err}
	}
	go attachScript(first, "root/Alpha", "scripts/alpha.gd")
	go attachScript(second, "root/Beta", "scripts/beta.gd")

	for range 2 {
		got := <-resultCh
		if got.err != nil {
			t.Errorf("attach_script: %v", got.err)
			continue
		}
		if got.res.IsError {
			t.Errorf("attach_script: %s", firstText(t, got.res))
		}
	}

	text := requireToolText(t, first, "read", map[string]any{"type": "scene", "path": "scenes/main.tscn"})
	requireContains(t, text, "scripts/alpha.gd", "scripts/beta.gd")
}
