package transport_test

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/vilppuvuorinen/slopfood-godot-mcp/internal/mcp"
	"github.com/vilppuvuorinen/slopfood-godot-mcp/internal/mcp/daemon"
	"github.com/vilppuvuorinen/slopfood-godot-mcp/internal/mcp/transport"
)

// Long enough that no test job reaches it by accident.
const testQueueTimeout = time.Minute

func testServer() *mcp.Server {
	s := mcp.NewServer(mcp.ServerInfo{Name: "test", Version: "0.0.0"}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	s.Instructions = "test instructions"
	s.RegisterTool(mcp.Tool{
		Name:         "echo",
		Title:        "Echo",
		Description:  "echo",
		InputSchema:  map[string]any{"type": "object"},
		OutputSchema: map[string]any{"type": "object"},
		Annotations:  &mcp.ToolAnnotations{ReadOnlyHint: mcp.Bool(true)},
		Handler: func(_ context.Context, args json.RawMessage) (any, error) {
			return string(args), nil
		},
	})
	return s
}

// A "block" tool signals started then blocks until canceled, then signals canceled.
func blockingServer() (*mcp.Server, <-chan struct{}, <-chan struct{}) {
	s := mcp.NewServer(mcp.ServerInfo{Name: "test", Version: "0.0.0"}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	started := make(chan struct{})
	canceled := make(chan struct{})
	s.RegisterTool(mcp.Tool{
		Name:        "block",
		Description: "block",
		InputSchema: map[string]any{"type": "object"},
		Handler: func(ctx context.Context, _ json.RawMessage) (any, error) {
			close(started)
			<-ctx.Done()
			close(canceled)
			return nil, ctx.Err()
		},
	})
	return s, started, canceled
}

func stopDaemon(t *testing.T, d *daemon.Daemon, done <-chan error) {
	t.Helper()
	if err := d.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Serve: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Serve did not return after Close")
	}
}

func writeLine(t *testing.T, w io.Writer, line string) {
	t.Helper()
	if _, err := io.WriteString(w, line+"\n"); err != nil {
		t.Fatalf("write %q: %v", line, err)
	}
}

// fixedListen hands the daemon a pre-bound listener; the address is ignored.
func fixedListen(l transport.Listener) transport.ListenFunc {
	return func(string) (transport.Listener, error) { return l, nil }
}

// Fresh Unix socket path under a short temp prefix, removed when the test ends.
func shortSocketPath(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "mcp-it-")
	if err != nil {
		t.Fatalf("MkdirTemp: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	return filepath.Join(dir, "d.sock")
}

// newTCPEndpointForTest binds a loopback-TCP listener and returns it with its
// published endpoint path.
func newTCPEndpointForTest(t *testing.T) (*transport.TCPListener, string) {
	t.Helper()
	endpointPath := filepath.Join(t.TempDir(), "runtime", "proj.endpoint")
	lock, err := transport.OpenListenerLock(endpointPath + ".lock")
	if err != nil {
		t.Fatalf("OpenListenerLock: %v", err)
	}
	listener, err := transport.ListenTCP("", endpointPath, lock)
	if err != nil {
		_ = lock.Close()
		t.Fatalf("ListenTCP: %v", err)
	}
	t.Cleanup(func() { _ = listener.Close() })
	return listener, endpointPath
}

// readTCPEndpoint reads the published host and port from the endpoint file.
func readTCPEndpoint(t *testing.T, path string) (host string, port int) {
	t.Helper()
	data, err := os.ReadFile(path) //nolint:gosec // the path is derived from a test temp dir.
	if err != nil {
		t.Fatalf("read endpoint: %v", err)
	}
	var record struct {
		Host string `json:"host"`
		Port int    `json:"port"`
	}
	if err := json.Unmarshal(data, &record); err != nil {
		t.Fatalf("decode endpoint: %v", err)
	}
	return record.Host, record.Port
}
