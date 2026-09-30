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

// The token travels over the parent's private stdout pipe rather than argv or the environment.
type readyInfo struct {
	Ready     bool   `json:"ready"`
	Transport string `json:"transport"`
	Path      string `json:"path"`
	Host      string `json:"host"`
	Port      int    `json:"port"`
	Token     string `json:"token"`
}

const startupReadyTimeout = 30 * time.Second

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

// The deadline is enforced by canceling ctx, which closes the pipe under the reader.
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

// The token is presented on the first request frame of each connection.
type engineSocket struct {
	conn      net.Conn
	reader    *bufio.Reader
	token     string
	tokenSent bool
}

func dialEngineSocket(ctx context.Context, path, token string) (*engineSocket, error) {
	return dialEngineSocketNetwork(ctx, "unix", path, token)
}

func dialEngineSocketNetwork(ctx context.Context, network, address, token string) (*engineSocket, error) {
	var dialer net.Dialer
	conn, err := dialer.DialContext(ctx, network, address)
	if err != nil {
		return nil, fmt.Errorf("engine session: dial %s: %w", network, err)
	}
	return &engineSocket{conn: conn, reader: bufio.NewReader(conn), token: token}, nil
}

// The token is only marked sent once the write succeeds, so a failed first write does not drop it.
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

// The stream frames one JSON object per newline, with no read-size cap or blank-line terminator.
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

// The directory is the access gate for the socket: Godot creates the socket 0755 and GDScript cannot chmod it.
func prepareSocketDir(socketPath string) error {
	if err := os.MkdirAll(filepath.Dir(socketPath), 0o700); err != nil {
		return fmt.Errorf("engine session: create socket directory: %w", err)
	}
	return nil
}

// Unlinks a stale file left by a previous crash so a fresh bind does not fail with EINVAL.
func removeStaleSocket(socketPath string) error {
	if err := os.Remove(socketPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("engine session: remove stale socket: %w", err)
	}
	return nil
}

// Narrows the socket to 0600 after the bridge binds it, matching the private directory that gates access.
func secureSocket(socketPath string) error {
	if err := os.Chmod(socketPath, 0o600); err != nil {
		return fmt.Errorf("engine session: secure socket: %w", err)
	}
	return nil
}
