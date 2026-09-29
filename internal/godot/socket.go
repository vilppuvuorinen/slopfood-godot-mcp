package godot

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// readyInfo is the readiness object the bridge prints on stdout after it binds
// the socket. The parent's stdout pipe is private, so the per-process token
// travels here rather than through argv or the environment. Path is set for the
// UDS transport; Host and Port are set for TCP.
type readyInfo struct {
	Ready     bool   `json:"ready"`
	Transport string `json:"transport"`
	Path      string `json:"path"`
	Host      string `json:"host"`
	Port      int    `json:"port"`
	Token     string `json:"token"`
}

// startupReadyTimeout bounds how long a socket session waits for the readiness
// line before killing the engine.
const startupReadyTimeout = 30 * time.Second

// parseReady returns the readiness object from line, nil for the banner or any
// other non-object output, and an error for a `{`-prefixed line that is not a
// valid JSON object.
func parseReady(line string) (*readyInfo, error) {
	trimmed := strings.TrimSpace(line)
	if !strings.HasPrefix(trimmed, "{") {
		return nil, nil
	}
	var info readyInfo
	if err := json.Unmarshal([]byte(trimmed), &info); err != nil {
		return nil, fmt.Errorf("engine session: parse readiness: %w", err)
	}
	if !info.Ready {
		return nil, nil
	}
	return &info, nil
}

// readReadiness reads stdout lines until the readiness object, skipping the
// engine banner and any other non-object line. The deadline is enforced by
// canceling ctx, which kills the engine and closes the pipe under the reader;
// the read then returns its error rather than blocking forever.
func readReadiness(reader *bufio.Reader, cancel context.CancelFunc) (*readyInfo, error) {
	timer := time.AfterFunc(startupReadyTimeout, cancel)
	defer timer.Stop()
	for {
		line, err := reader.ReadString('\n')
		if len(line) > 0 {
			info, parseErr := parseReady(line)
			if parseErr != nil {
				return nil, parseErr
			}
			if info != nil {
				return info, nil
			}
		}
		if err != nil {
			return nil, fmt.Errorf("engine session: read readiness: %w", err)
		}
	}
}

// engineSocket is the Go end of the bridge's Unix-socket transport. It holds
// the token the bridge reported on stdout and presents it on the first request
// frame of each connection.
type engineSocket struct {
	conn      net.Conn
	reader    *bufio.Reader
	token     string
	tokenSent bool
}

// dialEngineSocket connects to the bridge's Unix socket and returns its framing
// wrapper.
func dialEngineSocket(ctx context.Context, path, token string) (*engineSocket, error) {
	return dialEngineSocketNetwork(ctx, "unix", path, token)
}

// dialEngineSocketNetwork connects to the bridge over the given network ("unix"
// for a socket path, "tcp" for a host:port) and returns its framing wrapper.
func dialEngineSocketNetwork(ctx context.Context, network, address, token string) (*engineSocket, error) {
	var dialer net.Dialer
	conn, err := dialer.DialContext(ctx, network, address)
	if err != nil {
		return nil, fmt.Errorf("engine session: dial %s: %w", network, err)
	}
	return &engineSocket{conn: conn, reader: bufio.NewReader(conn), token: token}, nil
}

// writeRequest writes one newline-delimited request frame, adding the token
// field to the first frame after (re)connect. The token is only marked sent
// once the write succeeds, so a failed first write does not silently drop it.
func (c *engineSocket) writeRequest(payload []byte) error {
	frame := payload
	if !c.tokenSent {
		var request map[string]any
		if err := json.Unmarshal(payload, &request); err != nil {
			return fmt.Errorf("engine session: encode request token: %w", err)
		}
		request["token"] = c.token
		encoded, err := json.Marshal(request)
		if err != nil {
			return fmt.Errorf("engine session: encode request token: %w", err)
		}
		frame = encoded
	}
	frame = append(frame, '\n')
	if _, err := c.conn.Write(frame); err != nil {
		return fmt.Errorf("engine session: write request: %w", err)
	}
	c.tokenSent = true
	return nil
}

// readFrame reads one newline-delimited response frame, reassembling partial
// reads and skipping any non-frame line. It has no read-size cap and no blank
// line terminator: the stream frames one JSON object per newline.
func (c *engineSocket) readFrame() (*responseFrame, error) {
	for {
		line, err := c.reader.ReadString('\n')
		if len(line) > 0 {
			if f := parseFrame(line); f != nil {
				return f, nil
			}
		}
		if err != nil {
			return nil, fmt.Errorf("engine session: read frame: %w", err)
		}
	}
}

func (c *engineSocket) Close() error {
	if err := c.conn.Close(); err != nil {
		return fmt.Errorf("engine session: close socket: %w", err)
	}
	return nil
}

// prepareSocketDir creates the directory that holds the engine socket 0700. The
// directory is the access gate for the socket: Godot creates the socket 0755
// and the bridge cannot chmod it from GDScript.
func prepareSocketDir(socketPath string) error {
	if err := os.MkdirAll(filepath.Dir(socketPath), 0o700); err != nil {
		return fmt.Errorf("engine session: create socket directory: %w", err)
	}
	return nil
}

// removeStaleSocket unlinks a socket or regular file left at path by a previous
// crash, so a fresh bind does not fail with EINVAL.
func removeStaleSocket(socketPath string) error {
	if err := os.Remove(socketPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("engine session: remove stale socket: %w", err)
	}
	return nil
}

// secureSocket narrows the socket to 0600 after the bridge binds it, so its own
// permissions match the private directory that gates access.
func secureSocket(socketPath string) error {
	if err := os.Chmod(socketPath, 0o600); err != nil {
		return fmt.Errorf("engine session: secure socket: %w", err)
	}
	return nil
}
