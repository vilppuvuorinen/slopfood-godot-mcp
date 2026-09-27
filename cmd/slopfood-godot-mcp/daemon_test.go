package main

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

// TestDaemonLogPath asserts the daemon log path is a stable per-project name
// under the runtime directory, distinct across projects.
func TestDaemonLogPath(t *testing.T) {
	runtimeDir := t.TempDir()
	t.Setenv("XDG_RUNTIME_DIR", runtimeDir)

	first, err := daemonLogPath("/tmp/project-a")
	if err != nil {
		t.Fatalf("daemonLogPath: %v", err)
	}
	again, err := daemonLogPath("/tmp/project-a")
	if err != nil {
		t.Fatalf("daemonLogPath again: %v", err)
	}
	if first != again {
		t.Errorf("daemonLogPath not deterministic: %q, %q", first, again)
	}
	other, err := daemonLogPath("/tmp/project-b")
	if err != nil {
		t.Fatalf("daemonLogPath other: %v", err)
	}
	if first == other {
		t.Errorf("daemonLogPath collided across projects: %q", first)
	}
	wantDir := filepath.Join(runtimeDir, "slopfood-godot-mcp")
	if got := filepath.Dir(first); got != wantDir {
		t.Errorf("log dir = %q, want %q", got, wantDir)
	}
	if !strings.HasSuffix(first, ".log") {
		t.Errorf("log path %q does not end in .log", first)
	}
}

// TestOpenDaemonLog asserts the log directory is created 0700, the file 0600,
// and existing content is appended to rather than truncated.
func TestOpenDaemonLog(t *testing.T) {
	path := filepath.Join(t.TempDir(), "logs", "daemon.log")

	first, openErr := openDaemonLog(path)
	if openErr != nil {
		t.Fatalf("openDaemonLog: %v", openErr)
	}
	if _, writeErr := first.WriteString("one\n"); writeErr != nil {
		t.Fatalf("write: %v", writeErr)
	}
	if closeErr := first.Close(); closeErr != nil {
		t.Fatalf("close: %v", closeErr)
	}

	if info, statErr := os.Stat(filepath.Dir(path)); statErr != nil {
		t.Fatalf("stat dir: %v", statErr)
	} else if got := info.Mode().Perm(); got != 0o700 {
		t.Errorf("log dir mode = %o, want 700", got)
	}
	if info, statErr := os.Stat(path); statErr != nil {
		t.Fatalf("stat file: %v", statErr)
	} else if got := info.Mode().Perm(); got != 0o600 {
		t.Errorf("log file mode = %o, want 600", got)
	}

	second, reopenErr := openDaemonLog(path)
	if reopenErr != nil {
		t.Fatalf("reopen: %v", reopenErr)
	}
	if _, writeErr := second.WriteString("two\n"); writeErr != nil {
		t.Fatalf("append: %v", writeErr)
	}
	if closeErr := second.Close(); closeErr != nil {
		t.Fatalf("close second: %v", closeErr)
	}

	data, readErr := os.ReadFile(path)
	if readErr != nil {
		t.Fatalf("read: %v", readErr)
	}
	if got := string(data); got != "one\ntwo\n" {
		t.Errorf("log content = %q, want %q", got, "one\ntwo\n")
	}
}

// TestDaemonChildArgs asserts the detached child argv carries the bound project,
// the log level, the log file, the idle timeout, and every enable value.
func TestDaemonChildArgs(t *testing.T) {
	opts := &options{
		logLevel:    "debug",
		enable:      enableValues{"scene", "script"},
		idleTimeout: 90 * time.Second,
	}
	got := daemonChildArgs("/tmp/project", opts, "/tmp/project.log")
	want := []string{
		daemonSubcommand,
		"--project", "/tmp/project",
		"--log-level", "debug",
		"--log-file", "/tmp/project.log",
		"--idle-timeout", "1m30s",
		"--enable", "scene",
		"--enable", "script",
	}
	if !slices.Equal(got, want) {
		t.Errorf("daemonChildArgs = %q, want %q", got, want)
	}
}

// TestRunDaemonRejectsNegativeIdleTimeout asserts a negative idle timeout fails
// fast instead of starting a daemon.
func TestRunDaemonRejectsNegativeIdleTimeout(t *testing.T) {
	var stdout, stderr strings.Builder
	if err := run([]string{daemonSubcommand, "--idle-timeout", "-1s"}, strings.NewReader(""), &stdout, &stderr); err == nil {
		t.Fatal("run with a negative idle timeout returned nil, want an error")
	}
}

// TestRunDaemonDetach asserts the detach path resolves the executable and hands
// the child seam the cleaned project and the derived per-project log path.
func TestRunDaemonDetach(t *testing.T) {
	runtimeDir := t.TempDir()
	t.Setenv("XDG_RUNTIME_DIR", runtimeDir)

	originalExe, originalStart := executablePath, startDetachedDaemon
	t.Cleanup(func() {
		executablePath = originalExe
		startDetachedDaemon = originalStart
	})

	executablePath = func() (string, error) { return "/fake/exe", nil }
	var gotExe, gotRoot, gotLogPath string
	startDetachedDaemon = func(exe, root string, _ *options, logPath string) error {
		gotExe, gotRoot, gotLogPath = exe, root, logPath
		return nil
	}

	project := filepath.Join(t.TempDir(), "unclean", "..")
	wantRoot, err := resolveProject(project)
	if err != nil {
		t.Fatalf("resolveProject: %v", err)
	}
	wantLog, err := daemonLogPath(wantRoot)
	if err != nil {
		t.Fatalf("daemonLogPath: %v", err)
	}

	var stdout, stderr strings.Builder
	args := []string{daemonSubcommand, "--detach", "--project", project, "--enable", "scene"}
	if err := run(args, strings.NewReader(""), &stdout, &stderr); err != nil {
		t.Fatalf("run detach: %v", err)
	}
	if gotExe != "/fake/exe" {
		t.Errorf("exe = %q, want /fake/exe", gotExe)
	}
	if gotRoot != wantRoot {
		t.Errorf("root = %q, want %q", gotRoot, wantRoot)
	}
	if gotLogPath != wantLog {
		t.Errorf("logPath = %q, want %q", gotLogPath, wantLog)
	}
}
