//go:build windows

package mcp

import (
	"io"
	"testing"
)

// newTestEndpoint binds the production loopback-TCP transport, the default on
// Windows where Unix sockets are unavailable, and returns its listen and dial
// functions.
func newTestEndpoint(t *testing.T) (ListenFunc, DialFunc) {
	t.Helper()
	listener := newTCPListenerForTest(t)
	dial := func(string) (io.ReadWriteCloser, error) { return DialTCP(listener.endpointPath) }
	return fixedListen(listener), dial
}
