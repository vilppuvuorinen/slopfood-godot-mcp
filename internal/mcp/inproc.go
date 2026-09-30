package mcp

import (
	"errors"
	"io"
	"net"
	"os"
	"sync"
)

var errInProcClosed = errors.New("in-process listener closed")

// In-process stand-in for connection refused, driving the client's spawn path.
var errInProcNotStarted = errors.New("in-process endpoint not started")

// Bounds dialed-but-not-yet-accepted connections, standing in for a socket's pending queue.
const inProcBacklog = 8

// In-process backend: Dial returns one end of a stream pair to the client and
// Accept returns the other to the daemon.
type InProcListener struct {
	conns     chan Connection
	closed    chan struct{}
	closeOnce sync.Once
	// The identity every accepted connection reports; settable by tests.
	peer PeerIdentity
}

func NewInProcListener() *InProcListener {
	return &InProcListener{
		conns:  make(chan Connection, inProcBacklog),
		closed: make(chan struct{}),
		peer:   currentPeerIdentity(),
	}
}

// The address is ignored; the listener value itself is the rendezvous.
func ListenInProc(_ string) (Listener, error) {
	return NewInProcListener(), nil
}

// Returns the client end of a stream pair and queues the daemon end for Accept.
func (l *InProcListener) Dial() (io.ReadWriteCloser, error) {
	select {
	case <-l.closed:
		return nil, errInProcClosed
	default:
	}
	client, server := net.Pipe()
	conn := &inProcConn{Conn: server, peer: l.peer}
	select {
	case l.conns <- conn:
		return client, nil
	case <-l.closed:
		_ = client.Close()
		_ = server.Close()
		return nil, errInProcClosed
	}
}

// The closed check comes first so a connection queued before Close is still returned.
func (l *InProcListener) Accept() (Connection, error) {
	select {
	case <-l.closed:
		return nil, errInProcClosed
	default:
	}
	select {
	case conn := <-l.conns:
		return conn, nil
	case <-l.closed:
		return nil, errInProcClosed
	}
}

func (l *InProcListener) Close() error {
	l.closeOnce.Do(func() { close(l.closed) })
	return nil
}

// A connected stream plus the identity the transport reports.
type inProcConn struct {
	net.Conn
	peer PeerIdentity
}

func (c *inProcConn) PeerIdentity() (PeerIdentity, error) {
	return c.peer, nil
}

func currentPeerIdentity() PeerIdentity {
	uid := os.Getuid()
	var userID uint32
	if uid >= 0 {
		// os.Getuid is non-negative wherever meaningful; the guard keeps the
		// conversion honest where it reports -1.
		userID = uint32(uid) //nolint:gosec // guarded non-negative above.
	}
	return PeerIdentity{UserID: userID, ProcessID: os.Getpid()}
}

// The in-process transport as a ListenFunc and DialFunc sharing one listener,
// created on first Listen so an early Dial drives the spawn path.
type InProcEndpoint struct {
	mu       sync.Mutex
	listener *InProcListener
}

func NewInProcEndpoint() *InProcEndpoint { return &InProcEndpoint{} }

func (e *InProcEndpoint) Listen(_ string) (Listener, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.listener == nil {
		e.listener = NewInProcListener()
	}
	return e.listener, nil
}

// Fails with errInProcNotStarted until Listen has created the listener.
func (e *InProcEndpoint) Dial(_ string) (io.ReadWriteCloser, error) {
	e.mu.Lock()
	listener := e.listener
	e.mu.Unlock()
	if listener == nil {
		return nil, errInProcNotStarted
	}
	return listener.Dial()
}

var (
	_ Connection = (*inProcConn)(nil)
	_ Listener   = (*InProcListener)(nil)
	_ ListenFunc = ListenInProc
)
