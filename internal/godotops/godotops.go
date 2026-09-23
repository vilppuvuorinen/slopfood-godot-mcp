// Package godotops embeds the GDScript bridge the scene and UID tools drive
// under `godot --headless --script`, and materializes it for Godot at runtime.
package godotops

import (
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

//go:embed godot_operations.gd
var operationsScript string

//go:embed validate_script.gd
var validateScript string

var (
	once sync.Once
	path string
	err  error
)

var (
	validateOnce sync.Once
	validatePath string
	validateErr  error
)

// userCacheDir is os.UserCacheDir, overridable in tests to keep the shared
// cache out of the real user cache.
var userCacheDir = os.UserCacheDir

// OperationsScript materializes the embedded godot_operations.gd to a file and
// returns its path. Godot needs a real path for --script, and the binary is
// dependency-free so it cannot ship a loose file.
func OperationsScript() (string, error) {
	once.Do(func() {
		path, err = materialize("godot_operations", operationsScript)
	})
	return path, err
}

// ValidateScript materializes the embedded validate_script.gd to a file and
// returns its path. Like OperationsScript, Godot needs a real path for
// --script.
func ValidateScript() (string, error) {
	validateOnce.Do(func() {
		validatePath, validateErr = materialize("validate_script", validateScript)
	})
	return validatePath, validateErr
}

// materialize writes content to a content-addressed file in the shared script
// cache and returns its path. Unlike a per-process temp dir the path is stable
// across processes, so concurrent servers reuse a single copy instead of each
// leaking its own, and different embedded script versions land in different
// files rather than overwriting a running server's script.
//
// The install is atomic: content is written to a temp file and renamed into
// place, so a crash can never expose a partial script, and a concurrent writer
// racing to the same bytes is harmless.
func materialize(name, content string) (string, error) {
	dir, err := cacheDir()
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256([]byte(content))
	dst := filepath.Join(dir, name+"-"+hex.EncodeToString(sum[:8])+".gd")
	if complete(dst, content) {
		return dst, nil
	}

	tmp, err := os.CreateTemp(dir, name+"-*.tmp")
	if err != nil {
		return "", fmt.Errorf("create %s script: %w", name, err)
	}
	tmpName := tmp.Name()
	cleanup := func() {
		tmp.Close()
		os.Remove(tmpName)
	}
	if _, err := tmp.WriteString(content); err != nil {
		cleanup()
		return "", fmt.Errorf("write %s script: %w", name, err)
	}
	if err := tmp.Chmod(0o644); err != nil { //nolint:gosec // script is read-only and lives in a 0700 cache dir.
		cleanup()
		return "", fmt.Errorf("write %s script: %w", name, err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return "", fmt.Errorf("write %s script: %w", name, err)
	}
	if err := os.Rename(tmpName, dst); err != nil {
		// A concurrent server may have won the race; on Windows that makes
		// rename fail rather than overwrite. A complete file is a success.
		if complete(dst, content) {
			os.Remove(tmpName)
			return dst, nil
		}
		os.Remove(tmpName)
		return "", fmt.Errorf("install %s script: %w", name, err)
	}
	return dst, nil
}

// complete reports whether path already holds exactly content. Content
// addressing guarantees a present file was renamed into place whole, so a size
// match is enough to treat it as complete.
func complete(path, content string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular() && info.Size() == int64(len(content))
}

// cacheDir returns the shared, per-user directory that holds materialized
// scripts. It is stable across processes, so the cache never grows per server,
// and its contents are only ever added, never removed. That makes it safe for
// instances to share, including when one exits uncleanly: no server can delete
// or corrupt a file another is reading.
func cacheDir() (string, error) {
	dirs := make([]string, 0, 2)
	if base, err := userCacheDir(); err == nil {
		dirs = append(dirs, filepath.Join(base, "go-dot-mcp", "scripts"))
	}
	dirs = append(dirs, filepath.Join(os.TempDir(), fmt.Sprintf("go-dot-mcp-scripts-%d", os.Getuid())))

	var lastErr error
	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			lastErr = err
			continue
		}
		return dir, nil
	}
	return "", fmt.Errorf("create script cache dir: %w", lastErr)
}
