package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func newTCPListenerForTest(t *testing.T) *TCPListener {
	t.Helper()
	endpointPath := filepath.Join(t.TempDir(), "runtime", "proj.endpoint")
	lock, err := OpenTCPLock(endpointPath + ".lock")
	if err != nil {
		t.Fatalf("OpenTCPLock: %v", err)
	}
	listener, err := ListenTCP("", endpointPath, lock)
	if err != nil {
		_ = lock.Close()
		t.Fatalf("ListenTCP: %v", err)
	}
	t.Cleanup(func() { _ = listener.Close() })
	return listener
}

func TestTCPEndpointRecordAndLock(t *testing.T) {
	listener := newTCPListenerForTest(t)
	if listener.Port() == 0 {
		t.Fatal("bound port = 0, want an ephemeral port")
	}

	info, err := os.Stat(listener.endpointPath)
	if err != nil {
		t.Fatalf("stat endpoint: %v", err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Errorf("endpoint mode = %o, want 600", got)
	}
	dirInfo, err := os.Stat(filepath.Dir(listener.endpointPath))
	if err != nil {
		t.Fatalf("stat endpoint dir: %v", err)
	}
	if got := dirInfo.Mode().Perm(); got != 0o700 {
		t.Errorf("endpoint dir mode = %o, want 700", got)
	}

	record, err := readEndpoint(listener.endpointPath)
	if err != nil {
		t.Fatalf("readEndpoint: %v", err)
	}
	if record.Transport != "tcp" || record.Host != "127.0.0.1" || record.Port == 0 || record.Token == "" {
		t.Errorf("record = %+v, want a populated tcp record", record)
	}

	lock, err := OpenTCPLock(listener.endpointPath + ".lock")
	if err == nil {
		_ = lock.Close()
		t.Fatal("second OpenTCPLock succeeded, want the lock to reject it")
	}
}

func TestTCPCloseRemovesEndpoint(t *testing.T) {
	endpointPath := filepath.Join(t.TempDir(), "runtime", "proj.endpoint")
	lock, err := OpenTCPLock(endpointPath + ".lock")
	if err != nil {
		t.Fatalf("OpenTCPLock: %v", err)
	}
	listener, err := ListenTCP("", endpointPath, lock)
	if err != nil {
		_ = lock.Close()
		t.Fatalf("ListenTCP: %v", err)
	}
	if closeErr := listener.Close(); closeErr != nil {
		t.Fatalf("Close: %v", closeErr)
	}
	if closeErr := listener.Close(); closeErr != nil {
		t.Fatalf("second Close: %v", closeErr)
	}
	if _, statErr := os.Stat(endpointPath); !os.IsNotExist(statErr) {
		t.Errorf("endpoint still present after Close: %v", statErr)
	}
	secondLock, lockErr := OpenTCPLock(endpointPath + ".lock")
	if lockErr != nil {
		t.Fatalf("OpenTCPLock after Close: %v", lockErr)
	}
	_ = secondLock.Close()
}

func TestTCPServesProtocol(t *testing.T) {
	listener := newTCPListenerForTest(t)
	d := NewDaemon(testServer(), nil, func(string) (Listener, error) { return listener, nil }, "tcp")
	done := make(chan error, 1)
	go func() { done <- d.Serve(context.Background()) }()

	client, err := DialTCP(listener.endpointPath)
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
	listener := newTCPListenerForTest(t)
	d := NewDaemon(testServer(), nil, func(string) (Listener, error) { return listener, nil }, "tcp")
	done := make(chan error, 1)
	go func() { done <- d.Serve(context.Background()) }()

	record, err := readEndpoint(listener.endpointPath)
	if err != nil {
		t.Fatalf("readEndpoint: %v", err)
	}
	address := record.Host + ":" + itoa(record.Port)
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
	listener := newTCPListenerForTest(t)
	d := NewDaemon(testServer(), nil, func(string) (Listener, error) { return listener, nil }, "tcp")
	done := make(chan error, 1)
	go func() { done <- d.Serve(context.Background()) }()

	record, err := readEndpoint(listener.endpointPath)
	if err != nil {
		t.Fatalf("readEndpoint: %v", err)
	}
	var dialer net.Dialer
	conn, err := dialer.DialContext(context.Background(), "tcp", record.Host+":"+itoa(record.Port))
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

func TestTCPPeerIdentityAbsent(t *testing.T) {
	conn := &tcpConn{}
	if _, err := conn.PeerIdentity(); !errors.Is(err, errPeerIdentityAbsent) {
		t.Fatalf("PeerIdentity err = %v, want errPeerIdentityAbsent", err)
	}
}

// Backed by a live pipe so the prelude deadline calls are safe.
func newTestTCPConn(t *testing.T, body, wantToken string) *tcpConn {
	t.Helper()
	client, server := net.Pipe()
	t.Cleanup(func() { _ = client.Close() })
	return &tcpConn{Conn: server, reader: bufio.NewReader(strings.NewReader(body)), wantToken: wantToken}
}

func TestTCPAuthenticateRejectsMalformed(t *testing.T) {
	conn := newTestTCPConn(t, "nope\n", "secret")
	if err := conn.Authenticate(); !errors.Is(err, errTCPTokenRejected) {
		t.Fatalf("Authenticate err = %v, want errTCPTokenRejected", err)
	}
	if conn.Authenticated() {
		t.Error("Authenticated = true after a rejected prelude")
	}
}

func TestTCPAuthenticateAcceptsToken(t *testing.T) {
	conn := newTestTCPConn(t, `{"token":"secret"}`+"\n"+"hello\n", "secret")
	if err := conn.Authenticate(); err != nil {
		t.Fatalf("Authenticate: %v", err)
	}
	if !conn.Authenticated() {
		t.Fatal("Authenticated = false after a matching prelude")
	}
	line, err := conn.reader.ReadString('\n')
	if err != nil {
		t.Fatalf("read buffered MCP bytes: %v", err)
	}
	if line != "hello\n" {
		t.Errorf("buffered bytes = %q, want hello", line)
	}
}

func TestReadBoundedLineRejectsOversize(t *testing.T) {
	long := strings.Repeat("a", maxTokenPreludeBytes+1)
	reader := bufio.NewReaderSize(strings.NewReader(long+"\n"), 64)
	if _, err := readBoundedLine(reader, maxTokenPreludeBytes); err == nil {
		t.Fatal("readBoundedLine accepted an oversize line, want an error")
	}
}

func itoa(n int) string {
	data, _ := json.Marshal(n)
	return string(data)
}
