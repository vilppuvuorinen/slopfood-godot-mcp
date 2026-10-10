//go:build windows

package transport_test

import (
	"io"
	"testing"

	"github.com/vilppuvuorinen/slopfood-godot-mcp/internal/mcp/transport"
)

// newTestEndpoint binds the production loopback-TCP transport, the default on
// Windows where Unix sockets are unavailable, and returns its listener and dial
// function.
func newTestEndpoint(t *testing.T) (transport.Listener, transport.DialFunc) {
	t.Helper()
	listener, endpointPath := newTCPEndpointForTest(t)
	dial := func(string) (io.ReadWriteCloser, error) { return transport.DialTCP(endpointPath) }
	return listener, dial
}
