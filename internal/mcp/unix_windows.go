//go:build windows

package mcp

import (
	"errors"
	"io"
)

// errUnixUnsupported reports that the Unix-socket backend is unavailable on
// Windows. The design's named-pipe backend would fill the same seam later.
var errUnixUnsupported = errors.New("unix sockets are unsupported on windows")

// ListenUnix is the Windows stub for the Unix-socket listener. It always fails;
// a named-pipe backend is what a Windows user would need.
func ListenUnix(string) (Listener, error) {
	return nil, errUnixUnsupported
}

// DialUnix is the Windows stub for the Unix-socket dialer. It always fails for
// the same reason ListenUnix does.
func DialUnix(string) (io.ReadWriteCloser, error) {
	return nil, errUnixUnsupported
}

var (
	_ ListenFunc = ListenUnix
	_ DialFunc   = DialUnix
)
