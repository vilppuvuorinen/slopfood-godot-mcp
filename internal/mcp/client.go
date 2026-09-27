package mcp

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"time"
)

// SpawnFunc starts a daemon the client can then connect to. It returns once the
// daemon has been launched, not necessarily ready: the client polls its dial
// until the daemon accepts or the connect deadline passes. A nil SpawnFunc
// disables spawn-on-demand, so a failed dial is fatal.
type SpawnFunc func(ctx context.Context) error

// Client defaults for reaching a freshly spawned daemon.
const (
	defaultConnectTimeout = 5 * time.Second
	defaultConnectPoll    = 20 * time.Millisecond
)

// errDaemonLost reports that the daemon closed the connection while the client
// still held it. The pump keeps no request log, so it cannot replay an
// in-flight call; the client exits instead and lets the MCP host restart it.
var errDaemonLost = errors.New("daemon connection closed")

// Client is the thin stdio client from the IPC daemon design: it owns no
// protocol knowledge. It dials the daemon, spawning one when the dial fails,
// then copies bytes both ways between the MCP host's stdio and the daemon
// connection. It is deliberately dumber than an MCP-aware relay, so it cannot
// corrupt or delay a request, and the daemon remains the only place that knows
// the protocol.
type Client struct {
	dial   DialFunc
	spawn  SpawnFunc
	addr   string
	logger *slog.Logger
	// connectTimeout bounds how long connect polls after a spawn, and
	// pollInterval is the wait between dial attempts while polling.
	connectTimeout time.Duration
	pollInterval   time.Duration
}

// NewClient returns a client that connects to the daemon at addr through dial,
// spawning one with spawn when no daemon is listening. A nil spawn disables
// spawning, making a failed dial fatal; a nil logger uses the default logger.
func NewClient(dial DialFunc, spawn SpawnFunc, addr string, logger *slog.Logger) *Client {
	if logger == nil {
		logger = slog.Default()
	}
	return &Client{
		dial:           dial,
		spawn:          spawn,
		addr:           addr,
		logger:         logger,
		connectTimeout: defaultConnectTimeout,
		pollInterval:   defaultConnectPoll,
	}
}

// Run connects to the daemon and pumps the MCP host's stdio to and from it
// until either side ends. It is the client's whole role.
func (c *Client) Run(ctx context.Context, stdin io.Reader, stdout io.Writer) error {
	conn, err := c.connect(ctx)
	if err != nil {
		return err
	}
	return Pump(ctx, conn, stdin, stdout)
}

// connect returns a live daemon connection. It dials first and, when that fails
// and a spawn function is set, launches a daemon and polls until it accepts or
// the connect deadline passes. The first dial is what makes an existing daemon
// cheap to join: the usual case is connect, not spawn.
func (c *Client) connect(ctx context.Context) (io.ReadWriteCloser, error) {
	conn, err := c.dial(c.addr)
	if err == nil {
		return conn, nil
	}
	if c.spawn == nil {
		return nil, fmt.Errorf("dial daemon: %w", err)
	}
	c.logger.Debug("no daemon listening; spawning one", "error", err)
	if spawnErr := c.spawn(ctx); spawnErr != nil {
		return nil, fmt.Errorf("spawn daemon: %w", spawnErr)
	}
	deadline := time.Now().Add(c.connectTimeout)
	for {
		conn, err = c.dial(c.addr)
		if err == nil {
			return conn, nil
		}
		if !time.Now().Before(deadline) {
			return nil, fmt.Errorf("connect to spawned daemon: %w", err)
		}
		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("connect to spawned daemon: %w", ctx.Err())
		case <-time.After(c.pollInterval):
		}
	}
}

// copyResult reports one direction of the pump: which copy ended and why.
type copyResult struct {
	fromDaemon bool
	err        error
}

// Pump copies stdin to conn and conn to stdout concurrently until either copy
// ends, then closes conn and returns. The copies are concurrent because the
// protocol is not one reply per line: a notification gets no response, so a
// synchronous relay would hang on the first notifications/initialized. Nothing
// is inspected, so the pump cannot corrupt or delay a request.
//
// A clean EOF from the daemon side is not success: it means the daemon went
// away with a request possibly in flight, so Pump returns errDaemonLost and the
// caller can exit non-zero. A clean EOF from stdin means the host is done, so
// Pump returns nil.
func Pump(ctx context.Context, conn io.ReadWriteCloser, stdin io.Reader, stdout io.Writer) error {
	results := make(chan copyResult, 2)
	go func() {
		_, err := io.Copy(conn, stdin)
		results <- copyResult{err: err}
	}()
	go func() {
		_, err := io.Copy(stdout, conn)
		results <- copyResult{fromDaemon: true, err: err}
	}()

	// The first copy to end decides the outcome; closing conn afterwards
	// unblocks the other one, and the buffered channel keeps it from leaking.
	var outcome error
	select {
	case <-ctx.Done():
		outcome = fmt.Errorf("pump: %w", ctx.Err())
	case res := <-results:
		switch {
		case res.err != nil:
			outcome = fmt.Errorf("pump stdio: %w", res.err)
		case res.fromDaemon:
			outcome = errDaemonLost
		}
	}
	_ = conn.Close()
	return outcome
}
