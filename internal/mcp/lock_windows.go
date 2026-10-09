//go:build windows

package mcp

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"syscall"

	"github.com/vilppuvuorinen/slopfood-godot-mcp/internal/fsutil"
)

// Opens path with exclusive share mode so only the first opener wins; the OS releases it when the process dies.
func OpenListenerLock(path string) (io.Closer, error) {
	if err := fsutil.EnsurePrivateDir(filepath.Dir(path)); err != nil {
		return nil, fmt.Errorf("prepare lock directory for %q: %w", path, err)
	}
	pathPtr, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return nil, fmt.Errorf("encode tcp lock path %q: %w", path, err)
	}
	handle, err := syscall.CreateFile(
		pathPtr,
		syscall.GENERIC_READ|syscall.GENERIC_WRITE,
		0, // exclusive: no other process may open it while held.
		nil,
		syscall.OPEN_ALWAYS,
		syscall.FILE_ATTRIBUTE_NORMAL,
		0,
	)
	if err != nil {
		return nil, fmt.Errorf("lock %q: another daemon owns it: %w", path, err)
	}
	return os.NewFile(uintptr(handle), path), nil
}
