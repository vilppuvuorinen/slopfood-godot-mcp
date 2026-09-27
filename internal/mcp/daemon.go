package mcp

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"
)

// errDaemonClosed reports that the daemon has been closed and no longer starts
// new work.
var errDaemonClosed = errors.New("daemon closed")

// errPeerRejected reports that a connection's peer could not be authorized and
// was therefore not served.
var errPeerRejected = errors.New("connection rejected by peer authorization")

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
//
// The daemon also owns its lifetime: a context canceled by Shutdown, by the
// idle grace timer once the last connection leaves, or by Close, which Serve
// observes to stop accepting.
type Daemon struct {
	server *Server
	queue  *Queue
	listen ListenFunc
	addr   string

	// lifeCtx is canceled when the daemon should stop serving, whether from
	// Shutdown, an expired idle grace period, or Close. Done exposes it so a
	// process owner can return as soon as the daemon goes down.
	lifeCtx    context.Context
	lifeCancel context.CancelFunc

	mu       sync.Mutex
	listener Listener
	conns    map[Connection]struct{}
	closed   bool
	// idleTimeout, when positive, is how long the daemon may sit with no
	// active connections before it shuts itself down; idleTimer holds the
	// pending countdown while it is idle. Both are guarded by mu.
	idleTimeout time.Duration
	idleTimer   *time.Timer

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
	lifeCtx, lifeCancel := context.WithCancel(context.Background())
	return &Daemon{
		server:     server,
		queue:      queue,
		listen:     listen,
		addr:       addr,
		lifeCtx:    lifeCtx,
		lifeCancel: lifeCancel,
		conns:      make(map[Connection]struct{}),
	}
}

// SetIdleTimeout sets how long the daemon may sit with no active connections
// before it shuts itself down, as if Shutdown had been called. A non-positive
// timeout disables idle exit, which is the default. It must be called before
// Serve, which arms the countdown.
func (d *Daemon) SetIdleTimeout(timeout time.Duration) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.idleTimeout = timeout
}

// ActiveConnections reports how many client connections the daemon is serving
// right now. It is the idle-accounting figure: at zero the grace timer arms,
// and any tracked connection disarms it.
func (d *Daemon) ActiveConnections() int {
	d.mu.Lock()
	defer d.mu.Unlock()
	return len(d.conns)
}

// Shutdown cancels the daemon's lifetime context, which stops Serve the same
// way a canceled caller context does: the listener closes, a pending Accept
// unblocks, and Serve returns nil. It is the hook a later phase wires to a
// signal. It does not close active connections or the queue; Close does that.
// It is safe to call more than once.
func (d *Daemon) Shutdown() { d.lifeCancel() }

// Done returns a channel closed once the daemon is shutting down, whether from
// Shutdown, an expired idle grace period, or Close.
func (d *Daemon) Done() <-chan struct{} { return d.lifeCtx.Done() }

// Serve opens the listener and accepts connections until the daemon closes,
// the context is canceled, or accepting fails. Each accepted connection is
// served on its own goroutine, so a slow client cannot stall the accept loop;
// all of them share the daemon's one Server and Queue. Serve returns nil after
// a clean Close, Shutdown, or idle exit, the context error when ctx is canceled
// first, and a listen or accept error otherwise.
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
	// A daemon that no client ever reaches still has zero active connections,
	// so the idle countdown starts here too, not only on the way down from a
	// served connection.
	d.armIdleLocked()
	d.mu.Unlock()
	defer d.loopWG.Done()

	// A canceled context closes the listener, which unblocks Accept. Serve
	// never cancels the per-connection contexts itself: a connection stops when
	// its transport closes, and in-flight queue work is bounded by its own
	// timeout.
	stop := context.AfterFunc(ctx, func() { _ = l.Close() })
	defer stop()

	// The lifetime context is the second way to stop accepting: Shutdown and an
	// expired idle grace period cancel it, and it closes the listener exactly
	// as a canceled caller context does.
	stopLife := context.AfterFunc(d.lifeCtx, func() { _ = l.Close() })
	defer stopLife()

	for {
		conn, err := l.Accept()
		if err != nil {
			if d.isClosed() || d.lifeCtx.Err() != nil {
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
// the daemon is already down, and errPeerRejected when the peer fails
// authorization.
func (d *Daemon) ServeConnection(ctx context.Context, conn Connection) error {
	if d.isClosed() {
		return errDaemonClosed
	}
	if !d.authorize(conn) {
		_ = conn.Close()
		return errPeerRejected
	}
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
	// Closing is a shutdown: cancel the lifetime context so anything selecting
	// on Done wakes, and drop any pending idle countdown.
	d.lifeCancel()
	d.disarmIdleLocked()
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
// connection accepted after Close, or one whose peer fails authorization, is
// dropped rather than served.
func (d *Daemon) startConnection(ctx context.Context, conn Connection) {
	if !d.authorize(conn) {
		_ = conn.Close()
		return
	}
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

// authorize reports whether conn's peer may be served. The transport may be
// unable to name the peer, which is only acceptable when the platform reports
// credentials as unsupported; every other failure, and any UID that differs
// from this process's, is a rejection. The caller must not track or serve a
// connection authorize rejects.
func (d *Daemon) authorize(conn Connection) bool {
	peer, err := conn.PeerIdentity()
	if err != nil {
		if errors.Is(err, errPeerIdentityUnsupported) {
			d.server.logger.Debug("peer identity unavailable; relying on filesystem permissions")
			return true
		}
		d.server.logger.Warn("rejected connection: cannot identify peer", "error", err)
		return false
	}
	if self := currentPeerIdentity(); peer.UserID != self.UserID {
		d.server.logger.Warn("rejected connection from another user", "uid", peer.UserID)
		return false
	}
	return true
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
	// An attached peer keeps the daemon alive, so an arriving connection stops
	// any pending idle countdown.
	d.disarmIdleLocked()
	return true
}

// untrack removes conn from the active set and releases its connWG slot. The
// transition to zero active connections is what arms the idle countdown.
func (d *Daemon) untrack(conn Connection) {
	d.mu.Lock()
	defer d.mu.Unlock()
	delete(d.conns, conn)
	d.connWG.Done()
	d.armIdleLocked()
}

// armIdleLocked starts the idle countdown when the daemon is serving, has no
// active connections, and an idle timeout is set. It is a no-op when a
// countdown is already pending or the daemon is on its way down, so it can be
// called on every transition to zero. The caller must hold d.mu.
func (d *Daemon) armIdleLocked() {
	if d.idleTimeout <= 0 || d.closed || d.lifeCtx.Err() != nil || len(d.conns) > 0 || d.idleTimer != nil {
		return
	}
	d.idleTimer = time.AfterFunc(d.idleTimeout, d.idleExpired)
}

// disarmIdleLocked stops a pending idle countdown, if any. The caller must hold
// d.mu.
func (d *Daemon) disarmIdleLocked() {
	if d.idleTimer != nil {
		d.idleTimer.Stop()
		d.idleTimer = nil
	}
}

// idleExpired is the grace timer callback. It re-checks the active count under
// the lock, because a connection may have arrived between the timer firing and
// this running, and cancels the lifetime context only when the daemon is still
// idle.
func (d *Daemon) idleExpired() {
	d.mu.Lock()
	d.idleTimer = nil
	idle := !d.closed && d.lifeCtx.Err() == nil && len(d.conns) == 0
	d.mu.Unlock()
	if idle {
		d.lifeCancel()
	}
}

func (d *Daemon) isClosed() bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.closed
}
