package mcp

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func daemonListen(l Listener) ListenFunc {
	return func(string) (Listener, error) { return l, nil }
}

func stopDaemon(t *testing.T, d *Daemon, done <-chan error) {
	t.Helper()
	if err := d.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Serve: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Serve did not return after Close")
	}
}

func TestDaemonServesAcceptedConnections(t *testing.T) {
	l := NewInProcListener()
	d := NewDaemon(testServer(), nil, daemonListen(l), "inproc")
	done := make(chan error, 1)
	go func() { done <- d.Serve(context.Background()) }()

	client, err := l.Dial()
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer client.Close()

	if _, err = io.WriteString(client, `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`+"\n"); err != nil {
		t.Fatalf("write request: %v", err)
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

func TestDaemonServeConnectionServesStdio(t *testing.T) {
	var out bytes.Buffer
	conn := NewStdioConn(strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"tools/list"}`+"\n"), &out)
	d := NewDaemon(testServer(), nil, ListenInProc, "")
	defer func() { _ = d.Close() }()

	if err := d.ServeConnection(context.Background(), conn); err != nil {
		t.Fatalf("ServeConnection: %v", err)
	}
	if !strings.Contains(out.String(), `"tools"`) {
		t.Errorf("output = %s", out.String())
	}
}

func TestDaemonCloseClosesActiveConnections(t *testing.T) {
	l := NewInProcListener()
	d := NewDaemon(testServer(), nil, daemonListen(l), "inproc")
	done := make(chan error, 1)
	go func() { done <- d.Serve(context.Background()) }()

	client, err := l.Dial()
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer client.Close()

	if _, err := io.WriteString(client, `{"jsonrpc":"2.0","id":1,"method":"ping"}`+"\n"); err != nil {
		t.Fatalf("write request: %v", err)
	}
	if _, err := bufio.NewReader(client).ReadString('\n'); err != nil {
		t.Fatalf("read response: %v", err)
	}

	stopDaemon(t, d, done)

	if _, err := client.Read(make([]byte, 1)); err == nil {
		t.Error("read from the closed connection succeeded, want an error")
	}
}

func TestDaemonSerializesAcrossConnections(t *testing.T) {
	srv := NewServer(ServerInfo{Name: "test", Version: "0.0.0"}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	var active, maxActive int64
	srv.RegisterTool(Tool{
		Name:        "engine",
		Description: "engine",
		InputSchema: map[string]any{"type": "object"},
		Handler: func(context.Context, json.RawMessage) (any, error) {
			n := atomic.AddInt64(&active, 1)
			for {
				highest := atomic.LoadInt64(&maxActive)
				if n <= highest || atomic.CompareAndSwapInt64(&maxActive, highest, n) {
					break
				}
			}
			time.Sleep(time.Millisecond)
			atomic.AddInt64(&active, -1)
			return nil, nil
		},
	})
	queue := NewQueue(64, testQueueTimeout)
	l := NewInProcListener()
	d := NewDaemon(srv, queue, daemonListen(l), "inproc")
	done := make(chan error, 1)
	go func() { done <- d.Serve(context.Background()) }()

	const clients = 4
	var wg sync.WaitGroup
	for range clients {
		client, err := l.Dial()
		if err != nil {
			t.Fatalf("dial: %v", err)
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer client.Close()
			req := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"engine"}}` + "\n"
			if _, err := io.WriteString(client, req); err != nil {
				t.Errorf("write request: %v", err)
				return
			}
			if _, err := bufio.NewReader(client).ReadString('\n'); err != nil {
				t.Errorf("read response: %v", err)
			}
		}()
	}
	wg.Wait()

	stopDaemon(t, d, done)

	if got := atomic.LoadInt64(&maxActive); got != 1 {
		t.Errorf("max concurrent engine handlers = %d, want 1", got)
	}
}

func TestDaemonCloseClosesQueue(t *testing.T) {
	queue := NewQueue(1, testQueueTimeout)
	d := NewDaemon(testServer(), queue, ListenInProc, "")

	if err := d.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if _, err := queue.Submit(context.Background(), func(context.Context) (any, error) { return nil, nil }); !errors.Is(err, ErrQueueClosed) {
		t.Fatalf("Submit err = %v, want %v", err, ErrQueueClosed)
	}
}

func TestDaemonServeAfterCloseFails(t *testing.T) {
	d := NewDaemon(testServer(), nil, ListenInProc, "")
	if err := d.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if err := d.Close(); err != nil {
		t.Fatalf("second Close: %v", err)
	}
	if err := d.Serve(context.Background()); !errors.Is(err, errDaemonClosed) {
		t.Fatalf("Serve err = %v, want %v", err, errDaemonClosed)
	}
	if err := d.ServeConnection(context.Background(), &fakeConn{}); !errors.Is(err, errDaemonClosed) {
		t.Fatalf("ServeConnection err = %v, want %v", err, errDaemonClosed)
	}
}

func TestDaemonServeStopsOnContextCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	l := NewInProcListener()
	d := NewDaemon(testServer(), nil, daemonListen(l), "inproc")
	done := make(chan error, 1)
	go func() { done <- d.Serve(ctx) }()
	cancel()

	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("Serve err = %v, want %v", err, context.Canceled)
		}
	case <-time.After(time.Second):
		t.Fatal("Serve did not return after cancel")
	}
	if err := d.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
}

func awaitCondition(t *testing.T, cond func() bool, what string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

func roundTripPing(t *testing.T, client io.ReadWriteCloser) {
	t.Helper()
	if _, err := io.WriteString(client, `{"jsonrpc":"2.0","id":1,"method":"ping"}`+"\n"); err != nil {
		t.Fatalf("write request: %v", err)
	}
	if _, err := bufio.NewReader(client).ReadString('\n'); err != nil {
		t.Fatalf("read response: %v", err)
	}
}

func TestDaemonActiveConnections(t *testing.T) {
	l := NewInProcListener()
	d := NewDaemon(testServer(), nil, daemonListen(l), "inproc")
	done := make(chan error, 1)
	go func() { done <- d.Serve(context.Background()) }()

	client, err := l.Dial()
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	roundTripPing(t, client)
	if got := d.ActiveConnections(); got != 1 {
		t.Errorf("ActiveConnections = %d while connected, want 1", got)
	}

	_ = client.Close()
	awaitCondition(t, func() bool { return d.ActiveConnections() == 0 }, "the connection to be released")
	stopDaemon(t, d, done)
}

func TestDaemonIdleExit(t *testing.T) {
	l := NewInProcListener()
	d := NewDaemon(testServer(), nil, daemonListen(l), "inproc")
	if err := d.SetIdleTimeout(10 * time.Millisecond); err != nil {
		t.Fatalf("SetIdleTimeout: %v", err)
	}
	done := make(chan error, 1)
	go func() { done <- d.Serve(context.Background()) }()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Serve: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Serve did not return after the idle grace period")
	}
	select {
	case <-d.Done():
	default:
		t.Error("Done is not closed after idle exit")
	}
	if err := d.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
}

func TestDaemonConnectionDisarmsIdleTimer(t *testing.T) {
	l := NewInProcListener()
	d := NewDaemon(testServer(), nil, daemonListen(l), "inproc")
	if err := d.SetIdleTimeout(80 * time.Millisecond); err != nil {
		t.Fatalf("SetIdleTimeout: %v", err)
	}
	done := make(chan error, 1)
	go func() { done <- d.Serve(context.Background()) }()

	client, err := l.Dial()
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	roundTripPing(t, client)
	awaitCondition(t, func() bool { return d.ActiveConnections() == 1 }, "the connection to be tracked")

	select {
	case err := <-done:
		t.Fatalf("Serve returned while a connection was active: %v", err)
	case <-time.After(240 * time.Millisecond):
	}

	_ = client.Close()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Serve: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Serve did not return after the last connection left")
	}
	if err := d.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
}

func TestDaemonIdleDisabledByDefault(t *testing.T) {
	l := NewInProcListener()
	d := NewDaemon(testServer(), nil, daemonListen(l), "inproc")
	done := make(chan error, 1)
	go func() { done <- d.Serve(context.Background()) }()

	select {
	case err := <-done:
		t.Fatalf("Serve returned without an idle timeout: %v", err)
	case <-time.After(50 * time.Millisecond):
	}
	stopDaemon(t, d, done)
}

func TestDaemonShutdownStopsServe(t *testing.T) {
	l := NewInProcListener()
	d := NewDaemon(testServer(), nil, daemonListen(l), "inproc")
	done := make(chan error, 1)
	go func() { done <- d.Serve(context.Background()) }()

	d.Shutdown()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Serve: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Serve did not return after Shutdown")
	}
	select {
	case <-d.Done():
	default:
		t.Error("Done is not closed after Shutdown")
	}
	if err := d.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
}

func TestStdioConnAdaptsStreams(t *testing.T) {
	var out bytes.Buffer
	conn := NewStdioConn(strings.NewReader("hello"), &out)

	buf := make([]byte, len("hello"))
	if _, err := io.ReadFull(conn, buf); err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(buf) != "hello" {
		t.Errorf("read %q, want hello", buf)
	}
	if _, err := conn.Write([]byte("world")); err != nil {
		t.Fatalf("write: %v", err)
	}
	if out.String() != "world" {
		t.Errorf("wrote %q, want world", out.String())
	}
	if err := conn.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	got, err := conn.PeerIdentity()
	if err != nil {
		t.Fatalf("PeerIdentity: %v", err)
	}
	if got != currentPeerIdentity() {
		t.Errorf("peer = %+v, want current process %+v", got, currentPeerIdentity())
	}
}

func TestDaemonRejectsUnidentifiedPeer(t *testing.T) {
	d := NewDaemon(testServer(), nil, ListenInProc, "")
	defer func() { _ = d.Close() }()

	conn := &fakeConn{peerErr: errors.New("no credentials")}
	if err := d.ServeConnection(context.Background(), conn); !errors.Is(err, errPeerRejected) {
		t.Fatalf("ServeConnection err = %v, want %v", err, errPeerRejected)
	}
	if !conn.closed {
		t.Error("rejected connection was not closed")
	}
}

func TestDaemonRejectsDifferentUser(t *testing.T) {
	d := NewDaemon(testServer(), nil, ListenInProc, "")
	defer func() { _ = d.Close() }()

	conn := &fakeConn{peer: PeerIdentity{UserID: currentPeerIdentity().UserID + 1}}
	if err := d.ServeConnection(context.Background(), conn); !errors.Is(err, errPeerRejected) {
		t.Fatalf("ServeConnection err = %v, want %v", err, errPeerRejected)
	}
	if !conn.closed {
		t.Error("rejected connection was not closed")
	}
}

func TestDaemonRejectsAbsentPeerWithoutToken(t *testing.T) {
	d := NewDaemon(testServer(), nil, ListenInProc, "")
	defer func() { _ = d.Close() }()

	conn := &authConn{authErr: errTCPTokenRejected}
	if err := d.ServeConnection(context.Background(), conn); !errors.Is(err, errPeerRejected) {
		t.Fatalf("ServeConnection err = %v, want %v", err, errPeerRejected)
	}
	if !conn.closed {
		t.Error("rejected connection was not closed")
	}
}

func TestDaemonAllowsAuthenticatedAbsentPeer(t *testing.T) {
	conn := &authConn{
		fakeConn: fakeConn{in: strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"tools/list"}` + "\n")},
	}
	d := NewDaemon(testServer(), nil, ListenInProc, "")
	defer func() { _ = d.Close() }()

	// ServeConnection does not run the pre-auth hook, so authenticate directly.
	if err := conn.Authenticate(); err != nil {
		t.Fatalf("Authenticate: %v", err)
	}
	if err := d.ServeConnection(context.Background(), conn); err != nil {
		t.Fatalf("ServeConnection: %v", err)
	}
	if !strings.Contains(conn.out.String(), `"tools"`) {
		t.Errorf("output = %s", conn.out.String())
	}
}

func TestDaemonAllowsUnsupportedPeerIdentity(t *testing.T) {
	conn := &fakeConn{
		in:      strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"tools/list"}` + "\n"),
		peerErr: errPeerIdentityUnsupported,
	}
	d := NewDaemon(testServer(), nil, ListenInProc, "")
	defer func() { _ = d.Close() }()

	if err := d.ServeConnection(context.Background(), conn); err != nil {
		t.Fatalf("ServeConnection: %v", err)
	}
	if !strings.Contains(conn.out.String(), `"tools"`) {
		t.Errorf("output = %s", conn.out.String())
	}
}
