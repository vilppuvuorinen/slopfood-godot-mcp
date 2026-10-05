package godot

import (
	"context"
	"strings"
	"testing"
)

func TestRunEngineCapturesStreamsSeparately(t *testing.T) {
	bin := writeExecutable(t, "echo out\necho 'ERROR: boom' >&2\nexit 0")

	stdout, stderr, exitCode, err := runEngine(context.Background(), bin)
	if err != nil {
		t.Fatalf("runEngine: %v", err)
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

func TestCheckScriptsCapturesExitCodeAndStreams(t *testing.T) {
	bin := writeExecutable(t, "echo out\necho 'SCRIPT ERROR: boom' >&2\nexit 1")

	stdout, stderr, exitCode, err := CheckScripts(context.Background(), bin, "/proj", "/checker.gd", []string{"res://player.gd"})
	if err != nil {
		t.Fatalf("CheckScripts: %v, want nil", err)
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

func TestCheckScriptsForwardsEveryPath(t *testing.T) {
	bin := writeExecutable(t, `printf '%s\n' "$@"`)

	stdout, _, _, err := CheckScripts(context.Background(), bin, "/proj", "/checker.gd",
		[]string{"res://a.gd", "res://nested/b.gd"})
	if err != nil {
		t.Fatalf("CheckScripts: %v", err)
	}
	for _, want := range []string{"--script", "/checker.gd", "res://a.gd", "res://nested/b.gd"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("stdout = %q, missing %q", stdout, want)
		}
	}
}
