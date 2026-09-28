// Package daemonpath derives the per-project paths a daemon and its thin client
// share: the Unix socket the daemon listens on and the log file it writes to.
// Deriving them from the project root means a client that knows the project
// finds the same daemon with no extra configuration, and two projects never
// collide.
package daemonpath

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
)

// dirName is the subdirectory under the runtime root that holds every daemon
// socket and log.
const dirName = "slopfood-godot-mcp"

// RuntimeDir returns the directory that holds per-project daemon sockets and
// logs. It is $XDG_RUNTIME_DIR/slopfood-godot-mcp when XDG_RUNTIME_DIR is set,
// and <os.UserCacheDir()>/slopfood-godot-mcp otherwise. The directory is not
// created here; the Unix backend creates it 0700 when it binds the socket.
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

// ProjectHash returns the first 16 hex characters of the SHA-256 of the cleaned
// project root. It is the shared name component of a project's socket and log,
// so both are stable for a project and distinct across projects.
func ProjectHash(root string) string {
	sum := sha256.Sum256([]byte(filepath.Clean(root)))
	return hex.EncodeToString(sum[:8])
}

// SocketPath returns the Unix socket path for the daemon bound to root.
func SocketPath(root string) (string, error) {
	dir, err := RuntimeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, ProjectHash(root)+".sock"), nil
}

// EngineSocketPath returns the Unix socket path the engine operations bridge
// bound to root listens on. It shares the runtime directory and project hash
// with SocketPath but carries a distinct .engine.sock suffix, so the bridge
// and the MCP daemon never collide. The suffix is short enough to stay under
// the Linux sun_path limit for the same runtime directory SocketPath assumes.
func EngineSocketPath(root string) (string, error) {
	dir, err := RuntimeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, ProjectHash(root)+".engine.sock"), nil
}

// LogPath returns the log file path for the daemon bound to root.
func LogPath(root string) (string, error) {
	dir, err := RuntimeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, ProjectHash(root)+".log"), nil
}
