package main

import (
	"fmt"
	"io"

	"github.com/vilppuvuorinen/slopfood-godot-mcp/internal/daemonpath"
	"github.com/vilppuvuorinen/slopfood-godot-mcp/internal/godot"
	"github.com/vilppuvuorinen/slopfood-godot-mcp/internal/mcp"
)

type daemonEndpoint struct {
	transport    godot.Transport
	endpointPath string
	socketPath   string
	// tcpListener is the pre-bound TCP listener, held so teardown before daemon construction closes it; nil for UDS.
	tcpListener *mcp.TCPListener
}

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

func (e *daemonEndpoint) addr() string {
	if e.transport == godot.TransportTCP {
		return e.endpointPath
	}
	return e.socketPath
}

// The TCP dialer re-reads the endpoint file each call to pick up a fresh daemon's port and token.
func (e *daemonEndpoint) dialFunc() mcp.DialFunc {
	if e.transport == godot.TransportTCP {
		return func(string) (io.ReadWriteCloser, error) { return mcp.DialTCP(e.endpointPath) }
	}
	return mcp.DialUnix
}

// The TCP endpoint binds before the daemon is constructed so the record is published before any accept.
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

func (e *daemonEndpoint) close() {
	if e.tcpListener != nil {
		_ = e.tcpListener.Close()
	}
}
