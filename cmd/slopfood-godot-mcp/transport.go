package main

import (
	"fmt"
	"runtime"

	"github.com/vilppuvuorinen/slopfood-godot-mcp/internal/godot"
)

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
