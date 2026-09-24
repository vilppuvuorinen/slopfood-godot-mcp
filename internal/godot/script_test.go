package godot

import (
	"context"
	"strings"
	"testing"
)

// TestRunScriptCapturesStreamsSeparately confirms stdout, stderr, and the exit
// code are not merged, so a caller can combine them.
func TestRunScriptCapturesStreamsSeparately(t *testing.T) {
	bin := writeExecutable(t, "echo out\necho 'ERROR: boom' >&2\nexit 0")

	stdout, stderr, exitCode, err := runScript(context.Background(), bin, nil)
	if err != nil {
		t.Fatalf("runScript: %v", err)
	}
	if stdout != "out\n" {
		t.Errorf("stdout = %q, want %q", stdout, "out\n")
	}
	if stderr != "ERROR: boom\n" {
		t.Errorf("stderr = %q, want %q", stderr, "ERROR: boom\n")
	}
	if exitCode != 0 {
		t.Errorf("exitCode = %d, want 0", exitCode)
	}
}

// TestCheckScriptCapturesExitCodeAndStreams confirms a non-zero exit is not a
// wrapper error and the exit code and streams are all returned.
func TestCheckScriptCapturesExitCodeAndStreams(t *testing.T) {
	bin := writeExecutable(t, "echo out\necho 'SCRIPT ERROR: boom' >&2\nexit 1")

	stdout, stderr, exitCode, err := CheckScript(context.Background(), bin, "/proj", "/checker.gd", "res://player.gd")
	if err != nil {
		t.Fatalf("CheckScript: %v, want nil", err)
	}
	if exitCode != 1 {
		t.Errorf("exitCode = %d, want 1", exitCode)
	}
	if strings.Contains(stdout, "SCRIPT ERROR") || !strings.Contains(stdout, "out") {
		t.Errorf("stdout = %q", stdout)
	}
	if !strings.Contains(stderr, "SCRIPT ERROR: boom") || strings.Contains(stderr, "out") {
		t.Errorf("stderr = %q", stderr)
	}
}
