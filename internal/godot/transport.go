package godot

import "runtime"

// EngineTransport selects the engine IPC channel. It is kept separate from the
// daemon<->client transport in internal/mcp/transport.
//
// EngineTransportAuto resolves to UDS off Windows and TCP on Windows, which
// exposes no Unix-socket classes.
type EngineTransport string

const (
	EngineTransportAuto EngineTransport = "auto"
	EngineTransportUDS  EngineTransport = "uds"
	EngineTransportTCP  EngineTransport = "tcp"
)

// An explicit choice is returned as-is so a dev override can force either transport; an unrecognized
// value resolves like auto.
func ResolveTransport(explicit EngineTransport) EngineTransport {
	switch explicit {
	case EngineTransportUDS:
		return EngineTransportUDS
	case EngineTransportTCP:
		return EngineTransportTCP
	case EngineTransportAuto:
	}
	if runtime.GOOS == "windows" {
		return EngineTransportTCP
	}
	return EngineTransportUDS
}
