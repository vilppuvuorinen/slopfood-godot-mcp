//go:build !windows

package transport_test

import (
	"io"
	"testing"

	"github.com/vilppuvuorinen/slopfood-godot-mcp/internal/mcp/transport"
)

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
