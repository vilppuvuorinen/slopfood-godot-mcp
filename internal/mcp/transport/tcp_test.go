package transport

import (
	"bufio"
	"errors"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func newTCPListenerForTest(t *testing.T) *TCPListener {
	t.Helper()
	endpointPath := filepath.Join(t.TempDir(), "runtime", "proj.endpoint")
	lock, err := OpenListenerLock(endpointPath + ".lock")
	if err != nil {
		t.Fatalf("OpenListenerLock: %v", err)
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

	lock, err := OpenListenerLock(listener.endpointPath + ".lock")
	if err == nil {
		_ = lock.Close()
		t.Fatal("second OpenListenerLock succeeded, want the lock to reject it")
	}
}

func TestTCPCloseRemovesEndpoint(t *testing.T) {
	endpointPath := filepath.Join(t.TempDir(), "runtime", "proj.endpoint")
	lock, err := OpenListenerLock(endpointPath + ".lock")
	if err != nil {
		t.Fatalf("OpenListenerLock: %v", err)
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
	secondLock, lockErr := OpenListenerLock(endpointPath + ".lock")
	if lockErr != nil {
		t.Fatalf("OpenListenerLock after Close: %v", lockErr)
	}
	_ = secondLock.Close()
}

func TestTCPPeerIdentityAbsent(t *testing.T) {
	conn := &tcpConn{}
	if _, err := conn.PeerIdentity(); !errors.Is(err, ErrPeerIdentityAbsent) {
		t.Fatalf("PeerIdentity err = %v, want ErrPeerIdentityAbsent", err)
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
