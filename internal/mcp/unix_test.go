//go:build !windows

package mcp

import (
	"bufio"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestUnixSocketServesProtocol drives the existing line protocol over a real
// Unix socket: the daemon accepts a connection from net.Dial and answers it on
// the shared server, exactly as stdio and the in-process backend do.
func TestUnixSocketServesProtocol(t *testing.T) {
	sock := filepath.Join(t.TempDir(), "daemon.sock")
	l, err := ListenUnix(sock)
	if err != nil {
		t.Fatalf("ListenUnix: %v", err)
	}
	d := NewDaemon(testServer(), nil, func(string) (Listener, error) { return l, nil }, sock)
	done := make(chan error, 1)
	go func() { done <- d.Serve(context.Background()) }()

	client, err := DialUnix(sock)
	if err != nil {
		t.Fatalf("DialUnix: %v", err)
	}
	defer func() { _ = client.Close() }()

	if _, werr := io.WriteString(client, `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`+"\n"); werr != nil {
		t.Fatalf("write request: %v", werr)
	}
	line, err := bufio.NewReader(client).ReadString('\n')
	if err != nil {
		t.Fatalf("read response: %v", err)
	}
	if !strings.Contains(line, `"tools"`) {
		t.Errorf("response = %s", line)
	}

	stopDaemon(t, d, done)
}

// TestUnixSocketPermissions asserts the socket directory is 0700 and the socket
// itself 0600, so only the owning user can reach the daemon.
func TestUnixSocketPermissions(t *testing.T) {
	sock := filepath.Join(t.TempDir(), "nested", "daemon.sock")
	l, err := ListenUnix(sock)
	if err != nil {
		t.Fatalf("ListenUnix: %v", err)
	}
	defer func() { _ = l.Close() }()

	if info, err := os.Stat(filepath.Dir(sock)); err != nil {
		t.Fatalf("stat socket dir: %v", err)
	} else if got := info.Mode().Perm(); got != 0o700 {
		t.Errorf("socket dir mode = %o, want 700", got)
	}
	if info, err := os.Stat(sock); err != nil {
		t.Fatalf("stat socket: %v", err)
	} else if got := info.Mode().Perm(); got != 0o600 {
		t.Errorf("socket mode = %o, want 600", got)
	}
}

// TestUnixSecondListenFails asserts the exclusive lock is the single-instance
// rule: a second ListenUnix on the same path fails while the first is open.
func TestUnixSecondListenFails(t *testing.T) {
	sock := filepath.Join(t.TempDir(), "daemon.sock")
	first, err := ListenUnix(sock)
	if err != nil {
		t.Fatalf("first ListenUnix: %v", err)
	}
	defer func() { _ = first.Close() }()

	second, err := ListenUnix(sock)
	if err == nil {
		_ = second.Close()
		t.Fatal("second ListenUnix succeeded, want the lock to reject it")
	}
}

// TestUnixReclaimsStaleSocketPath asserts a leftover regular file at the socket
// path is removed and rebound, the recovery a crash needs.
func TestUnixReclaimsStaleSocketPath(t *testing.T) {
	sock := filepath.Join(t.TempDir(), "daemon.sock")
	if err := os.WriteFile(sock, []byte("stale"), 0o600); err != nil {
		t.Fatalf("seed stale path: %v", err)
	}

	l, err := ListenUnix(sock)
	if err != nil {
		t.Fatalf("ListenUnix over stale path: %v", err)
	}
	defer func() { _ = l.Close() }()
}

// TestUnixCloseRemovesSocket asserts Close unlinks the socket path and is
// idempotent.
func TestUnixCloseRemovesSocket(t *testing.T) {
	sock := filepath.Join(t.TempDir(), "daemon.sock")
	l, err := ListenUnix(sock)
	if err != nil {
		t.Fatalf("ListenUnix: %v", err)
	}
	if err := l.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if err := l.Close(); err != nil {
		t.Fatalf("second Close: %v", err)
	}
	if _, err := os.Stat(sock); !os.IsNotExist(err) {
		t.Errorf("socket still present after Close: %v", err)
	}
}

// TestUnixDialWithoutListenerFails asserts a dial with no daemon behind it
// fails, which is what makes the client spawn one.
func TestUnixDialWithoutListenerFails(t *testing.T) {
	sock := filepath.Join(t.TempDir(), "missing.sock")
	conn, err := DialUnix(sock)
	if err == nil {
		_ = conn.Close()
		t.Fatal("DialUnix to a missing socket succeeded, want an error")
	}
}

// TestUnixPeerIdentity asserts the accepted connection reports the kernel's
// view of the peer, which for a same-process dial is this process's UID. It
// skips on platforms whose backend reports peer credentials as unsupported.
func TestUnixPeerIdentity(t *testing.T) {
	sock := filepath.Join(t.TempDir(), "daemon.sock")
	l, err := ListenUnix(sock)
	if err != nil {
		t.Fatalf("ListenUnix: %v", err)
	}
	defer func() { _ = l.Close() }()

	client, err := DialUnix(sock)
	if err != nil {
		t.Fatalf("DialUnix: %v", err)
	}
	defer func() { _ = client.Close() }()

	server, err := l.Accept()
	if err != nil {
		t.Fatalf("Accept: %v", err)
	}
	defer func() { _ = server.Close() }()

	peer, err := server.PeerIdentity()
	if errors.Is(err, errPeerIdentityUnsupported) {
		t.Skip("peer credentials unsupported on this platform")
	}
	if err != nil {
		t.Fatalf("PeerIdentity: %v", err)
	}
	wantUID := os.Getuid()
	if wantUID >= 0 && peer.UserID != uint32(wantUID) { //nolint:gosec // guarded non-negative above.
		t.Errorf("peer UID = %d, want %d", peer.UserID, wantUID)
	}
}
