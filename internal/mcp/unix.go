//go:build !windows

package mcp

import (
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"sync"
	"syscall"
)

// ListenUnix opens a Unix domain socket Listener at addr. It creates the socket
// directory 0700, takes a non-blocking exclusive flock on addr+".lock" for the
// listener's whole life, reclaims a stale socket left by a crash, and creates
// the socket 0600. The lock is the single-instance rule: a second daemon for the
// same project fails here instead of stealing the address.
func ListenUnix(addr string) (Listener, error) {
	dir := filepath.Dir(addr)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("create socket directory %q: %w", dir, err)
	}
	// MkdirAll leaves a pre-existing directory's mode alone, so tighten it in
	// case a previous version created it world-readable.
	if err := os.Chmod(dir, 0o700); err != nil {
		return nil, fmt.Errorf("tighten socket directory %q: %w", dir, err)
	}

	//nolint:gosec // the lock path is derived from the bound project, not user input.
	lock, err := os.OpenFile(addr+".lock", os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open lock file for %q: %w", addr, err)
	}
	if chmodErr := lock.Chmod(0o600); chmodErr != nil {
		_ = lock.Close()
		return nil, fmt.Errorf("secure lock file for %q: %w", addr, chmodErr)
	}
	// The file descriptor is a valid *os.File descriptor; int is its native
	// syscall type.
	//nolint:gosec // the fd comes from a live *os.File.
	if flockErr := syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); flockErr != nil {
		_ = lock.Close()
		return nil, fmt.Errorf("lock %q: another daemon owns the address: %w", addr, flockErr)
	}

	// A live listener already owns the path: the lock says we are the only lock
	// holder, so this is a duplicate bind from the same process, not a stale
	// socket. Refuse without unlinking, which would strand the live listener.
	if existing, dialErr := net.Dial("unix", addr); dialErr == nil { //nolint:noctx // the dial is a local liveness probe with no context to carry.
		_ = existing.Close()
		_ = lock.Close()
		return nil, fmt.Errorf("listen unix %q: already listening", addr)
	}
	// No live listener, so any file at the path is a leftover from a crash.
	if removeErr := os.Remove(addr); removeErr != nil && !os.IsNotExist(removeErr) {
		_ = lock.Close()
		return nil, fmt.Errorf("remove stale socket %q: %w", addr, removeErr)
	}
	ln, err := net.Listen("unix", addr) //nolint:noctx // the ListenFunc seam carries no context; binding is local and fast.
	if err != nil {
		_ = lock.Close()
		return nil, fmt.Errorf("listen unix %q: %w", addr, err)
	}
	if socketErr := os.Chmod(addr, 0o600); socketErr != nil {
		_ = ln.Close()
		_ = lock.Close()
		return nil, fmt.Errorf("secure socket %q: %w", addr, socketErr)
	}
	unixLn, ok := ln.(*net.UnixListener)
	if !ok {
		_ = ln.Close()
		_ = lock.Close()
		return nil, fmt.Errorf("listen unix %q: unexpected listener type %T", addr, ln)
	}
	return &unixListener{ln: unixLn, lock: lock, path: addr}, nil
}

// DialUnix connects to a Unix domain socket at addr. A failed dial means no
// daemon is listening yet, which is what drives the client's spawn path.
func DialUnix(addr string) (io.ReadWriteCloser, error) {
	conn, err := net.Dial("unix", addr) //nolint:noctx // the DialFunc seam carries no context; the dial is local and fast.
	if err != nil {
		return nil, fmt.Errorf("dial unix %q: %w", addr, err)
	}
	return conn, nil
}

// unixListener is the Unix-socket backend for the Listener seam. It owns the
// *net.UnixListener, the exclusive lock file that makes it the single daemon
// for the address, and the socket path, so Close can unlink the socket and
// release the lock.
type unixListener struct {
	ln   *net.UnixListener
	lock *os.File
	path string
	once sync.Once
	err  error
}

// Accept blocks until a client connects, returning the accepted connection with
// its peer identity available.
func (l *unixListener) Accept() (Connection, error) {
	conn, err := l.ln.AcceptUnix()
	if err != nil {
		return nil, fmt.Errorf("accept unix %q: %w", l.path, err)
	}
	return &unixConn{UnixConn: conn}, nil
}

// Close stops accepting, closes the listener, removes the socket path, and
// releases the lock. It is idempotent and joins any errors it hits.
func (l *unixListener) Close() error {
	l.once.Do(func() {
		var errs []error
		if err := l.ln.Close(); err != nil {
			errs = append(errs, fmt.Errorf("close unix listener %q: %w", l.path, err))
		}
		if err := os.Remove(l.path); err != nil && !os.IsNotExist(err) {
			errs = append(errs, fmt.Errorf("remove socket %q: %w", l.path, err))
		}
		if err := l.lock.Close(); err != nil {
			errs = append(errs, fmt.Errorf("close lock file %q: %w", l.path+".lock", err))
		}
		l.err = errors.Join(errs...)
	})
	return l.err
}

// unixConn is one accepted Unix-socket connection: the byte stream plus the
// kernel-reported peer identity, delegated to the per-OS helper.
type unixConn struct {
	*net.UnixConn
}

// PeerIdentity returns the kernel-reported identity of the connected process.
func (c *unixConn) PeerIdentity() (PeerIdentity, error) {
	return peerIdentity(c.UnixConn)
}

var (
	_ Connection = (*unixConn)(nil)
	_ Listener   = (*unixListener)(nil)
	_ ListenFunc = ListenUnix
	_ DialFunc   = DialUnix
)
