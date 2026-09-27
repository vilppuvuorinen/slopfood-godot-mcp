package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/vilppuvuorinen/slopfood-godot-mcp/internal/daemonpath"
)

// daemonLogPath derives the per-project daemon log path from the shared
// daemonpath resolver, so the log and the socket agree on the runtime
// directory and the project hash.
func daemonLogPath(root string) (string, error) {
	path, err := daemonpath.LogPath(root)
	if err != nil {
		return "", fmt.Errorf("resolve daemon log path: %w", err)
	}
	return path, nil
}

// openDaemonLog creates the log's parent directory if needed and opens the log
// for appending. The directory is 0700 and the file 0600, so only the owning
// user can read a daemon's log.
func openDaemonLog(path string) (*os.File, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, fmt.Errorf("create log directory for %q: %w", path, err)
	}
	//nolint:gosec // path is derived from the bound project's runtime directory, not user input.
	logFile, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open log file %q: %w", path, err)
	}
	return logFile, nil
}
