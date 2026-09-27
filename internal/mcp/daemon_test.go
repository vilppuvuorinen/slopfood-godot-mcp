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

// daemonListen returns a ListenFunc handing out l, so a test can dial the
// daemon through the in-process backend while the daemon still owns the
// listener's lifecycle.
func daemonListen(l Listener) ListenFunc {
	return func(string) (Listener, error) { return l, nil }
}

// stopDaemon closes the daemon and asserts its accept loop returned cleanly.
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

// TestDaemonServesAcceptedConnections drives the line protocol through the
// accept loop: the daemon accepts an in-process connection and serves it on the
// shared server, exactly as it will serve an accepted socket.
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

// TestDaemonServeConnectionServesStdio asserts the stdio path is served as one
// connection to the core: the same line protocol answers it, with no separate
// stdio code path.
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

// TestDaemonCloseClosesActiveConnections asserts Close tears down a served
// connection, so a client cannot outlive the daemon that served it.
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

	// A round trip proves the daemon accepted and is serving the connection
	// before Close runs.
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

// TestDaemonSerializesAcrossConnections asserts every connection shares the
// daemon's one queue, so concurrent clients never run engine work at once.
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

// TestDaemonCloseClosesQueue asserts ownership: the queue dies with the daemon,
// so a later submit is rejected rather than run.
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

// TestDaemonServeAfterCloseFails asserts a closed daemon starts no new work,
// including dropping a connection offered directly to the core.
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

// TestDaemonServeStopsOnContextCancel asserts a canceled context stops the
// accept loop, so shutdown does not have to wait for Close.
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
