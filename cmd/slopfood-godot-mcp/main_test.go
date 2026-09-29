package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestRunVersion asserts both roles answer --version without binding a project
// or starting a daemon.
func TestRunVersion(t *testing.T) {
	for _, args := range [][]string{{"--version"}, {daemonSubcommand, "--version"}} {
		var stdout, stderr strings.Builder
		if err := run(args, strings.NewReader(""), &stdout, &stderr); err != nil {
			t.Fatalf("run %v: %v", args, err)
		}
		if got := strings.TrimSpace(stdout.String()); got != version {
			t.Errorf("run %v printed %q, want %q", args, got, version)
		}
	}
}

// TestRunRejectsInvalidLogLevel asserts a bad log level fails before any
// project or signal setup.
func TestRunRejectsInvalidLogLevel(t *testing.T) {
	var stdout, stderr strings.Builder
	if err := run([]string{"--log-level", "loud"}, strings.NewReader(""), &stdout, &stderr); err == nil {
		t.Fatal("run with an invalid log level returned nil, want an error")
	}
}

// TestRunRejectsGDDaemonFlag asserts the removed one-shot opt-in is now just an
// unknown flag: flag.Parse fails, so a stale --gd-daemon invocation cannot
// silently start a daemon.
func TestRunRejectsGDDaemonFlag(t *testing.T) {
	var stdout, stderr strings.Builder
	args := []string{daemonSubcommand, "--gd-daemon", "--project", t.TempDir()}
	if err := run(args, strings.NewReader(""), &stdout, &stderr); err == nil {
		t.Fatalf("run %v returned nil, want an unknown-flag error", args)
	}
}

// TestResolveProjectPrecedence asserts the flag beats the environment, the
// environment beats the working directory, and blank values fall through.
func TestResolveProjectPrecedence(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	t.Setenv(projectEnv, "/tmp/env-project")

	got, err := resolveProject("/tmp/flag-project")
	if err != nil {
		t.Fatalf("resolve flag: %v", err)
	}
	if want := filepath.Clean("/tmp/flag-project"); got != want {
		t.Errorf("flag project = %q, want %q", got, want)
	}

	got, err = resolveProject("")
	if err != nil {
		t.Fatalf("resolve env: %v", err)
	}
	if want := filepath.Clean("/tmp/env-project"); got != want {
		t.Errorf("env project = %q, want %q", got, want)
	}

	t.Setenv(projectEnv, "   ")
	got, err = resolveProject("")
	if err != nil {
		t.Fatalf("resolve cwd: %v", err)
	}
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if want := filepath.Clean(cwd); got != want {
		t.Errorf("cwd project = %q, want %q", got, want)
	}
}

// TestEnableValuesAccumulates asserts the repeatable flag appends rather than
// replacing, which the category resolver depends on.
func TestEnableValuesAccumulates(t *testing.T) {
	var e enableValues
	for _, value := range []string{"scene", "script"} {
		if err := e.Set(value); err != nil {
			t.Fatalf("set %q: %v", value, err)
		}
	}
	if got := e.String(); got != "scene,script" {
		t.Errorf("enable = %q, want scene,script", got)
	}
}
