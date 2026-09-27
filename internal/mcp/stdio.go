package mcp

import "io"

// StdioConn adapts a reader and a writer, normally os.Stdin and os.Stdout, to
// the Connection seam. It lets the stdio path be one connection to the daemon
// core rather than a special case: the core runs the same Serve loop on it as
// on an accepted socket. A stdio stream has no kernel-attested peer, so it
// reports the current process, which is honest: the client on the other side
// of stdio is this process.
type StdioConn struct {
	reader io.Reader
	writer io.Writer
}

// NewStdioConn returns a Connection backed by r and w.
func NewStdioConn(r io.Reader, w io.Writer) *StdioConn {
	return &StdioConn{reader: r, writer: w}
}

func (c *StdioConn) Read(p []byte) (int, error) {
	return c.reader.Read(p) //nolint:wrapcheck // a stream adapter passes io errors through unchanged.
}

func (c *StdioConn) Write(p []byte) (int, error) {
	return c.writer.Write(p) //nolint:wrapcheck // a stream adapter passes io errors through unchanged.
}

// Close is a no-op: the standard streams belong to the process, not to the
// connection, so a daemon shutdown must not close them.
func (c *StdioConn) Close() error { return nil }

// PeerIdentity reports the current process as the peer, the same identity the
// in-process transport claims. Stdio exposes no kernel peer credentials.
func (c *StdioConn) PeerIdentity() (PeerIdentity, error) {
	return currentPeerIdentity(), nil
}

var _ Connection = (*StdioConn)(nil)
