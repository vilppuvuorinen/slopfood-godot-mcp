package main

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/vilppuvuorinen/slopfood-godot-mcp/internal/godot"
)

// TestParseTransport asserts the flag value maps to a transport and that a
// platform-incompatible explicit choice and an unknown value are rejected.
func TestParseTransport(t *testing.T) {
	cases := []struct {
		value   string
		want    godot.Transport
		wantErr bool
	}{
		{"", godot.TransportAuto, false},
		{"auto", godot.TransportAuto, false},
		{"tcp", godot.TransportTCP, false},
		{"uds", godot.TransportUDS, runtime.GOOS == "windows"},
		{"pipe", "", true},
	}
	for _, tc := range cases {
		got, err := parseTransport(tc.value)
		if tc.wantErr {
			if err == nil {
				t.Errorf("parseTransport(%q) = %q, want an error", tc.value, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("parseTransport(%q): %v", tc.value, err)
			continue
		}
		if got != tc.want {
			t.Errorf("parseTransport(%q) = %q, want %q", tc.value, got, tc.want)
		}
	}
}

// TestResolveTransportAuto asserts auto resolves to UDS off Windows and TCP on
// Windows, and an explicit choice is preserved.
func TestResolveTransportAuto(t *testing.T) {
	want := godot.TransportUDS
	if runtime.GOOS == "windows" {
		want = godot.TransportTCP
	}
	if got := resolveTransport(godot.TransportAuto); got != want {
		t.Errorf("resolveTransport(auto) = %q, want %q", got, want)
	}
	if got := resolveTransport(godot.TransportTCP); got != godot.TransportTCP {
		t.Errorf("resolveTransport(tcp) = %q, want tcp", got)
	}
	if runtime.GOOS != "windows" {
		if got := resolveTransport(godot.TransportUDS); got != godot.TransportUDS {
			t.Errorf("resolveTransport(uds) = %q, want uds", got)
		}
	}
}

// TestEndpointFactoryUDS asserts auto on a non-Windows host resolves to the UDS
// endpoint, whose address is the derived socket path.
func TestEndpointFactoryUDS(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("auto resolves to TCP on Windows")
	}
	t.Setenv("XDG_RUNTIME_DIR", t.TempDir())

	endpoint, err := newDaemonEndpoint("/tmp/project", godot.TransportAuto)
	if err != nil {
		t.Fatalf("newDaemonEndpoint: %v", err)
	}
	if endpoint.transport != godot.TransportUDS {
		t.Fatalf("transport = %q, want uds", endpoint.transport)
	}
	if got := filepath.Ext(endpoint.addr()); got != ".sock" {
		t.Errorf("addr = %q, want a .sock path", endpoint.addr())
	}
}

// TestEndpointFactoryTCP asserts the forced TCP endpoint binds before the
// daemon, publishes its endpoint record, and loses a second bind to the
// exclusive lock.
func TestEndpointFactoryTCP(t *testing.T) {
	t.Setenv("XDG_RUNTIME_DIR", t.TempDir())

	endpoint, err := newDaemonEndpoint("/tmp/project", godot.TransportTCP)
	if err != nil {
		t.Fatalf("newDaemonEndpoint: %v", err)
	}
	if endpoint.transport != godot.TransportTCP {
		t.Fatalf("transport = %q, want tcp", endpoint.transport)
	}
	if got := filepath.Ext(endpoint.addr()); got != ".endpoint" {
		t.Errorf("addr = %q, want a .endpoint path", endpoint.addr())
	}

	listen, err := endpoint.listenFunc()
	if err != nil {
		t.Fatalf("listenFunc: %v", err)
	}
	listener, err := listen(endpoint.addr())
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer func() { _ = listener.Close() }()

	if _, statErr := os.Stat(endpoint.addr()); statErr != nil {
		t.Errorf("endpoint record missing after bind: %v", statErr)
	}

	// A second endpoint for the same project loses to the lock.
	second, err := newDaemonEndpoint("/tmp/project", godot.TransportTCP)
	if err != nil {
		t.Fatalf("second newDaemonEndpoint: %v", err)
	}
	if _, err := second.listenFunc(); err == nil {
		t.Fatal("second TCP bind succeeded, want the exclusive lock to reject it")
	}
}

// TestEndpointFactoryTCPCloseReleasesEndpoint asserts endpoint.close removes
// the endpoint record after listenFunc binds, so an early return between bind
// and daemon construction does not leak it.
func TestEndpointFactoryTCPCloseReleasesEndpoint(t *testing.T) {
	t.Setenv("XDG_RUNTIME_DIR", t.TempDir())

	endpoint, err := newDaemonEndpoint("/tmp/project", godot.TransportTCP)
	if err != nil {
		t.Fatalf("newDaemonEndpoint: %v", err)
	}
	if _, err := endpoint.listenFunc(); err != nil {
		t.Fatalf("listenFunc: %v", err)
	}
	if _, statErr := os.Stat(endpoint.addr()); statErr != nil {
		t.Fatalf("endpoint record missing after bind: %v", statErr)
	}
	endpoint.close()
	if _, statErr := os.Stat(endpoint.addr()); !os.IsNotExist(statErr) {
		t.Errorf("endpoint record still present after close: %v", statErr)
	}
}

// TestDaemonChildArgsForwardsTransport asserts --transport is forwarded only
// when it was actually set, so a release child never sees an unknown flag.
func TestDaemonChildArgsForwardsTransport(t *testing.T) {
	base := &options{idleTimeout: 0}
	if got := daemonChildArgs("/p", base, "/p.log"); containsFlag(got, "--transport") {
		t.Errorf("unset transport was forwarded: %q", got)
	}
	set := &options{transport: "tcp", transportSet: true}
	if got := daemonChildArgs("/p", set, "/p.log"); !containsFlag(got, "--transport") {
		t.Errorf("set transport was not forwarded: %q", got)
	}
}

// containsFlag reports whether args contain the named flag.
func containsFlag(args []string, name string) bool {
	for _, arg := range args {
		if arg == name {
			return true
		}
	}
	return false
}
