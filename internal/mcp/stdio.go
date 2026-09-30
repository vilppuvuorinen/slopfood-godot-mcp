package mcp

import "io"

// Adapts a reader and writer to the Connection seam. Stdio has no
// kernel-attested peer, so it reports the current process.
type StdioConn struct {
	reader io.Reader
	writer io.Writer
}

func NewStdioConn(r io.Reader, w io.Writer) *StdioConn {
	return &StdioConn{reader: r, writer: w}
}

func (c *StdioConn) Read(p []byte) (int, error) {
	return c.reader.Read(p) //nolint:wrapcheck // a stream adapter passes io errors through unchanged.
}

func (c *StdioConn) Write(p []byte) (int, error) {
	return c.writer.Write(p) //nolint:wrapcheck // a stream adapter passes io errors through unchanged.
}

// No-op: the standard streams belong to the process, so shutdown must not close them.
func (c *StdioConn) Close() error { return nil }

func (c *StdioConn) PeerIdentity() (PeerIdentity, error) {
	return currentPeerIdentity(), nil
}

var _ Connection = (*StdioConn)(nil)
