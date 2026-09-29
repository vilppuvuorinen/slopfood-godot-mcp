package main

import (
	"fmt"
	"io"

	"github.com/vilppuvuorinen/slopfood-godot-mcp/internal/daemonpath"
	"github.com/vilppuvuorinen/slopfood-godot-mcp/internal/godot"
	"github.com/vilppuvuorinen/slopfood-godot-mcp/internal/mcp"
)

// daemonEndpoint bundles the transport seams both roles use: the listener the
// daemon accepts on, the dialer the client connects through, and the opaque
// address the core carries. It is selected at runtime by the effective
// transport, so the rest of the code is transport-agnostic; the only OS
// branches are what auto resolves to and the lock helper.
type daemonEndpoint struct {
	transport    godot.Transport
	endpointPath string
	socketPath   string
	// tcpListener is the pre-bound listener for the TCP transport, held so a
	// teardown before the daemon is constructed still closes it. It is nil for
	// UDS, whose listener is owned entirely by ListenUnix inside Serve.
	tcpListener *mcp.TCPListener
}

// newDaemonEndpoint resolves the effective transport and derives the paths it
// needs. It does not bind: the daemon binds through listenFunc, and the client
// only dials.
func newDaemonEndpoint(root string, explicit godot.Transport) (*daemonEndpoint, error) {
	transport := resolveTransport(explicit)
	endpoint := &daemonEndpoint{transport: transport}
	switch transport {
	case godot.TransportTCP:
		path, err := daemonpath.EndpointPath(root)
		if err != nil {
			return nil, fmt.Errorf("resolve tcp endpoint path: %w", err)
		}
		endpoint.endpointPath = path
	case godot.TransportUDS, godot.TransportAuto:
		path, err := daemonpath.SocketPath(root)
		if err != nil {
			return nil, fmt.Errorf("resolve daemon socket path: %w", err)
		}
		endpoint.socketPath = path
	}
	return endpoint, nil
}

// addr is the opaque address the daemon core carries. The UDS backend resolves
// its socket path from it; the TCP backend ignores it and dials through the
// endpoint file instead.
func (e *daemonEndpoint) addr() string {
	if e.transport == godot.TransportTCP {
		return e.endpointPath
	}
	return e.socketPath
}

// dialFunc returns the client half of the seam. The TCP dialer reads the
// endpoint file each call, so it picks up a freshly spawned daemon's port and
// token; the UDS dialer is the plain Unix dial.
func (e *daemonEndpoint) dialFunc() mcp.DialFunc {
	if e.transport == godot.TransportTCP {
		return func(string) (io.ReadWriteCloser, error) { return mcp.DialTCP(e.endpointPath) }
	}
	return mcp.DialUnix
}

// listenFunc returns the daemon half of the seam. The TCP endpoint opens its
// exclusive lock and binds 127.0.0.1:0 now, before the daemon is constructed,
// so the endpoint record is published before any accept; the closure hands the
// daemon that one pre-bound listener. The UDS endpoint binds lazily through
// ListenUnix, which owns its own directory, lock, and stale-socket reclaim.
func (e *daemonEndpoint) listenFunc() (mcp.ListenFunc, error) {
	if e.transport != godot.TransportTCP {
		return mcp.ListenUnix, nil
	}
	lock, err := mcp.OpenTCPLock(e.endpointPath + ".lock")
	if err != nil {
		return nil, fmt.Errorf("open tcp endpoint lock: %w", err)
	}
	listener, err := mcp.ListenTCP(e.addr(), e.endpointPath, lock)
	if err != nil {
		_ = lock.Close()
		return nil, fmt.Errorf("bind tcp endpoint: %w", err)
	}
	e.tcpListener = listener
	return func(string) (mcp.Listener, error) { return listener, nil }, nil
}

// close releases the TCP listener if one was bound, so an early return between
// binding and daemon construction does not leak the endpoint file and lock. The
// daemon's own Close also closes the listener; TCPListener.Close is idempotent,
// so both paths are safe.
func (e *daemonEndpoint) close() {
	if e.tcpListener != nil {
		_ = e.tcpListener.Close()
	}
}
