//go:build !windows

package client

import (
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/vilppuvuorinen/slopfood-godot-mcp/internal/mcp/transport"
)

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

// newTestEndpoint binds the production Unix-socket transport, the default on
// this platform, and returns its listen and dial functions.
func newTestEndpoint(t *testing.T) (transport.ListenFunc, transport.DialFunc) {
	t.Helper()
	sock := shortSocketPath(t)
	l, err := transport.ListenUnix(sock)
	if err != nil {
		t.Fatalf("ListenUnix: %v", err)
	}
	t.Cleanup(func() { _ = l.Close() })
	dial := func(string) (io.ReadWriteCloser, error) { return transport.DialUnix(sock) }
	return fixedListen(l), dial
}
