package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/vilppuvuorinen/slopfood-godot-mcp/internal/daemonpath"
)

func daemonLogPath(root string) (string, error) {
	path, err := daemonpath.LogPath(root)
	if err != nil {
		return "", fmt.Errorf("resolve daemon log path: %w", err)
	}
	return path, nil
}

// Creates the log directory 0700 and opens the log 0600 append-only, so only the owning user can read it.
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
