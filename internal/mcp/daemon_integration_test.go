//go:build !windows

package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// Fresh Unix socket path under a short temp prefix, removed when the test ends.
func shortSocketPath(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "mcp-it-")
	if err != nil {
		t.Fatalf("MkdirTemp: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	return filepath.Join(dir, "d.sock")
}

func toolCallLine(name string) string {
	return `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"` + name + `"}}`
}

// The handler does an unsynchronized read-modify-write; if handlers overlapped,
// -race would flag it and the final count would be short.
func TestUnixDaemonConcurrentClientsSerialize(t *testing.T) {
	sock := shortSocketPath(t)
	l, err := ListenUnix(sock)
	if err != nil {
		t.Fatalf("ListenUnix: %v", err)
	}

	const calls = 8
	var counter int // deliberately unsynchronized; the queue must serialize the handlers.
	var active, maxActive int32
	// The channel orders each handler's counter write before the test's final
	// read; the socket round trip is not a happens-before edge for -race.
	completed := make(chan struct{}, calls)

	srv := NewServer(ServerInfo{Name: "test", Version: "0.0.0"}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	srv.RegisterTool(Tool{
		Name:        "mutate",
		Description: "mutate",
		InputSchema: map[string]any{"type": "object"},
		Handler: func(context.Context, json.RawMessage) (any, error) {
			n := atomic.AddInt32(&active, 1)
			for {
				highest := atomic.LoadInt32(&maxActive)
				if n <= highest || atomic.CompareAndSwapInt32(&maxActive, highest, n) {
					break
				}
			}
			value := counter
			time.Sleep(2 * time.Millisecond)
			counter = value + 1
			atomic.AddInt32(&active, -1)
			completed <- struct{}{}
			return nil, nil
		},
	})

	q := NewQueue(calls, testQueueTimeout)
	d := NewDaemon(srv, q, func(string) (Listener, error) { return l, nil }, sock)
	done := make(chan error, 1)
	go func() { done <- d.Serve(context.Background()) }()

	var wg sync.WaitGroup
	for range calls {
		client, err := DialUnix(sock)
		if err != nil {
			t.Fatalf("DialUnix: %v", err)
		}
		wg.Add(1)
		go func(c io.ReadWriteCloser) {
			defer wg.Done()
			defer func() { _ = c.Close() }()
			if _, err := io.WriteString(c, toolCallLine("mutate")+"\n"); err != nil {
				t.Errorf("write request: %v", err)
				return
			}
			if _, err := bufio.NewReader(c).ReadString('\n'); err != nil {
				t.Errorf("read response: %v", err)
			}
		}(client)
	}
	wg.Wait()
	for range calls {
		<-completed
	}

	if got := atomic.LoadInt32(&maxActive); got != 1 {
		t.Errorf("max concurrent handlers = %d, want 1", got)
	}
	if counter != calls {
		t.Errorf("counter = %d, want %d (handlers lost an update)", counter, calls)
	}

	stopDaemon(t, d, done)
}

// With a depth-1 backlog one call runs and one waits, so a third is shed as an
// ordinary tool error rather than a protocol failure.
func TestUnixDaemonSaturationReturnsOverload(t *testing.T) {
	sock := shortSocketPath(t)
	l, err := ListenUnix(sock)
	if err != nil {
		t.Fatalf("ListenUnix: %v", err)
	}

	var startedOnce sync.Once
	started := make(chan struct{})
	srv := NewServer(ServerInfo{Name: "test", Version: "0.0.0"}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	srv.RegisterTool(Tool{
		Name:        "block",
		Description: "block",
		InputSchema: map[string]any{"type": "object"},
		Handler: func(ctx context.Context, _ json.RawMessage) (any, error) {
			startedOnce.Do(func() { close(started) })
			<-ctx.Done()
			return nil, ctx.Err()
		},
	})

	q := NewQueue(1, testQueueTimeout)
	d := NewDaemon(srv, q, func(string) (Listener, error) { return l, nil }, sock)
	done := make(chan error, 1)
	go func() { done <- d.Serve(context.Background()) }()

	dial := func() io.ReadWriteCloser {
		client, dialErr := DialUnix(sock)
		if dialErr != nil {
			t.Fatalf("DialUnix: %v", dialErr)
		}
		t.Cleanup(func() { _ = client.Close() })
		return client
	}

	first := dial()
	writeLine(t, first, toolCallLine("block"))
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("first handler did not start")
	}

	second := dial()
	writeLine(t, second, toolCallLine("block"))
	awaitCondition(t, func() bool { return len(q.jobs) == 1 }, "the depth-1 backlog to fill")

	third := dial()
	writeLine(t, third, toolCallLine("block"))
	line, err := bufio.NewReader(third).ReadString('\n')
	if err != nil {
		t.Fatalf("read overload response: %v", err)
	}
	var resp struct {
		Result struct {
			IsError bool `json:"isError"`
			Content []struct {
				Text string `json:"text"`
			} `json:"content"`
		} `json:"result"`
	}
	if err := json.Unmarshal([]byte(line), &resp); err != nil {
		t.Fatalf("unmarshal overload response %q: %v", line, err)
	}
	if !resp.Result.IsError {
		t.Fatalf("overload response isError = false: %s", line)
	}
	if len(resp.Result.Content) == 0 || !strings.Contains(resp.Result.Content[0].Text, ErrQueueFull.Error()) {
		t.Errorf("overload content = %+v, want a message containing %q", resp.Result.Content, ErrQueueFull.Error())
	}

	_ = first.Close()
	_ = second.Close()

	stopDaemon(t, d, done)
}

func TestUnixDaemonIdleExitThenRespawn(t *testing.T) {
	sock := shortSocketPath(t)
	l, err := ListenUnix(sock)
	if err != nil {
		t.Fatalf("ListenUnix: %v", err)
	}

	d := NewDaemon(testServer(), nil, func(string) (Listener, error) { return l, nil }, sock)
	d.SetIdleTimeout(20 * time.Millisecond)
	done := make(chan error, 1)
	go func() { done <- d.Serve(context.Background()) }()

	select {
	case serveErr := <-done:
		if serveErr != nil {
			t.Fatalf("Serve: %v", serveErr)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Serve did not return after the idle grace period")
	}
	// Close joins listener cleanup, so socket removal and lock release are done.
	if closeErr := d.Close(); closeErr != nil {
		t.Fatalf("Close: %v", closeErr)
	}
	if _, statErr := os.Stat(sock); !os.IsNotExist(statErr) {
		t.Errorf("socket still present after idle exit: %v", statErr)
	}

	respawned, respawnErr := ListenUnix(sock)
	if respawnErr != nil {
		t.Fatalf("respawn ListenUnix: %v", respawnErr)
	}
	if closeErr := respawned.Close(); closeErr != nil {
		t.Fatalf("respawn Close: %v", closeErr)
	}
}
