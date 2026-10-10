//go:build !windows

package transport

import (
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"sync"

	"github.com/vilppuvuorinen/slopfood-godot-mcp/internal/fsutil"
)

// Creates the socket directory 0700, takes an exclusive flock on addr+".lock"
// for the listener's life, reclaims a stale socket, and creates the socket 0600.
func ListenUnix(addr string) (Listener, error) {
	dir := filepath.Dir(addr)
	if err := fsutil.EnsurePrivateDir(dir); err != nil {
		return nil, fmt.Errorf("prepare socket directory %q: %w", dir, err)
	}

	lock, err := OpenListenerLock(addr + ".lock")
	if err != nil {
		return nil, err
	}

	// The lock says we are the only holder, so a live listener means a duplicate
	// bind from this process; refuse without unlinking, which would strand it.
	if existing, dialErr := net.Dial("unix", addr); dialErr == nil { //nolint:noctx // the dial is a local liveness probe with no context to carry.
		_ = existing.Close()
		_ = lock.Close()
		return nil, fmt.Errorf("listen unix %q: already listening", addr)
	}
	if removeErr := fsutil.RemoveIfExists(addr); removeErr != nil {
		_ = lock.Close()
		return nil, fmt.Errorf("remove stale socket: %w", removeErr)
	}
	ln, err := net.Listen("unix", addr) //nolint:noctx // binding a local socket is fast and carries no context.
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
	lock io.Closer
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
		if err := fsutil.RemoveIfExists(l.path); err != nil {
			errs = append(errs, fmt.Errorf("remove socket: %w", err))
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
	_ DialFunc   = DialUnix
)
