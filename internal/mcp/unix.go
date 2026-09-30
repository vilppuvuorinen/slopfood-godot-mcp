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

// Creates the socket directory 0700, takes an exclusive flock on addr+".lock"
// for the listener's life, reclaims a stale socket, and creates the socket 0600.
func ListenUnix(addr string) (Listener, error) {
	dir := filepath.Dir(addr)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("create socket directory %q: %w", dir, err)
	}
	// MkdirAll leaves an existing directory's mode alone, so tighten it in case
	// an older version created it world-readable.
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
	//nolint:gosec // the fd comes from a live *os.File.
	if flockErr := syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); flockErr != nil {
		_ = lock.Close()
		return nil, fmt.Errorf("lock %q: another daemon owns the address: %w", addr, flockErr)
	}

	// The lock says we are the only holder, so a live listener means a duplicate
	// bind from this process; refuse without unlinking, which would strand it.
	if existing, dialErr := net.Dial("unix", addr); dialErr == nil { //nolint:noctx // the dial is a local liveness probe with no context to carry.
		_ = existing.Close()
		_ = lock.Close()
		return nil, fmt.Errorf("listen unix %q: already listening", addr)
	}
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

// A failed dial means no daemon is listening yet, which drives the spawn path.
func DialUnix(addr string) (io.ReadWriteCloser, error) {
	conn, err := net.Dial("unix", addr) //nolint:noctx // the DialFunc seam carries no context; the dial is local and fast.
	if err != nil {
		return nil, fmt.Errorf("dial unix %q: %w", addr, err)
	}
	return conn, nil
}

// Owns the listener, the exclusive lock file, and the socket path so Close can
// unlink the socket and release the lock.
type unixListener struct {
	ln   *net.UnixListener
	lock *os.File
	path string
	once sync.Once
	err  error
}

func (l *unixListener) Accept() (Connection, error) {
	conn, err := l.ln.AcceptUnix()
	if err != nil {
		return nil, fmt.Errorf("accept unix %q: %w", l.path, err)
	}
	return &unixConn{UnixConn: conn}, nil
}

// Idempotent; stops accepting, removes the socket path, and releases the lock.
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

// One accepted Unix-socket connection with its kernel-reported peer identity.
type unixConn struct {
	*net.UnixConn
}

func (c *unixConn) PeerIdentity() (PeerIdentity, error) {
	return peerIdentity(c.UnixConn)
}

var (
	_ Connection = (*unixConn)(nil)
	_ Listener   = (*unixListener)(nil)
	_ ListenFunc = ListenUnix
	_ DialFunc   = DialUnix
)
