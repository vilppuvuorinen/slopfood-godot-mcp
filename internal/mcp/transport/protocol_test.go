package transport_test

import (
	"runtime"
	"testing"

	"github.com/vilppuvuorinen/slopfood-godot-mcp/internal/mcp/transport"
)

func TestResolveProtocol(t *testing.T) {
	wantAuto := transport.ProtocolUDS
	if runtime.GOOS == "windows" {
		wantAuto = transport.ProtocolTCP
	}
	if got := transport.ResolveProtocol(transport.ProtocolAuto); got != wantAuto {
		t.Errorf("ResolveProtocol(auto) = %q, want %q", got, wantAuto)
	}
	if got := transport.ResolveProtocol(transport.ProtocolTCP); got != transport.ProtocolTCP {
		t.Errorf("ResolveProtocol(tcp) = %q, want tcp", got)
	}
	if got := transport.ResolveProtocol(transport.ProtocolUDS); got != transport.ProtocolUDS {
		t.Errorf("ResolveProtocol(uds) = %q, want uds", got)
	}
}
