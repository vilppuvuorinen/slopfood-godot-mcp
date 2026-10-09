// Package fsutil holds the secure-filesystem helpers shared by the daemon
// transports, the engine bridge, and script materialization: private
// directories, atomic file installs, and missing-file-tolerant removal.
package fsutil

import (
	"fmt"
	"os"
	"path/filepath"
)

// EnsurePrivateDir creates dir if needed and tightens it to 0700. MkdirAll
// leaves an existing directory's mode alone, so the chmod is what makes the
// directory private even if an older version created it world-readable.
func EnsurePrivateDir(dir string) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create directory %q: %w", dir, err)
	}
	if err := os.Chmod(dir, 0o700); err != nil {
		return fmt.Errorf("tighten directory %q: %w", dir, err)
	}
	return nil
}

// RemoveIfExists removes path, ignoring a missing file.
func RemoveIfExists(path string) error {
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove %q: %w", path, err)
	}
	return nil
}

// Write installs data at path atomically: it stages the bytes in a temp file in
// path's directory and renames it into place, so a concurrent reader never sees
// a partial file.
func Write(path string, data []byte, mode os.FileMode) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".*.tmp")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	tmpName := tmp.Name()
	cleanup := func() {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
	}
	if _, err := tmp.Write(data); err != nil {
		cleanup()
		return fmt.Errorf("write temp file: %w", err)
	}
	if err := tmp.Chmod(mode); err != nil {
		cleanup()
		return fmt.Errorf("chmod temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName)
		return fmt.Errorf("close temp file: %w", err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		_ = os.Remove(tmpName)
		return fmt.Errorf("install file: %w", err)
	}
	return nil
}
