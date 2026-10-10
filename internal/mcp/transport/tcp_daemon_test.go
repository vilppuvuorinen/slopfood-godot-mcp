package transport_test

import (
	"bufio"
	"context"
	"io"
	"net"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/vilppuvuorinen/slopfood-godot-mcp/internal/mcp/daemon"
	"github.com/vilppuvuorinen/slopfood-godot-mcp/internal/mcp/transport"
)

func TestTCPServesProtocol(t *testing.T) {
	listener, endpointPath := newTCPEndpointForTest(t)
	d := daemon.NewDaemon(testServer(), nil, func(string) (transport.Listener, error) { return listener, nil }, "tcp")
	done := make(chan error, 1)
	go func() { done <- d.Serve(context.Background()) }()

	client, err := transport.DialTCP(endpointPath)
	if err != nil {
		t.Fatalf("DialTCP: %v", err)
	}
	defer func() { _ = client.Close() }()

	if _, writeErr := io.WriteString(client, `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`+"\n"); writeErr != nil {
		t.Fatalf("write request: %v", writeErr)
	}
	line, readErr := bufio.NewReader(client).ReadString('\n')
	if readErr != nil {
		t.Fatalf("read response: %v", readErr)
	}
	if !strings.Contains(line, `"tools"`) {
		t.Errorf("response = %s", line)
	}

	stopDaemon(t, d, done)
}

func TestTCPTokenRejectClosesWithoutServing(t *testing.T) {
	listener, endpointPath := newTCPEndpointForTest(t)
	d := daemon.NewDaemon(testServer(), nil, func(string) (transport.Listener, error) { return listener, nil }, "tcp")
	done := make(chan error, 1)
	go func() { done <- d.Serve(context.Background()) }()

	host, port := readTCPEndpoint(t, endpointPath)
	address := host + ":" + strconv.Itoa(port)
	var dialer net.Dialer
	conn, err := dialer.DialContext(context.Background(), "tcp", address)
	if err != nil {
		t.Fatalf("raw dial: %v", err)
	}
	defer func() { _ = conn.Close() }()

	if _, err := conn.Write([]byte(`{"token":"wrong"}` + "\n")); err != nil {
		t.Fatalf("write token: %v", err)
	}
	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, readErr := bufio.NewReader(conn).ReadString('\n')
	if readErr == nil {
		t.Fatal("daemon answered a wrongly-tokened connection, want a close")
	}

	stopDaemon(t, d, done)
}

func TestTCPMissingTokenRejected(t *testing.T) {
	listener, endpointPath := newTCPEndpointForTest(t)
	d := daemon.NewDaemon(testServer(), nil, func(string) (transport.Listener, error) { return listener, nil }, "tcp")
	done := make(chan error, 1)
	go func() { done <- d.Serve(context.Background()) }()

	host, port := readTCPEndpoint(t, endpointPath)
	var dialer net.Dialer
	conn, err := dialer.DialContext(context.Background(), "tcp", host+":"+strconv.Itoa(port))
	if err != nil {
		t.Fatalf("raw dial: %v", err)
	}
	defer func() { _ = conn.Close() }()

	if _, err := conn.Write([]byte("not json\n")); err != nil {
		t.Fatalf("write prelude: %v", err)
	}
	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	if _, readErr := bufio.NewReader(conn).ReadString('\n'); readErr == nil {
		t.Fatal("daemon answered a malformed prelude, want a close")
	}

	stopDaemon(t, d, done)
}
