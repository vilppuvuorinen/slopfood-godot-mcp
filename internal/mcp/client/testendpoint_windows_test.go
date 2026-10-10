//go:build windows

package client

import (
	"io"
	"path/filepath"
	"testing"

	"github.com/vilppuvuorinen/slopfood-godot-mcp/internal/mcp/transport"
)

// newTestEndpoint binds the production loopback-TCP transport, the default on
// Windows where Unix sockets are unavailable, and returns its listen and dial
// functions.
func newTestEndpoint(t *testing.T) (transport.ListenFunc, transport.DialFunc) {
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
	dial := func(string) (io.ReadWriteCloser, error) { return transport.DialTCP(endpointPath) }
	return fixedListen(listener), dial
}
