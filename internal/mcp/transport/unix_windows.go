//go:build windows

package transport

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
