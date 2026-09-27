package mcp

import (
	"bufio"
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"
)

// TestInProcListenerServesProtocol drives the existing line protocol over an
// in-process connection: the client and the server are the two ends of a
// connected stream pair, so Serve runs against the listener exactly as it will
// against an accepted socket.
func TestInProcListenerServesProtocol(t *testing.T) {
	l := NewInProcListener()
	defer l.Close()

	client, err := l.Dial()
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer client.Close()

	server, err := l.Accept()
	if err != nil {
		t.Fatalf("accept: %v", err)
	}
	defer server.Close()

	done := make(chan struct{})
	go func() {
		defer close(done)
		// Serve returns when the stream closes; the error is expected here.
		_ = testServer().Serve(context.Background(), server, server)
	}()

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

	_ = client.Close()
	_ = server.Close()
	<-done
}

// TestInProcListenerQueuesDials asserts the listener holds several connections
// dialed ahead of Accept, the way a listening socket queues pending connects.
func TestInProcListenerQueuesDials(t *testing.T) {
	l := NewInProcListener()
	defer l.Close()

	const n = 3
	for i := range n {
		client, err := l.Dial()
		if err != nil {
			t.Fatalf("dial %d: %v", i, err)
		}
		defer client.Close()
	}
	for i := range n {
		conn, err := l.Accept()
		if err != nil {
			t.Fatalf("accept %d: %v", i, err)
		}
		defer conn.Close()
	}
}

// TestInProcListenerAcceptUnblocksOnClose asserts a blocked Accept returns when
// the listener closes, so daemon shutdown cannot hang on it.
func TestInProcListenerAcceptUnblocksOnClose(t *testing.T) {
	l := NewInProcListener()
	errCh := make(chan error, 1)
	go func() {
		_, err := l.Accept()
		errCh <- err
	}()

	if err := l.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	select {
	case err := <-errCh:
		if !errors.Is(err, errInProcClosed) {
			t.Fatalf("accept err = %v, want %v", err, errInProcClosed)
		}
	case <-time.After(time.Second):
		t.Fatal("Accept did not unblock on Close")
	}
}

func TestInProcListenerCloseStopsAcceptAndDial(t *testing.T) {
	l := NewInProcListener()
	if err := l.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	if err := l.Close(); err != nil {
		t.Fatalf("second close: %v", err)
	}

	if _, err := l.Accept(); !errors.Is(err, errInProcClosed) {
		t.Errorf("accept err = %v, want %v", err, errInProcClosed)
	}
	if _, err := l.Dial(); !errors.Is(err, errInProcClosed) {
		t.Errorf("dial err = %v, want %v", err, errInProcClosed)
	}
}

func TestInProcListenerPeerIdentity(t *testing.T) {
	l := NewInProcListener()
	defer l.Close()
	want := PeerIdentity{UserID: 4242, ProcessID: 99}
	l.peer = want

	client, err := l.Dial()
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer client.Close()

	conn, err := l.Accept()
	if err != nil {
		t.Fatalf("accept: %v", err)
	}
	defer conn.Close()

	got, err := conn.PeerIdentity()
	if err != nil {
		t.Fatalf("PeerIdentity: %v", err)
	}
	if got != want {
		t.Errorf("peer = %+v, want %+v", got, want)
	}
}

// TestInProcListenerDefaultPeerIsSelf asserts an unconfigured in-process
// connection reports the current process, since that is the only identity an
// in-process transport can honestly claim.
func TestInProcListenerDefaultPeerIsSelf(t *testing.T) {
	l := NewInProcListener()
	defer l.Close()

	client, err := l.Dial()
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer client.Close()
	conn, err := l.Accept()
	if err != nil {
		t.Fatalf("accept: %v", err)
	}
	defer conn.Close()

	got, err := conn.PeerIdentity()
	if err != nil {
		t.Fatalf("PeerIdentity: %v", err)
	}
	if got != currentPeerIdentity() {
		t.Errorf("peer = %+v, want current process %+v", got, currentPeerIdentity())
	}
}

// TestInProcEndpointDialBeforeListenFails asserts a dial that arrives before
// the daemon opened the endpoint fails, which is what drives the client's
// connect-or-spawn path.
func TestInProcEndpointDialBeforeListenFails(t *testing.T) {
	e := NewInProcEndpoint()

	var dial DialFunc = e.Dial
	if _, err := dial("inproc"); !errors.Is(err, errInProcNotStarted) {
		t.Fatalf("dial err = %v, want %v", err, errInProcNotStarted)
	}
}

// TestInProcEndpointListenDialAccept asserts the endpoint's Listen and Dial
// halves meet on one listener, the in-process rendezvous.
func TestInProcEndpointListenDialAccept(t *testing.T) {
	e := NewInProcEndpoint()

	var listen ListenFunc = e.Listen
	l, err := listen("inproc")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer l.Close()

	var dial DialFunc = e.Dial
	client, err := dial("inproc")
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer client.Close()

	server, err := l.Accept()
	if err != nil {
		t.Fatalf("accept: %v", err)
	}
	defer server.Close()

	// net.Pipe is a rendezvous, so the read must run while the write blocks.
	read := make(chan string, 1)
	go func() {
		buf := make([]byte, len("ping"))
		if _, err := io.ReadFull(server, buf); err != nil {
			read <- "read error: " + err.Error()
			return
		}
		read <- string(buf)
	}()
	if _, err := io.WriteString(client, "ping"); err != nil {
		t.Fatalf("write: %v", err)
	}
	if got := <-read; got != "ping" {
		t.Errorf("read %q, want ping", got)
	}
}

// TestListenInProcFillsListenSeam asserts the in-process backend is usable
// through the transport-agnostic seam the daemon core will take.
func TestListenInProcFillsListenSeam(t *testing.T) {
	var listen ListenFunc = ListenInProc
	l, err := listen("ignored-address")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer l.Close()

	inproc, ok := l.(*InProcListener)
	if !ok {
		t.Fatalf("listener = %T, want *InProcListener", l)
	}
	client, err := inproc.Dial()
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer client.Close()

	if _, err := l.Accept(); err != nil {
		t.Fatalf("accept: %v", err)
	}
}
