package main

import (
	"fmt"
	"io"

	"github.com/vilppuvuorinen/slopfood-godot-mcp/internal/daemonpath"
	"github.com/vilppuvuorinen/slopfood-godot-mcp/internal/mcp/client"
	"github.com/vilppuvuorinen/slopfood-godot-mcp/internal/mcp/transport"
)

type daemonEndpoint struct {
	transport    transport.Protocol
	endpointPath string
	socketPath   string
	// listener is the pre-bound listener, held so teardown before daemon construction closes it.
	listener transport.Listener
}

func newDaemonEndpoint(root string, explicit transport.Protocol) (*daemonEndpoint, error) {
	proto := transport.ResolveProtocol(explicit)
	endpoint := &daemonEndpoint{transport: proto}
	switch proto {
	case transport.ProtocolTCP:
		path, err := daemonpath.EndpointPath(root)
		if err != nil {
			return nil, fmt.Errorf("resolve tcp endpoint path: %w", err)
		}
		endpoint.endpointPath = path
	case transport.ProtocolUDS, transport.ProtocolAuto:
		path, err := daemonpath.SocketPath(root)
		if err != nil {
			return nil, fmt.Errorf("resolve daemon socket path: %w", err)
		}
		endpoint.socketPath = path
	}
	return endpoint, nil
}

func (e *daemonEndpoint) addr() string {
	if e.transport == transport.ProtocolTCP {
		return e.endpointPath
	}
	return e.socketPath
}

// The TCP dialer re-reads the endpoint file each call to pick up a fresh daemon's port and token.
func (e *daemonEndpoint) dialFunc() client.Dialer {
	if e.transport == transport.ProtocolTCP {
		return func() (io.ReadWriteCloser, error) { return transport.DialTCP(e.endpointPath) }
	}
	return func() (io.ReadWriteCloser, error) { return transport.DialUnix(e.socketPath) }
}

// Binds the endpoint before the daemon is constructed so the record is published before any accept.
func (e *daemonEndpoint) listen() (transport.Listener, error) {
	if e.transport != transport.ProtocolTCP {
		listener, err := transport.ListenUnix(e.socketPath)
		if err != nil {
			return nil, fmt.Errorf("bind unix endpoint: %w", err)
		}
		e.listener = listener
		return listener, nil
	}
	lock, err := transport.OpenListenerLock(e.endpointPath + ".lock")
	if err != nil {
		return nil, fmt.Errorf("open tcp endpoint lock: %w", err)
	}
	listener, err := transport.ListenTCP(e.addr(), e.endpointPath, lock)
	if err != nil {
		_ = lock.Close()
		return nil, fmt.Errorf("bind tcp endpoint: %w", err)
	}
	e.listener = listener
	return listener, nil
}

func (e *daemonEndpoint) close() {
	if e.listener != nil {
		_ = e.listener.Close()
	}
}
