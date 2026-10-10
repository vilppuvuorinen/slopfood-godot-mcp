package transport

import (
	"errors"
	"io"
)

// Returned by the non-Linux Unix backend; the daemon falls back to filesystem permissions.
var ErrPeerIdentityUnsupported = errors.New("peer credentials unsupported on this platform")

// A transport with no peer identity and no filesystem gate; admission requires a successful token pre-auth.
var ErrPeerIdentityAbsent = errors.New("peer identity absent for this transport")

// Optional pre-auth seam for transports with no kernel peer identity or filesystem gate.
type TokenConnection interface {
	// Consumes a single bounded newline-delimited token line; must be bounded in time and bytes.
	Authenticate() error
	// Reports whether Authenticate succeeded.
	Authenticated() bool
}

// Kernel-reported identity of the process on the far end of a Connection.
type PeerIdentity struct {
	// UserID is the effective user ID of the peer process.
	UserID uint32
	// ProcessID is the peer's process ID where the transport exposes one.
	ProcessID int
}

// One accepted client: the read/write stream plus the peer's identity.
type Connection interface {
	io.ReadWriteCloser
	// An error means the peer cannot be identified and is rejected, unless
	// credentials are unsupported, in which case filesystem permissions gate it.
	PeerIdentity() (PeerIdentity, error)
}

// Accept blocks until a client connects or the listener closes.
type Listener interface {
	Accept() (Connection, error)
	Close() error
}

// Opens a Listener; the address is opaque to the daemon core.
type ListenFunc func(addr string) (Listener, error)

// Opens a client connection; an error means no daemon is listening yet.
type DialFunc func(addr string) (io.ReadWriteCloser, error)
