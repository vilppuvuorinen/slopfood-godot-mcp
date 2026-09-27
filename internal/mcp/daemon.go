package mcp

import (
	"context"
	"errors"
	"fmt"
	"sync"
)

// errDaemonClosed reports that the daemon has been closed and no longer starts
// new work.
var errDaemonClosed = errors.New("daemon closed")

// Daemon is the core the IPC daemon phases converge on: one value that owns the
// one Server, the one Queue, and every connection it serves. The accept loop
// runs each accepted connection on its own goroutine against that shared Server
// and Queue, so the serialization point for engine work is the queue rather
// than a single Serve loop, and it survives any number of connections.
//
// The daemon never names a transport: it takes the ListenFunc seam, so the
// in-process backend drives it today and the Unix-socket backend fills the same
// seam later. The stdio path is one connection to the core via ServeConnection,
// not a special case.
type Daemon struct {
	server *Server
	queue  *Queue
	listen ListenFunc
	addr   string

	mu       sync.Mutex
	listener Listener
	conns    map[Connection]struct{}
	closed   bool

	// loopWG tracks the accept loop so Close can wait for it to stop before
	// declaring the daemon down; connWG tracks per-connection serve loops so
	// Close can wait for in-flight calls to wind down, bounded by the queue's
	// own per-request timeout.
	loopWG sync.WaitGroup
	connWG sync.WaitGroup
}

// NewDaemon returns a daemon that serves server over connections accepted from
// listen(addr). It installs queue into server and takes ownership of both: the
// one queue is the serialization point every connection shares, and Close
// closes it once no connection is being served. addr is opaque to the daemon
// and interpreted by the transport: the in-process backend ignores it, and the
// Unix backend later resolves its socket path from it.
func NewDaemon(server *Server, queue *Queue, listen ListenFunc, addr string) *Daemon {
	server.SetQueue(queue)
	return &Daemon{
		server: server,
		queue:  queue,
		listen: listen,
		addr:   addr,
		conns:  make(map[Connection]struct{}),
	}
}

// Serve opens the listener and accepts connections until the daemon closes,
// the context is canceled, or accepting fails. Each accepted connection is
// served on its own goroutine, so a slow client cannot stall the accept loop;
// all of them share the daemon's one Server and Queue. Serve returns nil after
// a clean Close, the context error when ctx is canceled first, and a listen or
// accept error otherwise.
func (d *Daemon) Serve(ctx context.Context) error {
	l, err := d.listen(d.addr)
	if err != nil {
		return fmt.Errorf("daemon listen: %w", err)
	}

	d.mu.Lock()
	if d.closed {
		d.mu.Unlock()
		_ = l.Close()
		return errDaemonClosed
	}
	d.listener = l
	d.loopWG.Add(1)
	d.mu.Unlock()
	defer d.loopWG.Done()

	// A canceled context closes the listener, which unblocks Accept. Serve
	// never cancels the per-connection contexts itself: a connection stops when
	// its transport closes, and in-flight queue work is bounded by its own
	// timeout.
	stop := context.AfterFunc(ctx, func() { _ = l.Close() })
	defer stop()

	for {
		conn, err := l.Accept()
		if err != nil {
			if d.isClosed() {
				return nil
			}
			if ctxErr := ctx.Err(); ctxErr != nil {
				return fmt.Errorf("daemon serve: %w", ctxErr)
			}
			return fmt.Errorf("daemon accept: %w", err)
		}
		d.startConnection(ctx, conn)
	}
}

// ServeConnection serves one connection that did not come from the listener,
// blocking until it ends. It is how the stdio path is fed to the core: the same
// Serve loop runs on stdio as on an accepted socket, so stdio is one connection
// rather than a special case. The daemon owns the connection once it is
// accepted here, closing it when the serve loop returns and tracking it so
// Close stops it. It returns errDaemonClosed, without taking ownership, when
// the daemon is already down.
func (d *Daemon) ServeConnection(ctx context.Context, conn Connection) error {
	if !d.track(conn) {
		return errDaemonClosed
	}
	defer d.untrack(conn)
	defer func() { _ = conn.Close() }()
	return d.server.Serve(ctx, conn, conn)
}

// Close stops accepting, closes the listener, closes every active connection,
// and waits for their serve loops to end before closing the queue. In-flight
// calls therefore finish or hit the queue's per-request timeout, and no
// goroutine outlives the daemon. It is idempotent and safe to call before or
// while Serve runs.
func (d *Daemon) Close() error {
	d.mu.Lock()
	if d.closed {
		d.mu.Unlock()
		return nil
	}
	d.closed = true
	listener := d.listener
	conns := make([]Connection, 0, len(d.conns))
	for conn := range d.conns {
		conns = append(conns, conn)
	}
	d.mu.Unlock()

	var closeErr error
	if listener != nil {
		// Closing the listener unblocks a pending Accept; Serve then sees the
		// closed state and returns cleanly.
		closeErr = listener.Close()
	}
	for _, conn := range conns {
		_ = conn.Close()
	}
	d.loopWG.Wait()
	d.connWG.Wait()
	if d.queue != nil {
		d.queue.Close()
	}
	if closeErr != nil {
		return fmt.Errorf("daemon close listener: %w", closeErr)
	}
	return nil
}

// startConnection registers conn and serves it on a new goroutine. A
// connection accepted after Close is dropped rather than served.
func (d *Daemon) startConnection(ctx context.Context, conn Connection) {
	if !d.track(conn) {
		_ = conn.Close()
		return
	}
	go func() {
		defer d.untrack(conn)
		defer func() { _ = conn.Close() }()
		if err := d.server.Serve(ctx, conn, conn); err != nil {
			d.server.logger.Debug("connection serve ended", "error", err)
		}
	}()
}

// track registers conn as active and reserves a connWG slot. It reports false
// when the daemon has already closed, in which case the caller must not serve
// the connection.
func (d *Daemon) track(conn Connection) bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.closed {
		return false
	}
	d.conns[conn] = struct{}{}
	d.connWG.Add(1)
	return true
}

// untrack removes conn from the active set and releases its connWG slot.
func (d *Daemon) untrack(conn Connection) {
	d.mu.Lock()
	defer d.mu.Unlock()
	delete(d.conns, conn)
	d.connWG.Done()
}

func (d *Daemon) isClosed() bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.closed
}
