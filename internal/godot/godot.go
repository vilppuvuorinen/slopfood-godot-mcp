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
	PathEnv = "GODOT_PATH"

	DefaultMinor = "4.7"
)

func engineGlobs(goos string) []string {
	switch goos {
	case "windows":
		// Prefer the console-subsystem build: its stdout survives a pipe, which the readiness line depends on.
		return []string{"Godot_v*_win64_console.exe", "Godot_v*_win64.exe", "Godot_v*_win32.exe"}
	case "darwin":
		return []string{"Godot.app/Contents/MacOS/Godot", "Godot_v*_macos.*"}
	default:
		return []string{"Godot_v*_linux.*"}
	}
}

var (
	localEngineOnce sync.Once
	localEngine     string
)

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

// Windows file modes are not meaningful, so there a regular file is enough.
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
