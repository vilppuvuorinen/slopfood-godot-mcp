package mcp

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
)

// The token prelude is one newline-delimited JSON line the client sends
// immediately after dialing and the daemon reads before it starts the MCP scan.
// It is a transport prelude, not an MCP frame, so the MCP stream never sees it.
// The read is bounded in both time and bytes so a silent client cannot grow the
// buffer without bound or pin an accept indefinitely.
const (
	maxTokenPreludeBytes = 4096
	tokenReadDeadline    = 5 * time.Second
)

// errTCPTokenRejected reports that a connection's token prelude was missing,
// malformed, or did not match the daemon's token.
var errTCPTokenRejected = errors.New("tcp: token rejected")

// endpointRecord is the JSON the daemon publishes after binding and the client
// reads to dial. It carries the per-spawn token, so the endpoint file must live
// in the same private runtime directory the Unix socket uses and be written
// 0600.
type endpointRecord struct {
	Transport string `json:"transport"`
	Host      string `json:"host"`
	Port      int    `json:"port"`
	Token     string `json:"token"`
}

// ensureEndpointDir creates path's parent directory 0700 and tightens a
// pre-existing one, mirroring ListenUnix's directory gate: the endpoint file
// carries the token, so its directory must be private.
func ensureEndpointDir(path string) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create tcp endpoint directory %q: %w", dir, err)
	}
	if err := os.Chmod(dir, 0o700); err != nil {
		return fmt.Errorf("tighten tcp endpoint directory %q: %w", dir, err)
	}
	return nil
}

// TCPListener is the loopback-TCP backend for the Listener and ListenFunc seams.
// It binds 127.0.0.1:0, generates the per-spawn token, and publishes the
// endpoint record atomically. It owns the exclusive lock and the endpoint file:
// Close closes the listener, removes the endpoint file while the lock is still
// held, then releases the lock, so cleanup has a single owner and no successor
// can lose the file it just republished.
type TCPListener struct {
	ln           net.Listener
	endpointPath string
	lock         io.Closer
	token        string
	once         sync.Once
	err          error
}

// ListenTCP opens a loopback-TCP Listener, takes ownership of lock, and
// publishes endpointPath. addr is ignored: the transport always binds an
// ephemeral port on 127.0.0.1 so the endpoint record, not a fixed port, is the
// rendezvous. It creates the runtime directory 0700 and writes the endpoint
// 0600, mirroring ListenUnix's directory gate. The caller opens the lock
// separately (OpenTCPLock) so the OS-specific handle lives behind its own
// build-tagged helper.
func ListenTCP(_ string, endpointPath string, lock io.Closer) (*TCPListener, error) {
	if err := ensureEndpointDir(endpointPath); err != nil {
		return nil, err
	}

	ln, err := net.Listen("tcp", "127.0.0.1:0") //nolint:noctx // the ListenFunc seam carries no context; binding is local and fast.
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

// Port reports the bound ephemeral port, so an endpoint factory can log or
// verify it without naming the listener type.
func (l *TCPListener) Port() int {
	if addr, ok := l.ln.Addr().(*net.TCPAddr); ok {
		return addr.Port
	}
	return 0
}

// Accept blocks until a client connects, returning the connection with its
// token pre-auth available.
func (l *TCPListener) Accept() (Connection, error) {
	conn, err := l.ln.Accept()
	if err != nil {
		return nil, fmt.Errorf("accept tcp: %w", err)
	}
	return &tcpConn{Conn: conn, reader: bufio.NewReader(conn), wantToken: l.token}, nil
}

// Close stops accepting, closes the listener, removes the endpoint file while
// the lock is still held, then releases the lock. It is idempotent and joins
// any errors it hits.
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

// tcpConn is one accepted TCP connection: the byte stream plus the bounded
// token pre-auth. The buffered reader is shared between Authenticate and Read,
// so any MCP bytes already buffered behind the token line are not dropped.
type tcpConn struct {
	net.Conn
	reader        *bufio.Reader
	wantToken     string
	authenticated bool
}

// Read drains the shared buffered reader, which Authenticate primed.
func (c *tcpConn) Read(p []byte) (int, error) {
	n, err := c.reader.Read(p)
	if err != nil {
		return n, fmt.Errorf("read tcp: %w", err)
	}
	return n, nil
}

// Authenticate consumes the token prelude: one newline-delimited JSON line read
// under a deadline and a fixed byte cap. It returns errTCPTokenRejected for a
// missing, malformed, or mismatched token, and a read error on failure.
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

// Authenticated reports whether the token prelude was accepted.
func (c *tcpConn) Authenticated() bool { return c.authenticated }

// PeerIdentity reports errPeerIdentityAbsent: a loopback port exposes no kernel
// credentials and no filesystem permission gate, so the daemon requires the
// token pre-auth instead. It is deliberately distinct from
// errPeerIdentityUnsupported, which would otherwise admit the connection.
func (c *tcpConn) PeerIdentity() (PeerIdentity, error) {
	return PeerIdentity{}, errPeerIdentityAbsent
}

func (c *tcpConn) Close() error {
	if err := c.Conn.Close(); err != nil {
		return fmt.Errorf("close tcp conn: %w", err)
	}
	return nil
}

// DialTCP reads the endpoint record at endpointPath, dials the daemon, and
// sends the token prelude. A missing endpoint file or a failed dial means no
// daemon is listening yet, which is what drives the client's spawn path. The
// returned connection is a plain io.ReadWriteCloser; the client's MCP bytes
// follow the prelude on the same stream.
func DialTCP(endpointPath string) (io.ReadWriteCloser, error) {
	record, err := readEndpoint(endpointPath)
	if err != nil {
		return nil, err
	}
	address := net.JoinHostPort(record.Host, strconv.Itoa(record.Port))
	conn, err := net.Dial("tcp", address) //nolint:noctx // the DialFunc seam carries no context; the dial is local and fast.
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

// newSessionToken returns a 128-bit random hex token, regenerated on every
// spawn and never placed in argv or the environment.
func newSessionToken() (string, error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generate session token: %w", err)
	}
	return hex.EncodeToString(buf), nil
}

// publishEndpoint writes record to path atomically via temp-file + rename, so a
// reader never sees a partial record. The file is 0600 because it carries the
// token.
func publishEndpoint(path string, record endpointRecord) error {
	data, err := json.Marshal(record)
	if err != nil {
		return fmt.Errorf("encode endpoint record: %w", err)
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".tmp")
	if err != nil {
		return fmt.Errorf("create endpoint temp file: %w", err)
	}
	tmpName := tmp.Name()
	cleanup := func() {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
	}
	if _, err := tmp.Write(data); err != nil {
		cleanup()
		return fmt.Errorf("write endpoint record: %w", err)
	}
	if err := tmp.Chmod(0o600); err != nil {
		cleanup()
		return fmt.Errorf("secure endpoint record: %w", err)
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName)
		return fmt.Errorf("close endpoint record: %w", err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		_ = os.Remove(tmpName)
		return fmt.Errorf("install endpoint record: %w", err)
	}
	return nil
}

// readEndpoint reads and decodes the endpoint record at path, mapping a missing
// file to a dial miss the caller turns into a spawn.
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

// readBoundedLine reads one newline-terminated line from r, refusing to buffer
// more than max bytes, so a client that never sends a newline cannot grow the
// buffer without bound. It returns the line without the trailing newline.
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

// trimNewline drops a trailing "\n" or "\r\n".
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
	_ PortNamer       = (*TCPListener)(nil)
	_ TokenConnection = (*tcpConn)(nil)
)

// PortNamer is implemented by a Listener that binds an ephemeral port and can
// report it, so an endpoint factory can publish it without naming the concrete
// listener type.
type PortNamer interface {
	Port() int
}
