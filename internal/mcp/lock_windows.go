//go:build windows

package mcp

import (
	"fmt"
	"io"
	"os"
	"syscall"
)

// Opens path with exclusive share mode so only the first opener wins; the OS releases it when the process dies.
func OpenTCPLock(path string) (io.Closer, error) {
	if err := ensureEndpointDir(path); err != nil {
		return nil, err
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
		return nil, fmt.Errorf("lock tcp %q: another daemon owns the endpoint: %w", path, err)
	}
	return os.NewFile(uintptr(handle), path), nil
}
