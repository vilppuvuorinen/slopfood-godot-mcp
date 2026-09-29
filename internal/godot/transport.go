package godot

import "runtime"

// Transport selects the engine bridge's wire transport. The zero value is
// TransportAuto, which resolves to UDS off Windows and TCP on Windows, where
// Godot exposes no Unix-socket classes. A dev build can force either transport
// through the daemon's --transport override; a release build always resolves
// auto and never exposes the switch.
type Transport string

const (
	// TransportAuto picks the platform default: UDS off Windows, TCP on Windows.
	TransportAuto Transport = "auto"
	// TransportUDS forces the Unix-domain-socket transport.
	TransportUDS Transport = "uds"
	// TransportTCP forces the loopback-TCP transport.
	TransportTCP Transport = "tcp"
)

// resolveTransport maps an explicit transport to one this build can use. Auto
// resolves by GOOS; an explicit choice is returned as-is so the D3 override can
// force TCP on Linux for local testing. An unrecognized value resolves like
// auto, mirroring the flag layer's validation.
func resolveTransport(explicit Transport) Transport {
	switch explicit {
	case TransportUDS:
		return TransportUDS
	case TransportTCP:
		return TransportTCP
	case TransportAuto:
	}
	if runtime.GOOS == "windows" {
		return TransportTCP
	}
	return TransportUDS
}
