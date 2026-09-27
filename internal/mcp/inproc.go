package mcp

import (
	"errors"
	"io"
	"net"
	"os"
	"sync"
)

// errInProcClosed reports that the in-process listener is closed and no longer
// accepts or dials connections.
var errInProcClosed = errors.New("in-process listener closed")

// inProcBacklog bounds how many dialed-but-not-yet-accepted connections the
// in-process listener holds, standing in for the small pending-connection queue
// a real listening socket keeps.
const inProcBacklog = 8

// InProcListener is the in-process backend for the connection seam. It stands a
// pair of connected streams in for the socket: Dial returns one end to the
// client and Accept returns the other to the daemon. It exists so the daemon
// core can be built and tested before any real transport lands; the Unix
// backend later fills the same seam and the core does not change. It names no
// socket type, only the Connection and Listener abstraction.
type InProcListener struct {
	conns     chan Connection
	closed    chan struct{}
	closeOnce sync.Once
	// peer is the identity every accepted connection reports. It defaults to
	// the current process, since an in-process connection is that process, and
	// is settable by tests that need a mismatched peer.
	peer PeerIdentity
}

// NewInProcListener returns an in-process listener exposing both halves of the
// transport: Accept for the daemon core and Dial for the client.
func NewInProcListener() *InProcListener {
	return &InProcListener{
		conns:  make(chan Connection, inProcBacklog),
		closed: make(chan struct{}),
		peer:   currentPeerIdentity(),
	}
}

// ListenInProc opens an in-process Listener through the ListenFunc seam. The
// address is ignored: the in-process transport has no namespace, so the
// listener value itself is the rendezvous. It matches ListenFunc so the daemon
// core can be driven with the in-process backend exactly as it will be with the
// Unix one.
func ListenInProc(_ string) (Listener, error) {
	return NewInProcListener(), nil
}

// Dial returns the client end of a fresh connected stream pair and queues the
// daemon end for Accept. It fails once the listener is closed. The returned
// value is a plain io.ReadWriteCloser; the peer identity belongs to the accepted
// side, which is where the daemon checks it.
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

// Accept returns the next dialed connection, blocking until one arrives or the
// listener closes. A connection that was queued before Close is still returned;
// the closed check comes first so the ordering stays deterministic.
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

// Close stops accepting and dialing and unblocks a pending Accept. It is safe
// to call more than once.
func (l *InProcListener) Close() error {
	l.closeOnce.Do(func() { close(l.closed) })
	return nil
}

// inProcConn is one accepted in-process connection: a connected stream plus the
// identity the in-process transport reports. net.Pipe already supplies the
// io.ReadWriteCloser, so the wrapper only adds PeerIdentity.
type inProcConn struct {
	net.Conn
	peer PeerIdentity
}

func (c *inProcConn) PeerIdentity() (PeerIdentity, error) {
	return c.peer, nil
}

// currentPeerIdentity reports this process's identity, the best an in-process
// transport can do without asking a kernel.
func currentPeerIdentity() PeerIdentity {
	uid := os.Getuid()
	var userID uint32
	if uid >= 0 {
		// os.Getuid is non-negative wherever it is meaningful; the guard keeps
		// the conversion honest on platforms that report -1.
		userID = uint32(uid) //nolint:gosec // guarded non-negative above.
	}
	return PeerIdentity{UserID: userID, ProcessID: os.Getpid()}
}

var (
	_ Connection = (*inProcConn)(nil)
	_ Listener   = (*InProcListener)(nil)
	_ ListenFunc = ListenInProc
)
