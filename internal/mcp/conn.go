package mcp

import (
	"errors"
	"io"
)

// errPeerIdentityUnsupported reports that a transport cannot ask its kernel for
// peer credentials on this platform. The daemon treats it as "rely on the
// filesystem permissions that gate the transport" rather than as a failed
// authorization; any other PeerIdentity error is a rejection. It is defined
// here rather than in a platform file because the daemon core references it
// wherever the package builds, and only the non-Linux Unix backend returns it.
var errPeerIdentityUnsupported = errors.New("peer credentials unsupported on this platform")

// errPeerIdentityAbsent reports that a transport has no peer identity at all
// and no filesystem permission acting as its gate: a loopback-TCP port is
// reachable by any local process and exposes no kernel credentials. Unlike
// errPeerIdentityUnsupported, the daemon must not fall through to trusting the
// transport; it requires the connection's token pre-auth to have succeeded
// first. Only the TCP backend returns it.
var errPeerIdentityAbsent = errors.New("peer identity absent for this transport")

// TokenConnection is the optional pre-auth seam for transports with no kernel
// peer identity and no filesystem permission gate. The TCP connection
// implements it; the Unix and stdio connections do not, so "no token" means
// the daemon skips the pre-auth step and runs the identical Serve path. The
// daemon calls Authenticate in the per-connection goroutine, never on the
// accept path, and only serves the connection when it succeeds and
// Authenticated reports true.
type TokenConnection interface {
	// Authenticate consumes the transport's token prelude, a single bounded
	// newline-delimited line read from the connection. It returns an error for
	// a missing, malformed, or mismatched token, or on a read failure or
	// deadline. It must be bounded in both time and bytes.
	Authenticate() error
	// Authenticated reports whether Authenticate succeeded. The daemon consults
	// it when PeerIdentity returns errPeerIdentityAbsent.
	Authenticated() bool
}

// PeerIdentity is the kernel-reported identity of the process on the far end of
// a Connection. Each platform fills it from whatever the kernel exposes:
// SO_PEERCRED on Linux, LOCAL_PEERCRED on macOS, and GetNamedPipeClientProcessId
// on a future named-pipe backend. The daemon compares it to its own identity as
// defense in depth over the filesystem permissions that gate the transport.
type PeerIdentity struct {
	// UserID is the effective user ID of the peer process.
	UserID uint32
	// ProcessID is the peer's process ID where the transport exposes one.
	ProcessID int
}

// Connection is one accepted client: the io.ReadWriteCloser pair Server.Serve
// consumes, plus the peer's identity for the authorization check. The daemon
// depends on this interface rather than *net.Conn, so the transport stays behind
// the platform-tagged backend and the core never names a socket type.
type Connection interface {
	io.ReadWriteCloser
	// PeerIdentity returns the kernel-reported identity of the process on the
	// other end. An error means the transport cannot identify the peer, which
	// the daemon treats as a failed authorization, unless the platform reports
	// credentials as unsupported (errPeerIdentityUnsupported), in which case the
	// daemon falls back to the filesystem permissions that gate the transport.
	PeerIdentity() (PeerIdentity, error)
}

// Listener accepts connections for the daemon. Accept blocks until a client
// connects or the listener closes. Close stops accepting and releases the
// underlying resource; connections already accepted are unaffected.
type Listener interface {
	Accept() (Connection, error)
	Close() error
}

// ListenFunc opens a Listener for a transport address. It is the one seam the
// daemon core takes as a dependency instead of naming a transport: the Unix
// backend returns a listener over net.Listen("unix", addr), the in-process
// backend returns one over a pair of connected streams, and a Windows backend
// would return one over a named pipe. The address is opaque to the core.
type ListenFunc func(addr string) (Listener, error)

// DialFunc opens a client connection to a transport address. It is the client
// half of the ListenFunc seam: the Unix backend returns one over
// net.Dial("unix", addr), the in-process backend returns one over a listener's
// Dial, and a Windows backend would return one over a named pipe. The address
// is opaque to the caller, and an error means no daemon is listening yet.
type DialFunc func(addr string) (io.ReadWriteCloser, error)
