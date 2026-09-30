package godot

import "runtime"

// TransportAuto resolves to UDS off Windows and TCP on Windows, which exposes no Unix-socket classes.
type Transport string

const (
	TransportAuto Transport = "auto"
	TransportUDS  Transport = "uds"
	TransportTCP  Transport = "tcp"
)

// An explicit choice is returned as-is so a dev override can force either transport; an unrecognized
// value resolves like auto.
func ResolveTransport(explicit Transport) Transport {
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
