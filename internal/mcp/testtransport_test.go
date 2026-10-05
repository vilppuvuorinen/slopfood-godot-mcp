//go:build !windows

package mcp

import (
	"io"
	"testing"
)

// newTestEndpoint binds the production Unix-socket transport, the default on
// this platform, and returns its listen and dial functions.
func newTestEndpoint(t *testing.T) (ListenFunc, DialFunc) {
	t.Helper()
	sock := shortSocketPath(t)
	l, err := ListenUnix(sock)
	if err != nil {
		t.Fatalf("ListenUnix: %v", err)
	}
	t.Cleanup(func() { _ = l.Close() })
	dial := func(string) (io.ReadWriteCloser, error) { return DialUnix(sock) }
	return fixedListen(l), dial
}
