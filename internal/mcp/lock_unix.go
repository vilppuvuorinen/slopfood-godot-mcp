//go:build !windows

package mcp

import (
	"fmt"
	"io"
	"os"
	"syscall"
)

// OpenTCPLock takes a non-blocking exclusive flock on path for the TCP
// endpoint's whole life. It is the Linux/Unix analogue of the Windows
// zero-share-mode handle: the OS releases the lock exactly when the process
// dies, so it is both the single-instance rule and the crash cleanup. This
// branch exists so the dev-build transport override can exercise the TCP
// endpoint locally; only the Windows branch ships.
func OpenTCPLock(path string) (io.Closer, error) {
	if err := ensureEndpointDir(path); err != nil {
		return nil, err
	}
	//nolint:gosec // the lock path is derived from the bound project, not user input.
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open tcp lock %q: %w", path, err)
	}
	if err := file.Chmod(0o600); err != nil {
		_ = file.Close()
		return nil, fmt.Errorf("secure tcp lock %q: %w", path, err)
	}
	//nolint:gosec // the fd comes from a live *os.File.
	if err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		_ = file.Close()
		return nil, fmt.Errorf("lock tcp %q: another daemon owns the endpoint: %w", path, err)
	}
	return file, nil
}
