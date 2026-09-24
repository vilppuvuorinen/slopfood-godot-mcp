package godot

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
)

func TestImportArgsAndSuccess(t *testing.T) {
	bin := writeExecutable(t, `echo "$@"`)

	stdout, stderr, exitCode, err := Import(context.Background(), bin, "/proj")
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	if exitCode != 0 {
		t.Errorf("exitCode = %d, want 0", exitCode)
	}
	if stderr != "" {
		t.Errorf("stderr = %q, want empty", stderr)
	}
	for _, want := range []string{"--headless", "--path /proj", "--import"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("stdout = %q, want it to contain %q", stdout, want)
		}
	}
	if strings.Contains(stdout, "--script") {
		t.Errorf("stdout = %q, want no --script", stdout)
	}
}

func TestImportCapturesOutputOnNonZeroExit(t *testing.T) {
	bin := writeExecutable(t, "echo out\necho 'ERROR: scan failed' >&2\nexit 1")

	stdout, stderr, exitCode, err := Import(context.Background(), bin, "/proj")
	if err != nil {
		t.Fatalf("Import: %v, want nil (the exit code reports the failure)", err)
	}
	if exitCode != 1 {
		t.Errorf("exitCode = %d, want 1", exitCode)
	}
	if !strings.Contains(stdout, "out") {
		t.Errorf("stdout = %q", stdout)
	}
	if !strings.Contains(stderr, "ERROR: scan failed") {
		t.Errorf("stderr = %q", stderr)
	}
}

func TestImportSpawnFailure(t *testing.T) {
	_, _, _, err := Import(context.Background(), filepath.Join(t.TempDir(), "nope"), "/proj")
	if err == nil {
		t.Fatal("expected an error when the executable cannot be started")
	}
}
