package mcp

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"
)

// fakeConn is an in-memory Connection. Reads come from in and writes go to out,
// so Serve and the byte pump can be exercised without a socket, and PeerIdentity
// reports whatever the test configured.
type fakeConn struct {
	in      io.Reader
	out     bytes.Buffer
	peer    PeerIdentity
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

func (c *fakeConn) PeerIdentity() (PeerIdentity, error) {
	return c.peer, c.peerErr
}

// authConn is a fake TokenConnection: it wraps a fakeConn, reports the absent
// peer identity sentinel, and authenticates according to authErr. It models the
// TCP transport's contract without a socket.
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

func (c *authConn) PeerIdentity() (PeerIdentity, error) {
	return PeerIdentity{}, errPeerIdentityAbsent
}

// errListenerClosed is the error fakeListener reports once Close has been called.
var errListenerClosed = errors.New("listener closed")

// fakeListener yields the connections queued on conns until it is closed, after
// which Accept fails. Closed is checked first so the ordering stays deterministic
// even when a connection is queued at the same time.
type fakeListener struct {
	conns  chan Connection
	closed chan struct{}
}

func (l *fakeListener) Accept() (Connection, error) {
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
	_ Connection      = (*fakeConn)(nil)
	_ Listener        = (*fakeListener)(nil)
	_ TokenConnection = (*authConn)(nil)
)

// TestConnectionServesProtocol asserts a Connection is exactly what Serve
// consumes: the daemon can run the existing line protocol over the abstraction
// without knowing it is not stdio.
func TestConnectionServesProtocol(t *testing.T) {
	conn := &fakeConn{in: strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"tools/list"}` + "\n")}

	if err := testServer().Serve(context.Background(), conn, conn); err != nil {
		t.Fatalf("Serve: %v", err)
	}
	if !strings.Contains(conn.out.String(), `"tools"`) {
		t.Errorf("output = %s", conn.out.String())
	}
}

func TestConnectionPeerIdentity(t *testing.T) {
	want := PeerIdentity{UserID: 1000, ProcessID: 4242}
	conn := &fakeConn{peer: want}

	got, err := conn.PeerIdentity()
	if err != nil {
		t.Fatalf("PeerIdentity: %v", err)
	}
	if got != want {
		t.Errorf("peer = %+v, want %+v", got, want)
	}
}

func TestConnectionPeerIdentityError(t *testing.T) {
	want := errors.New("no peer credentials")
	conn := &fakeConn{peerErr: want}

	if _, err := conn.PeerIdentity(); !errors.Is(err, want) {
		t.Fatalf("err = %v, want %v", err, want)
	}
}

func TestConnectionClose(t *testing.T) {
	conn := &fakeConn{}
	if err := conn.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if !conn.closed {
		t.Error("Close did not reach the connection")
	}
}

func TestListenerAcceptReturnsConnection(t *testing.T) {
	conn := &fakeConn{}
	l := &fakeListener{conns: make(chan Connection, 1), closed: make(chan struct{})}
	l.conns <- conn

	got, err := l.Accept()
	if err != nil {
		t.Fatalf("Accept: %v", err)
	}
	if got != conn {
		t.Errorf("Accept returned %v, want the queued connection", got)
	}
}

func TestListenerCloseStopsAccept(t *testing.T) {
	l := &fakeListener{conns: make(chan Connection), closed: make(chan struct{})}
	if err := l.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	if _, err := l.Accept(); !errors.Is(err, errListenerClosed) {
		t.Fatalf("err = %v, want %v", err, errListenerClosed)
	}
}

// TestListenFuncReturnsListener asserts the listen seam is a function the daemon
// can hold as a dependency, decoupled from any concrete backend.
func TestListenFuncReturnsListener(t *testing.T) {
	l := &fakeListener{conns: make(chan Connection), closed: make(chan struct{})}
	var gotAddr string
	var listen ListenFunc = func(addr string) (Listener, error) {
		gotAddr = addr
		return l, nil
	}

	got, err := listen("/tmp/demo.sock")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	if got != l {
		t.Errorf("listener = %v, want the fake", got)
	}
	if gotAddr != "/tmp/demo.sock" {
		t.Errorf("addr = %q, want the address passed to listen", gotAddr)
	}
}
