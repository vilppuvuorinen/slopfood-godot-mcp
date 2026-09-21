package e2e

import (
	"context"
	"os"
	"os/exec"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// connectWithEnv starts the server binary with extra environment entries
// (KEY=VALUE) and returns a connected MCP client session.
func connectWithEnv(t *testing.T, env ...string) *mcp.ClientSession {
	t.Helper()

	binary := os.Getenv(binaryEnv)
	if binary == "" {
		t.Fatalf("%s is not set; build the server and point it at the binary", binaryEnv)
	}

	cmd := exec.Command(binary) //nolint:gosec // the binary path is supplied by the test harness.
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

// requireGodot skips a test when no usable Godot executable is configured, so
// the black-box suite still passes on machines (and CI) without the engine.
func requireGodot(t *testing.T, session *mcp.ClientSession) {
	t.Helper()
	res := callTool(t, session, "get_godot_version", nil)
	if res.IsError {
		t.Skipf("Godot unavailable, skipping: %s", firstText(t, res))
	}
}
