//go:build !windows

package e2e

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

// requireDevTransport skips the test when the server binary was built without
// the devtransport tag, so --transport is an unknown flag and the tag-free
// Linux CI e2e job stays green. Local `make e2e` builds with the tag and runs
// the TCP lane.
func requireDevTransport(t *testing.T) {
	t.Helper()
	binary := os.Getenv(binaryEnv)
	if binary == "" {
		t.Fatalf("%s is not set; build the server and point it at the binary", binaryEnv)
	}
	cmd := exec.CommandContext(context.Background(), binary, "--transport", "tcp", "--version") //nolint:gosec // the binary path is supplied by the test harness.
	if err := cmd.Run(); err != nil {
		t.Skipf("server binary was built without devtransport (--transport is unknown): %v", err)
	}
}

// endpointFile waits until a single TCP endpoint record appears under
// runtimeDir's daemon directory and returns its path.
func endpointFile(t *testing.T, runtimeDir string) string {
	t.Helper()
	pattern := filepath.Join(runtimeDir, "slopfood-godot-mcp", "*.endpoint")
	deadline := time.Now().Add(daemonWaitTimeout)
	for time.Now().Before(deadline) {
		matches, err := filepath.Glob(pattern)
		if err != nil {
			t.Fatalf("glob %s: %v", pattern, err)
		}
		if len(matches) == 1 {
			return matches[0]
		}
		time.Sleep(daemonPollInterval)
	}
	t.Fatalf("no single endpoint file under %s", pattern)
	return ""
}

// TestDaemonTCPTransport runs a client, a detached daemon, and the engine over
// the loopback-TCP transport: the endpoint record appears, tools serve, a
// second daemon loses to the exclusive lock, and the record is removed on idle
// exit.
func TestDaemonTCPTransport(t *testing.T) {
	t.Parallel()
	requireDevTransport(t)
	project := newProject(t)
	runtimeDir := newRuntimeDir(t)

	session, err := connectInRuntime(t, runtimeDir, daemonProjectArgs(project, "--transport", "tcp", "--idle-timeout", "1s")...)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	if _, listErr := session.ListTools(context.Background(), nil); listErr != nil {
		t.Fatalf("tools/list: %v", listErr)
	}
	waitForDaemon(t, project, 1)

	endpoint := endpointFile(t, runtimeDir)

	// A second daemon for the same project cannot bind: the exclusive lock
	// rejects it, so the process exits non-zero instead of stealing the
	// endpoint.
	loser := exec.CommandContext(context.Background(), os.Getenv(binaryEnv), //nolint:gosec // the binary path is supplied by the test harness.
		daemonSubcommand, "--transport", "tcp", "--project", project, "--enable", "all")
	loser.Env = append(os.Environ(), "XDG_RUNTIME_DIR="+runtimeDir)
	if runErr := loser.Run(); runErr == nil {
		t.Error("second TCP daemon exited zero, want the exclusive lock to reject it")
	}
	// The loser must not have removed the winner's endpoint record.
	if _, statErr := os.Stat(endpoint); statErr != nil {
		t.Errorf("endpoint record missing after a losing spawn: %v", statErr)
	}

	// A second client joins the same daemon over TCP.
	joined, err := connectInRuntime(t, runtimeDir, daemonProjectArgs(project, "--transport", "tcp", "--idle-timeout", "1s")...)
	if err != nil {
		t.Fatalf("second connect: %v", err)
	}
	if _, listErr := joined.ListTools(context.Background(), nil); listErr != nil {
		t.Fatalf("second tools/list: %v", listErr)
	}
	waitForDaemon(t, project, 1)

	if closeErr := session.Close(); closeErr != nil {
		t.Errorf("close session: %v", closeErr)
	}
	if closeErr := joined.Close(); closeErr != nil {
		t.Errorf("close second session: %v", closeErr)
	}
	waitForNoDaemon(t, project)

	if _, statErr := os.Stat(endpoint); !os.IsNotExist(statErr) {
		t.Errorf("endpoint record still present after idle exit: %v", statErr)
	}
}

// daemonSubcommand mirrors the server's daemon role selector without importing
// the main package.
const daemonSubcommand = "daemon"
