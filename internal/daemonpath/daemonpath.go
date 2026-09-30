// Package daemonpath derives the per-project paths a daemon and its thin client
// share: the Unix socket the daemon listens on and the log file it writes to.
package daemonpath

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
)

const dirName = "slopfood-godot-mcp"

// RuntimeDir returns the directory holding per-project daemon sockets and logs.
// It is not created here; the Unix backend creates it 0700 when it binds.
func RuntimeDir() (string, error) {
	if runtimeDir := os.Getenv("XDG_RUNTIME_DIR"); runtimeDir != "" {
		return filepath.Join(runtimeDir, dirName), nil
	}
	cacheDir, err := os.UserCacheDir()
	if err != nil {
		return "", fmt.Errorf("resolve user cache directory: %w", err)
	}
	return filepath.Join(cacheDir, dirName), nil
}

// ProjectHash returns the first 16 hex characters of the cleaned root's
// SHA-256, the shared name component of a project's socket and log.
func ProjectHash(root string) string {
	sum := sha256.Sum256([]byte(filepath.Clean(root)))
	return hex.EncodeToString(sum[:8])
}

func SocketPath(root string) (string, error) {
	dir, err := RuntimeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, ProjectHash(root)+".sock"), nil
}

// EngineSocketPath returns the bridge's socket path; it shares the runtime
// directory and project hash with SocketPath but uses a distinct .engine.sock
// suffix, short enough to stay under the Linux sun_path limit.
func EngineSocketPath(root string) (string, error) {
	dir, err := RuntimeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, ProjectHash(root)+".engine.sock"), nil
}

func LogPath(root string) (string, error) {
	dir, err := RuntimeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, ProjectHash(root)+".log"), nil
}

// EndpointPath returns the TCP transport's endpoint record path. The record
// carries the per-spawn token, so the runtime directory must be 0700 and the
// file 0600; the lock file is EndpointPath(root)+".lock".
func EndpointPath(root string) (string, error) {
	dir, err := RuntimeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, ProjectHash(root)+".endpoint"), nil
}
