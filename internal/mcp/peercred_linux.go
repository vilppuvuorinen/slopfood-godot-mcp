//go:build linux

package mcp

import (
	"fmt"
	"net"
	"syscall"
)

// peerIdentity returns the Linux kernel's SO_PEERCRED identity for the process
// on the other end of conn: its effective UID and PID. The kernel fills the
// credentials at connect time, so a peer cannot forge them.
func peerIdentity(conn *net.UnixConn) (PeerIdentity, error) {
	raw, err := conn.SyscallConn()
	if err != nil {
		return PeerIdentity{}, fmt.Errorf("peer identity syscall conn: %w", err)
	}
	var (
		cred    *syscall.Ucred
		credErr error
	)
	if err := raw.Control(func(fd uintptr) {
		//nolint:gosec // fd comes from the live socket's SyscallConn.
		cred, credErr = syscall.GetsockoptUcred(int(fd), syscall.SOL_SOCKET, syscall.SO_PEERCRED)
	}); err != nil {
		return PeerIdentity{}, fmt.Errorf("peer identity control: %w", err)
	}
	if credErr != nil {
		return PeerIdentity{}, fmt.Errorf("peer identity getsockopt: %w", credErr)
	}
	return PeerIdentity{UserID: cred.Uid, ProcessID: int(cred.Pid)}, nil
}
