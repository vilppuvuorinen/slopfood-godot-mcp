package client

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"

	"github.com/vilppuvuorinen/slopfood-godot-mcp/internal/mcp"
	"github.com/vilppuvuorinen/slopfood-godot-mcp/internal/mcp/transport"
)

func testServer() *mcp.Server {
	s := mcp.NewServer(mcp.ServerInfo{Name: "test", Version: "0.0.0"}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	s.Instructions = "test instructions"
	s.RegisterTool(mcp.Tool{
		Name:         "echo",
		Title:        "Echo",
		Description:  "echo",
		InputSchema:  map[string]any{"type": "object"},
		OutputSchema: map[string]any{"type": "object"},
		Annotations:  &mcp.ToolAnnotations{ReadOnlyHint: mcp.Bool(true)},
		Handler: func(_ context.Context, args json.RawMessage) (any, error) {
			return string(args), nil
		},
	})
	return s
}

// In-memory ReadWriteCloser returned by the test dialer; the client tests never
// read or write through it.
type fakeConn struct {
	in     io.Reader
	out    bytes.Buffer
	closed bool
}

func (c *fakeConn) Read(p []byte) (int, error) {
	return c.in.Read(p) //nolint:wrapcheck // a fake must pass io.EOF and read errors through unchanged.
}

func (c *fakeConn) Write(p []byte) (int, error) {
	return c.out.Write(p) //nolint:wrapcheck // a fake must pass write errors through unchanged.
}

func (c *fakeConn) Close() error { c.closed = true; return nil }

// fixedListen hands a pre-bound listener to a consumer that ignores the address.
func fixedListen(l transport.Listener) transport.ListenFunc {
	return func(string) (transport.Listener, error) { return l, nil }
}
