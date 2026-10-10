package main

import (
	"fmt"
	"runtime"

	"github.com/vilppuvuorinen/slopfood-godot-mcp/internal/godot"
	"github.com/vilppuvuorinen/slopfood-godot-mcp/internal/mcp/transport"
)

// transports carries both sides of a --transport value: the engine IPC enum and
// the daemon<->client protocol.
type transports struct {
	engine godot.EngineTransport
	daemon transport.Protocol
}

func parseTransport(value string) (transports, error) {
	switch value {
	case "auto", "":
		return transports{engine: godot.EngineTransportAuto, daemon: transport.ProtocolAuto}, nil
	case "uds":
		if runtime.GOOS == "windows" {
			return transports{}, fmt.Errorf("--transport uds is unavailable on Windows: Godot exposes no Unix-socket classes there")
		}
		return transports{engine: godot.EngineTransportUDS, daemon: transport.ProtocolUDS}, nil
	case "tcp":
		return transports{engine: godot.EngineTransportTCP, daemon: transport.ProtocolTCP}, nil
	default:
		return transports{}, fmt.Errorf("invalid --transport %q: want auto, uds, or tcp", value)
	}
}
