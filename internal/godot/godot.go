// Package godot locates and invokes the Godot executable.
package godot

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const (
	// PathEnv overrides the Godot executable used by the server.
	PathEnv = "GODOT_PATH"

	// DefaultMinor is the Godot minor version used when PathEnv is unset. The
	// matching build is looked up under the repository-local .godot directory.
	DefaultMinor = "4.7"

	binaryGlob = "Godot_v*_linux.x86_64"
)

// Path resolves the Godot executable to use. An explicit PathEnv value wins;
// otherwise the repository-local DefaultMinor build is used, falling back to
// "godot" on PATH.
func Path() string {
	if p := strings.TrimSpace(os.Getenv(PathEnv)); p != "" {
		return p
	}
	if p := findLocal(); p != "" {
		return p
	}
	return "godot"
}

// Version runs "<path> --version" and returns the trimmed stdout.
func Version(ctx context.Context, path string) (string, error) {
	cmd := exec.CommandContext(ctx, path, "--version") //nolint:gosec // path is operator-controlled via GODOT_PATH.
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
// a .godot/<DefaultMinor>/Godot_v*_linux.x86_64 build.
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
	matches, err := filepath.Glob(filepath.Join(dir, ".godot", DefaultMinor, binaryGlob))
	if err != nil {
		return ""
	}
	for _, m := range matches {
		if info, err := os.Stat(m); err == nil && !info.IsDir() {
			return m
		}
	}
	return ""
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
