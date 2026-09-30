//go:build windows

package mcp

import (
	"errors"
	"io"
)

var errUnixUnsupported = errors.New("unix sockets are unsupported on windows")

func ListenUnix(string) (Listener, error) {
	return nil, errUnixUnsupported
}

func DialUnix(string) (io.ReadWriteCloser, error) {
	return nil, errUnixUnsupported
}

var (
	_ ListenFunc = ListenUnix
	_ DialFunc   = DialUnix
)
