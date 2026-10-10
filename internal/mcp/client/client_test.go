package client

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"net"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func testClientLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// Concurrency-safe io.Writer: the conn-to-stdout copy writes while the test reads.
type lockedBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p) //nolint:wrapcheck // a test buffer passes the writer's errors through unchanged.
}

func (b *lockedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

func TestClientConnectsWithoutSpawning(t *testing.T) {
	var spawns int32
	dial := func(string) (io.ReadWriteCloser, error) { return &fakeConn{}, nil }
	spawn := func(context.Context) error { atomic.AddInt32(&spawns, 1); return nil }

	c := NewClient(dial, spawn, "addr", testClientLogger())
	conn, err := c.connect(context.Background())
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	if conn == nil {
		t.Fatal("connect returned a nil connection")
	}
	if got := atomic.LoadInt32(&spawns); got != 0 {
		t.Errorf("spawn called %d times, want 0 when the dial succeeds", got)
	}
}

func TestClientSpawnsAndPollsWhenDialFails(t *testing.T) {
	var dials, spawns int32
	dial := func(string) (io.ReadWriteCloser, error) {
		if atomic.AddInt32(&dials, 1) < 3 {
			return nil, errors.New("connection refused")
		}
		return &fakeConn{}, nil
	}
	spawn := func(context.Context) error { atomic.AddInt32(&spawns, 1); return nil }

	c := NewClient(dial, spawn, "addr", testClientLogger())
	c.connectTimeout = time.Second
	c.pollInterval = time.Millisecond

	if _, err := c.connect(context.Background()); err != nil {
		t.Fatalf("connect: %v", err)
	}
	if got := atomic.LoadInt32(&spawns); got != 1 {
		t.Errorf("spawn called %d times, want 1", got)
	}
	if got := atomic.LoadInt32(&dials); got < 3 {
		t.Errorf("dial tried %d times, want at least 3", got)
	}
}

func TestClientNoSpawnReturnsDialError(t *testing.T) {
	want := errors.New("connection refused")
	dial := func(string) (io.ReadWriteCloser, error) { return nil, want }

	c := NewClient(dial, nil, "addr", testClientLogger())
	if _, err := c.connect(context.Background()); !errors.Is(err, want) {
		t.Fatalf("connect err = %v, want %v", err, want)
	}
}

func TestClientSpawnErrorIsReturned(t *testing.T) {
	want := errors.New("spawn failed")
	dial := func(string) (io.ReadWriteCloser, error) { return nil, errors.New("refused") }
	spawn := func(context.Context) error { return want }

	c := NewClient(dial, spawn, "addr", testClientLogger())
	if _, err := c.connect(context.Background()); !errors.Is(err, want) {
		t.Fatalf("connect err = %v, want %v", err, want)
	}
}

func TestClientConnectStopsOnContextCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	dial := func(string) (io.ReadWriteCloser, error) { return nil, errors.New("refused") }
	spawn := func(context.Context) error { cancel(); return nil }

	c := NewClient(dial, spawn, "addr", testClientLogger())
	c.connectTimeout = time.Minute
	c.pollInterval = time.Second

	if _, err := c.connect(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("connect err = %v, want %v", err, context.Canceled)
	}
}

// A notification gets no response, so a synchronous relay would hang there.
func TestClientRunPumpsNotificationsAndReplies(t *testing.T) {
	listen, dial := newTestEndpoint(t)
	l, err := listen("test")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer func() { _ = l.Close() }()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	served := make(chan struct{})
	go func() {
		defer close(served)
		conn, acceptErr := l.Accept()
		if acceptErr != nil {
			return
		}
		defer func() { _ = conn.Close() }()
		_ = testServer().Serve(ctx, conn, conn)
	}()

	client := NewClient(dial, nil, "test", testClientLogger())

	inR, inW := io.Pipe()
	var out lockedBuffer
	runErr := make(chan error, 1)
	go func() { runErr <- client.Run(context.Background(), inR, &out) }()

	writeLine(t, inW, `{"jsonrpc":"2.0","method":"notifications/initialized"}`)
	writeLine(t, inW, `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`)

	awaitCondition(t, func() bool { return strings.Contains(out.String(), `"tools"`) }, "the tools/list reply")
	_ = inW.Close()

	select {
	case err := <-runErr:
		if err != nil {
			t.Fatalf("Run: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Run did not return after stdin closed")
	}

	cancel()
	_ = l.Close()
	select {
	case <-served:
	case <-time.After(time.Second):
		t.Fatal("server did not stop after the client disconnected")
	}
}

func TestPumpCopiesBothDirections(t *testing.T) {
	client, daemonSide := net.Pipe()
	defer daemonSide.Close()
	stdinR, stdinW := io.Pipe()
	defer stdinW.Close()
	var stdout lockedBuffer

	done := make(chan error, 1)
	go func() { done <- Pump(context.Background(), client, stdinR, &stdout) }()

	read := make(chan string, 1)
	go func() {
		line, _ := bufio.NewReader(daemonSide).ReadString('\n')
		read <- line
	}()

	writeLine(t, stdinW, "hello")
	if got := <-read; got != "hello\n" {
		t.Errorf("daemon read %q, want hello", got)
	}

	if _, err := io.WriteString(daemonSide, "world\n"); err != nil {
		t.Fatalf("write reply: %v", err)
	}
	awaitCondition(t, func() bool { return stdout.String() == "world\n" }, "the reply to reach stdout")

	_ = stdinW.Close()
	if err := <-done; err != nil {
		t.Fatalf("Pump: %v", err)
	}
}

func TestPumpDaemonLossIsAnError(t *testing.T) {
	client, daemonSide := net.Pipe()
	stdinR, stdinW := io.Pipe()
	defer stdinW.Close()
	var stdout bytes.Buffer

	done := make(chan error, 1)
	go func() { done <- Pump(context.Background(), client, stdinR, &stdout) }()

	_ = daemonSide.Close()
	if err := <-done; !errors.Is(err, errDaemonLost) {
		t.Fatalf("Pump err = %v, want %v", err, errDaemonLost)
	}
}

func TestPumpStopsOnContextCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	client, daemonSide := net.Pipe()
	defer daemonSide.Close()
	stdinR, stdinW := io.Pipe()
	defer stdinW.Close()
	var stdout bytes.Buffer

	done := make(chan error, 1)
	go func() { done <- Pump(ctx, client, stdinR, &stdout) }()

	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("Pump err = %v, want %v", err, context.Canceled)
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

func writeLine(t *testing.T, w io.Writer, line string) {
	t.Helper()
	if _, err := io.WriteString(w, line+"\n"); err != nil {
		t.Fatalf("write %q: %v", line, err)
	}
}
