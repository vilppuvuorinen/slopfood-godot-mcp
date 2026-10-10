package transport_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"strings"
	"testing"

	"github.com/vilppuvuorinen/slopfood-godot-mcp/internal/mcp"
	"github.com/vilppuvuorinen/slopfood-godot-mcp/internal/mcp/transport"
)

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

func TestConnectionServesProtocol(t *testing.T) {
	conn := &fakeConn{in: strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"tools/list"}` + "\n")}
	srv := mcp.NewServer(mcp.ServerInfo{Name: "test", Version: "0.0.0"}, slog.New(slog.NewTextHandler(io.Discard, nil)))

	if err := srv.Serve(context.Background(), conn, conn); err != nil {
		t.Fatalf("Serve: %v", err)
	}
	if !strings.Contains(conn.out.String(), `"tools"`) {
		t.Errorf("output = %s", conn.out.String())
	}
}

func TestConnectionPeerIdentity(t *testing.T) {
	want := transport.PeerIdentity{UserID: 1000, ProcessID: 4242}
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
	l := &fakeListener{conns: make(chan transport.Connection, 1), closed: make(chan struct{})}
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
	l := &fakeListener{conns: make(chan transport.Connection), closed: make(chan struct{})}
	if err := l.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	if _, err := l.Accept(); !errors.Is(err, errListenerClosed) {
		t.Fatalf("err = %v, want %v", err, errListenerClosed)
	}
}

func TestListenFuncReturnsListener(t *testing.T) {
	l := &fakeListener{conns: make(chan transport.Connection), closed: make(chan struct{})}
	var gotAddr string
	var listen transport.ListenFunc = func(addr string) (transport.Listener, error) {
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
