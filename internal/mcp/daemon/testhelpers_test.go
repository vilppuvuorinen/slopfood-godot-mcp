package daemon

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/vilppuvuorinen/slopfood-godot-mcp/internal/mcp"
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

func writeLine(t *testing.T, w io.Writer, line string) {
	t.Helper()
	if _, err := io.WriteString(w, line+"\n"); err != nil {
		t.Fatalf("write %q: %v", line, err)
	}
}
