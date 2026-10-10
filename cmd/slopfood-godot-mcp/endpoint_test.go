package main

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/vilppuvuorinen/slopfood-godot-mcp/internal/godot"
)

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

func TestResolveTransportAuto(t *testing.T) {
	want := godot.TransportUDS
	if runtime.GOOS == "windows" {
		want = godot.TransportTCP
	}
	if got := godot.ResolveTransport(godot.TransportAuto); got != want {
		t.Errorf("ResolveTransport(auto) = %q, want %q", got, want)
	}
	if got := godot.ResolveTransport(godot.TransportTCP); got != godot.TransportTCP {
		t.Errorf("ResolveTransport(tcp) = %q, want tcp", got)
	}
	if runtime.GOOS != "windows" {
		if got := godot.ResolveTransport(godot.TransportUDS); got != godot.TransportUDS {
			t.Errorf("ResolveTransport(uds) = %q, want uds", got)
		}
	}
}

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

	listener, err := endpoint.listen()
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer func() { _ = listener.Close() }()

	if _, statErr := os.Stat(endpoint.addr()); statErr != nil {
		t.Errorf("endpoint record missing after bind: %v", statErr)
	}

	second, err := newDaemonEndpoint("/tmp/project", godot.TransportTCP)
	if err != nil {
		t.Fatalf("second newDaemonEndpoint: %v", err)
	}
	if _, err := second.listen(); err == nil {
		t.Fatal("second TCP bind succeeded, want the exclusive lock to reject it")
	}
}

func TestEndpointFactoryTCPCloseReleasesEndpoint(t *testing.T) {
	t.Setenv("XDG_RUNTIME_DIR", t.TempDir())

	endpoint, err := newDaemonEndpoint("/tmp/project", godot.TransportTCP)
	if err != nil {
		t.Fatalf("newDaemonEndpoint: %v", err)
	}
	if _, err := endpoint.listen(); err != nil {
		t.Fatalf("listen: %v", err)
	}
	if _, statErr := os.Stat(endpoint.addr()); statErr != nil {
		t.Fatalf("endpoint record missing after bind: %v", statErr)
	}
	endpoint.close()
	if _, statErr := os.Stat(endpoint.addr()); !os.IsNotExist(statErr) {
		t.Errorf("endpoint record still present after close: %v", statErr)
	}
}

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

func containsFlag(args []string, name string) bool {
	for _, arg := range args {
		if arg == name {
			return true
		}
	}
	return false
}
