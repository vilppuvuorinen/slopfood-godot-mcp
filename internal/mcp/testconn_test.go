package mcp

import (
	"bytes"
	"errors"
	"io"

	"github.com/vilppuvuorinen/slopfood-godot-mcp/internal/mcp/transport"
)

// fixedListen hands a pre-bound listener to a consumer that ignores the address.
func fixedListen(l transport.Listener) transport.ListenFunc {
	return func(string) (transport.Listener, error) { return l, nil }
}

// In-memory Connection; PeerIdentity reports whatever the test configured.
type fakeConn struct {
	in      io.Reader
	out     bytes.Buffer
	peer    transport.PeerIdentity
	peerErr error
	closed  bool
}

func (c *fakeConn) Read(p []byte) (int, error) {
	return c.in.Read(p) //nolint:wrapcheck // a fake must pass io.EOF and read errors through unchanged.
}

func (c *fakeConn) Write(p []byte) (int, error) {
	return c.out.Write(p) //nolint:wrapcheck // a fake must pass write errors through unchanged.
}

func (c *fakeConn) Close() error { c.closed = true; return nil }

func (c *fakeConn) PeerIdentity() (transport.PeerIdentity, error) {
	return c.peer, c.peerErr
}

// Fake TokenConnection modeling the TCP transport's contract without a socket.
type authConn struct {
	fakeConn
	authErr       error
	authenticated bool
}

func (c *authConn) Authenticate() error {
	if c.authErr != nil {
		return c.authErr
	}
	c.authenticated = true
	return nil
}

func (c *authConn) Authenticated() bool { return c.authenticated }

func (c *authConn) PeerIdentity() (transport.PeerIdentity, error) {
	return transport.PeerIdentity{}, transport.ErrPeerIdentityAbsent
}

var errListenerClosed = errors.New("listener closed")

// Yields queued connections until closed; the closed check comes first for determinism.
type fakeListener struct {
	conns  chan transport.Connection
	closed chan struct{}
}

func (l *fakeListener) Accept() (transport.Connection, error) {
	select {
	case <-l.closed:
		return nil, errListenerClosed
	default:
	}
	select {
	case conn := <-l.conns:
		return conn, nil
	case <-l.closed:
		return nil, errListenerClosed
	}
}

func (l *fakeListener) Close() error {
	close(l.closed)
	return nil
}

var (
	_ transport.Connection      = (*fakeConn)(nil)
	_ transport.Listener        = (*fakeListener)(nil)
	_ transport.TokenConnection = (*authConn)(nil)
)
