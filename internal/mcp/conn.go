package mcp

import "io"

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
	// the daemon treats as a failed authorization.
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
