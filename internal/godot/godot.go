// Package godot locates and invokes the Godot executable.
package godot

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
)

const (
	// PathEnv overrides the Godot executable used by the server.
	PathEnv = "GODOT_PATH"

	// DefaultMinor is the Godot minor version used when PathEnv is unset. The
	// matching build is looked up under the repository-local .godot directory.
	DefaultMinor = "4.7"
)

// engineGlobs returns the glob patterns, relative to a .godot/<minor> directory,
// that name the official Godot editor for goos. The patterns are OS-specific
// because official binary names differ and macOS ships an app bundle. Matching
// another OS's build would resolve to a path that cannot execute, so falling
// through to "godot" on PATH is preferable.
func engineGlobs(goos string) []string {
	switch goos {
	case "windows":
		// Prefer the console-subsystem build: its stdout survives a pipe, which
		// the readiness line and --version depend on. The plain GUI build is a
		// fallback for a hand-placed engine without the console sibling.
		return []string{"Godot_v*_win64_console.exe", "Godot_v*_win64.exe", "Godot_v*_win32.exe"}
	case "darwin":
		return []string{"Godot.app/Contents/MacOS/Godot", "Godot_v*_macos.*"}
	default:
		return []string{"Godot_v*_linux.*"}
	}
}

// localEngine caches the repository-local lookup so Path does not walk the
// filesystem on every engine call. The working directory and executable are
// fixed for the process's lifetime, so it is computed once. An explicit PathEnv
// is never cached, so an operator or test can still repoint the engine at any
// time.
var (
	localEngineOnce sync.Once
	localEngine     string
)

// Path resolves the Godot executable to use. An explicit PathEnv value wins;
// otherwise the repository-local DefaultMinor build is used, falling back to
// "godot" on PATH.
func Path() string {
	if p := strings.TrimSpace(os.Getenv(PathEnv)); p != "" {
		return p
	}
	localEngineOnce.Do(func() { localEngine = findLocal() })
	if localEngine != "" {
		return localEngine
	}
	return "godot"
}

// Version runs "<path> --version" and returns the trimmed stdout.
func Version(ctx context.Context, path string) (string, error) {
	cmd := newCommand(ctx, path, "--version")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return "", fmt.Errorf("running %s --version: %s", path, msg)
	}
	return strings.TrimSpace(stdout.String()), nil
}

// findLocal searches upward from the working directory and the executable for
// a .godot/<DefaultMinor> build matching the host OS.
func findLocal() string {
	return findIn(searchRoots())
}

func findIn(roots []string) string {
	for _, root := range roots {
		dir := root
		for {
			if p := globMinor(dir); p != "" {
				return p
			}
			parent := filepath.Dir(dir)
			if parent == dir {
				break
			}
			dir = parent
		}
	}
	return ""
}

func globMinor(dir string) string {
	base := filepath.Join(dir, ".godot", DefaultMinor)
	for _, pattern := range engineGlobs(runtime.GOOS) {
		matches, err := filepath.Glob(filepath.Join(base, pattern))
		if err != nil {
			continue
		}
		for _, m := range matches {
			if executableFile(m) {
				return m
			}
		}
	}
	return ""
}

// executableFile reports whether path is a regular file that can be run: regular
// everywhere, and carrying an execute bit on non-Windows. Windows file modes are
// not meaningful, so there a regular file is enough.
func executableFile(path string) bool {
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() {
		return false
	}
	if runtime.GOOS == "windows" {
		return true
	}
	return info.Mode().Perm()&0o111 != 0
}

func searchRoots() []string {
	roots := make([]string, 0, 2)
	if wd, err := os.Getwd(); err == nil {
		roots = append(roots, wd)
	}
	if exe, err := os.Executable(); err == nil {
		if resolved, err := filepath.EvalSymlinks(exe); err == nil {
			exe = resolved
		}
		roots = append(roots, filepath.Dir(exe))
	}
	return roots
}
