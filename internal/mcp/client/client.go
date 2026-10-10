package client

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"time"

	"github.com/vilppuvuorinen/slopfood-godot-mcp/internal/mcp/transport"
)

// A nil SpawnFunc disables spawn-on-demand, so a failed dial is fatal.
type SpawnFunc func(ctx context.Context) error

const (
	defaultConnectTimeout = 5 * time.Second
	defaultConnectPoll    = 20 * time.Millisecond
)

// The pump keeps no request log, so it cannot replay an in-flight call; the
// client exits instead and lets the MCP host restart it.
var errDaemonLost = errors.New("daemon connection closed")

type Client struct {
	dial   transport.DialFunc
	spawn  SpawnFunc
	addr   string
	logger *slog.Logger
	// connectTimeout and pollInterval configure post-spawn dial polling.
	connectTimeout time.Duration
	pollInterval   time.Duration
}

// A nil spawn disables spawning, making a failed dial fatal; a nil logger uses
// the default logger.
func NewClient(dial transport.DialFunc, spawn SpawnFunc, addr string, logger *slog.Logger) *Client {
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

func (c *Client) Run(ctx context.Context, stdin io.Reader, stdout io.Writer) error {
	conn, err := c.connect(ctx)
	if err != nil {
		return err
	}
	return Pump(ctx, conn, stdin, stdout)
}

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

type copyResult struct {
	fromDaemon bool
	err        error
}

// The copies are concurrent because a notification gets no response, so a
// synchronous relay would hang on the first notifications/initialized. A clean
// EOF from the daemon returns errDaemonLost; a clean EOF from stdin returns nil.
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
