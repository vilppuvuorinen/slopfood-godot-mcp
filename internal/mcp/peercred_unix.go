//go:build !linux && !windows

package mcp

import "net"

// peerIdentity is unimplemented on this Unix. Darwin and the BSDs do expose
// LOCAL_PEERCRED, but hand-rolling those syscalls is out of scope until a user
// needs it, so the transport reports credentials as unsupported and the daemon
// falls back to the socket's filesystem permissions. The sentinel lives in
// conn.go so the daemon core can reference it wherever the package builds.
func peerIdentity(_ *net.UnixConn) (PeerIdentity, error) {
	return PeerIdentity{}, errPeerIdentityUnsupported
}
