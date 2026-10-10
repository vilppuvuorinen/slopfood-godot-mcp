//go:build !windows

package daemon

import (
	"io"
	"testing"

	"github.com/vilppuvuorinen/slopfood-godot-mcp/internal/mcp/transport"
)

// newTestEndpoint binds the production Unix-socket transport, the default on
// this platform, and returns its listener and dial function.
func newTestEndpoint(t *testing.T) (transport.Listener, transport.DialFunc) {
	t.Helper()
	sock := shortSocketPath(t)
	l, err := transport.ListenUnix(sock)
	if err != nil {
		t.Fatalf("ListenUnix: %v", err)
	}
	t.Cleanup(func() { _ = l.Close() })
	dial := func(string) (io.ReadWriteCloser, error) { return transport.DialUnix(sock) }
	return l, dial
}
