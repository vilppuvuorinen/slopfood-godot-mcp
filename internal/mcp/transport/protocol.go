package transport

import "runtime"

// Protocol selects the daemon<->client transport. It is deliberately separate
// from the engine IPC transport so the two channels cannot be conflated.
type Protocol string

const (
	ProtocolAuto Protocol = "auto"
	ProtocolUDS  Protocol = "uds"
	ProtocolTCP  Protocol = "tcp"
)

// ResolveProtocol resolves auto to the platform default (TCP on Windows, which
// exposes no Unix-socket classes, and UDS elsewhere); explicit values pass
// through.
func ResolveProtocol(p Protocol) Protocol {
	switch p {
	case ProtocolUDS:
		return ProtocolUDS
	case ProtocolTCP:
		return ProtocolTCP
	case ProtocolAuto:
	}
	if runtime.GOOS == "windows" {
		return ProtocolTCP
	}
	return ProtocolUDS
}
