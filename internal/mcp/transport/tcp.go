package transport

import (
	"bufio"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	"github.com/vilppuvuorinen/slopfood-godot-mcp/internal/fsutil"
)

// One newline-delimited JSON line sent before the MCP stream; the read is
// bounded in time and bytes so a silent client cannot pin an accept.
const (
	maxTokenPreludeBytes = 4096
	tokenReadDeadline    = 5 * time.Second
)

var errTCPTokenRejected = errors.New("tcp: token rejected")

// Published by the daemon and read by the client; carries the token, so it must
// live in a private directory and be written 0600.
type endpointRecord struct {
	Transport string `json:"transport"`
	Host      string `json:"host"`
	Port      int    `json:"port"`
	Token     string `json:"token"`
}

// Loopback-TCP backend: binds 127.0.0.1:0, generates the token, and publishes
// the endpoint record; Close removes the file before releasing the lock.
type TCPListener struct {
	ln           net.Listener
	endpointPath string
	lock         io.Closer
	token        string
	once         sync.Once
	err          error
}

// Binds an ephemeral 127.0.0.1 port and publishes endpointPath; addr is
// ignored. The caller opens the lock separately so the OS handle stays build-tagged.
func ListenTCP(_ string, endpointPath string, lock io.Closer) (*TCPListener, error) {
	if err := fsutil.EnsurePrivateDir(filepath.Dir(endpointPath)); err != nil {
		return nil, fmt.Errorf("prepare tcp endpoint directory: %w", err)
	}

	ln, err := net.Listen("tcp", "127.0.0.1:0") //nolint:noctx // binding a local loopback port is fast and carries no context.
	if err != nil {
		return nil, fmt.Errorf("listen tcp: %w", err)
	}
	tcpAddr, ok := ln.Addr().(*net.TCPAddr)
	if !ok {
		_ = ln.Close()
		return nil, fmt.Errorf("listen tcp: unexpected address type %T", ln.Addr())
	}
	token, err := newSessionToken()
	if err != nil {
		_ = ln.Close()
		return nil, err
	}
	record := endpointRecord{Transport: "tcp", Host: "127.0.0.1", Port: tcpAddr.Port, Token: token}
	if err := publishEndpoint(endpointPath, record); err != nil {
		_ = ln.Close()
		return nil, err
	}
	return &TCPListener{ln: ln, endpointPath: endpointPath, lock: lock, token: token}, nil
}

func (l *TCPListener) Port() int {
	if addr, ok := l.ln.Addr().(*net.TCPAddr); ok {
		return addr.Port
	}
	return 0
}

func (l *TCPListener) Accept() (Connection, error) {
	conn, err := l.ln.Accept()
	if err != nil {
		return nil, fmt.Errorf("accept tcp: %w", err)
	}
	return &tcpConn{Conn: conn, reader: bufio.NewReader(conn), wantToken: l.token}, nil
}

// Idempotent; removes the endpoint file while the lock is still held, then releases it.
func (l *TCPListener) Close() error {
	l.once.Do(func() {
		var errs []error
		if err := l.ln.Close(); err != nil {
			errs = append(errs, fmt.Errorf("close tcp listener: %w", err))
		}
		if err := os.Remove(l.endpointPath); err != nil && !os.IsNotExist(err) {
			errs = append(errs, fmt.Errorf("remove tcp endpoint %q: %w", l.endpointPath, err))
		}
		if l.lock != nil {
			if err := l.lock.Close(); err != nil {
				errs = append(errs, fmt.Errorf("close tcp lock: %w", err))
			}
		}
		l.err = errors.Join(errs...)
	})
	return l.err
}

// The buffered reader is shared between Authenticate and Read, so MCP bytes
// buffered behind the token line are not dropped.
type tcpConn struct {
	net.Conn
	reader        *bufio.Reader
	wantToken     string
	authenticated bool
}

func (c *tcpConn) Read(p []byte) (int, error) {
	n, err := c.reader.Read(p)
	if err != nil {
		return n, fmt.Errorf("read tcp: %w", err)
	}
	return n, nil
}

// Reads one bounded, deadlined token prelude line; returns errTCPTokenRejected
// for a missing, malformed, or mismatched token.
func (c *tcpConn) Authenticate() error {
	_ = c.SetReadDeadline(time.Now().Add(tokenReadDeadline))
	defer func() { _ = c.SetReadDeadline(time.Time{}) }()

	line, err := readBoundedLine(c.reader, maxTokenPreludeBytes)
	if err != nil {
		return fmt.Errorf("tcp token prelude: %w", err)
	}
	var record endpointRecord
	if err := json.Unmarshal([]byte(line), &record); err != nil {
		return fmt.Errorf("%w: malformed prelude", errTCPTokenRejected)
	}
	if record.Token == "" || record.Token != c.wantToken {
		return errTCPTokenRejected
	}
	c.authenticated = true
	return nil
}

func (c *tcpConn) Authenticated() bool { return c.authenticated }

// A loopback port exposes no kernel credentials or filesystem gate, so the
// daemon requires the token pre-auth instead.
func (c *tcpConn) PeerIdentity() (PeerIdentity, error) {
	return PeerIdentity{}, ErrPeerIdentityAbsent
}

func (c *tcpConn) Close() error {
	if err := c.Conn.Close(); err != nil {
		return fmt.Errorf("close tcp conn: %w", err)
	}
	return nil
}

// Reads the endpoint record, dials, and sends the token prelude; a missing file
// or failed dial drives the client's spawn path.
func DialTCP(endpointPath string) (io.ReadWriteCloser, error) {
	record, err := readEndpoint(endpointPath)
	if err != nil {
		return nil, err
	}
	address := net.JoinHostPort(record.Host, strconv.Itoa(record.Port))
	conn, err := net.Dial("tcp", address) //nolint:noctx // the dial is local and fast and carries no context.
	if err != nil {
		return nil, fmt.Errorf("dial tcp %q: %w", address, err)
	}
	prelude, err := json.Marshal(endpointRecord{Token: record.Token})
	if err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("encode tcp prelude: %w", err)
	}
	if _, err := conn.Write(append(prelude, '\n')); err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("send tcp prelude: %w", err)
	}
	return conn, nil
}

// 128-bit random hex, regenerated per spawn and never placed in argv or the environment.
func newSessionToken() (string, error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generate session token: %w", err)
	}
	return hex.EncodeToString(buf), nil
}

// Writes atomically via temp-file + rename; 0600 since it carries the token.
func publishEndpoint(path string, record endpointRecord) error {
	data, err := json.Marshal(record)
	if err != nil {
		return fmt.Errorf("encode endpoint record: %w", err)
	}
	if err := fsutil.Write(path, data, 0o600); err != nil {
		return fmt.Errorf("publish endpoint record: %w", err)
	}
	return nil
}

// A missing file maps to a dial miss the caller turns into a spawn.
func readEndpoint(path string) (endpointRecord, error) {
	data, err := os.ReadFile(path) //nolint:gosec // the path is derived from the bound project, not user input.
	if err != nil {
		return endpointRecord{}, fmt.Errorf("read tcp endpoint %q: %w", path, err)
	}
	var record endpointRecord
	if err := json.Unmarshal(data, &record); err != nil {
		return endpointRecord{}, fmt.Errorf("decode tcp endpoint %q: %w", path, err)
	}
	return record, nil
}

// Refuses to buffer more than max bytes so a client that never sends a newline
// cannot grow the buffer without bound.
func readBoundedLine(r *bufio.Reader, max int) (string, error) {
	var line []byte
	for {
		chunk, err := r.ReadSlice('\n')
		line = append(line, chunk...)
		if len(line) > max {
			return "", fmt.Errorf("token prelude exceeds %d bytes", max)
		}
		if err == nil {
			return string(trimNewline(line)), nil
		}
		if errors.Is(err, bufio.ErrBufferFull) {
			continue
		}
		return "", fmt.Errorf("read token prelude line: %w", err)
	}
}

func trimNewline(line []byte) []byte {
	line = trimSuffixByte(line, '\n')
	return trimSuffixByte(line, '\r')
}

func trimSuffixByte(line []byte, b byte) []byte {
	if len(line) > 0 && line[len(line)-1] == b {
		return line[:len(line)-1]
	}
	return line
}

var (
	_ Connection      = (*tcpConn)(nil)
	_ Listener        = (*TCPListener)(nil)
	_ TokenConnection = (*tcpConn)(nil)
)
