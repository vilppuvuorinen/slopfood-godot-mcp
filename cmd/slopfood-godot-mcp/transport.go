package main

import (
	"fmt"
	"runtime"

	"github.com/vilppuvuorinen/slopfood-godot-mcp/internal/godot"
)

// parseTransport validates a --transport value and maps it to a godot.Transport.
// "auto" resolves to the platform default; the explicit forms are rejected only
// when the platform cannot use them, so a dev build cannot force a transport
// the release binary would never select.
func parseTransport(value string) (godot.Transport, error) {
	switch value {
	case "auto", "":
		return godot.TransportAuto, nil
	case "uds":
		if runtime.GOOS == "windows" {
			return "", fmt.Errorf("--transport uds is unavailable on Windows: Godot exposes no Unix-socket classes there")
		}
		return godot.TransportUDS, nil
	case "tcp":
		return godot.TransportTCP, nil
	default:
		return "", fmt.Errorf("invalid --transport %q: want auto, uds, or tcp", value)
	}
}

// resolveTransport maps an explicit transport to the one this platform uses,
// resolving auto to UDS off Windows and TCP on Windows. It mirrors the engine
// session's own resolution so the daemon endpoint and the bridge agree.
func resolveTransport(explicit godot.Transport) godot.Transport {
	switch explicit {
	case godot.TransportUDS:
		return godot.TransportUDS
	case godot.TransportTCP:
		return godot.TransportTCP
	case godot.TransportAuto:
	}
	if runtime.GOOS == "windows" {
		return godot.TransportTCP
	}
	return godot.TransportUDS
}
