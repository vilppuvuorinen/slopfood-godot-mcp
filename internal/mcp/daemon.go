package mcp

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"
)

var errDaemonClosed = errors.New("daemon closed")
var errPeerRejected = errors.New("connection rejected by peer authorization")

type Daemon struct {
	server *Server
	queue  *Queue
	listen ListenFunc
	addr   string

	lifeCtx    context.Context
	lifeCancel context.CancelFunc

	mu       sync.Mutex
	listener Listener
	conns    map[Connection]struct{}
	closed   bool
	// idleTimeout and idleTimer are guarded by mu.
	idleTimeout time.Duration
	idleTimer   *time.Timer

	loopWG sync.WaitGroup
	connWG sync.WaitGroup
}

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

// Must be called before Serve.
func (d *Daemon) SetIdleTimeout(timeout time.Duration) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.idleTimeout = timeout
}

func (d *Daemon) ActiveConnections() int {
	d.mu.Lock()
	defer d.mu.Unlock()
	return len(d.conns)
}

// Shutdown stops Serve by canceling the lifetime context but does not close
// active connections or the queue; it is safe to call more than once.
func (d *Daemon) Shutdown() { d.lifeCancel() }

func (d *Daemon) Done() <-chan struct{} { return d.lifeCtx.Done() }

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
	d.armIdleLocked()
	d.mu.Unlock()
	defer d.loopWG.Done()

	stop := context.AfterFunc(ctx, func() { _ = l.Close() })
	defer stop()

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

// The daemon takes ownership of conn and closes it when the serve loop returns.
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

func (d *Daemon) Close() error {
	d.mu.Lock()
	if d.closed {
		d.mu.Unlock()
		return nil
	}
	d.closed = true
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

// The connection is tracked before pre-auth and the token read runs off the
// accept path, so a silent client cannot stall accepts and Close still waits.
func (d *Daemon) startConnection(ctx context.Context, conn Connection) {
	if !d.track(conn) {
		_ = conn.Close()
		return
	}
	go func() {
		defer d.untrack(conn)
		defer func() { _ = conn.Close() }()
		if err := preAuthenticate(conn); err != nil {
			d.server.logger.Warn("rejected connection: token pre-auth failed", "error", err)
			return
		}
		if !d.authorize(conn) {
			return
		}
		if err := d.server.Serve(ctx, conn, conn); err != nil {
			d.server.logger.Debug("connection serve ended", "error", err)
		}
	}()
}

// No-op for transports that do not implement TokenConnection.
func preAuthenticate(conn Connection) error {
	tokenConn, ok := conn.(TokenConnection)
	if !ok {
		return nil
	}
	if err := tokenConn.Authenticate(); err != nil {
		return fmt.Errorf("token pre-auth: %w", err)
	}
	return nil
}

// Rejects any peer whose UID differs from this process's; unsupported
// credentials fall back to filesystem permissions, and absent identity requires
// a valid token. The caller must not track or serve a rejected connection.
func (d *Daemon) authorize(conn Connection) bool {
	peer, err := conn.PeerIdentity()
	if err != nil {
		if errors.Is(err, errPeerIdentityUnsupported) {
			d.server.logger.Debug("peer identity unavailable; relying on filesystem permissions")
			return true
		}
		if errors.Is(err, errPeerIdentityAbsent) {
			if tokenConn, ok := conn.(TokenConnection); ok && tokenConn.Authenticated() {
				return true
			}
			d.server.logger.Warn("rejected connection: transport has no peer identity and no valid token")
			return false
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

func (d *Daemon) track(conn Connection) bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.closed {
		return false
	}
	d.conns[conn] = struct{}{}
	d.connWG.Add(1)
	d.disarmIdleLocked()
	return true
}

func (d *Daemon) untrack(conn Connection) {
	d.mu.Lock()
	defer d.mu.Unlock()
	delete(d.conns, conn)
	d.connWG.Done()
	d.armIdleLocked()
}

// The caller must hold d.mu.
func (d *Daemon) armIdleLocked() {
	if d.idleTimeout <= 0 || d.closed || d.lifeCtx.Err() != nil || len(d.conns) > 0 || d.idleTimer != nil {
		return
	}
	d.idleTimer = time.AfterFunc(d.idleTimeout, d.idleExpired)
}

// The caller must hold d.mu.
func (d *Daemon) disarmIdleLocked() {
	if d.idleTimer != nil {
		d.idleTimer.Stop()
		d.idleTimer = nil
	}
}

// Re-checks the active count under the lock, since a connection may have
// arrived between the timer firing and this running.
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
