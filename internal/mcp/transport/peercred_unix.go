//go:build !linux && !windows

package transport

import "net"

// Unimplemented on this Unix; reports credentials as unsupported so the daemon
// falls back to the socket's filesystem permissions.
func peerIdentity(_ *net.UnixConn) (PeerIdentity, error) {
	return PeerIdentity{}, ErrPeerIdentityUnsupported
}
