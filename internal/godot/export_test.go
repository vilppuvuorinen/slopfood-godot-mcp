package godot

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
)

func TestExportArgsAndSuccess(t *testing.T) {
	bin := writeExecutable(t, `echo "$@"`)

	tests := []struct {
		name   string
		debug  bool
		export string
	}{
		{name: "release", debug: false, export: "--export-release"},
		{name: "debug", debug: true, export: "--export-debug"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stdout, stderr, exitCode, err := Export(context.Background(), bin, "/proj", "Linux", "build/game", tt.debug)
			if err != nil {
				t.Fatalf("Export: %v", err)
			}
			if exitCode != 0 {
				t.Errorf("exitCode = %d, want 0", exitCode)
			}
			if stderr != "" {
				t.Errorf("stderr = %q, want empty", stderr)
			}
			for _, want := range []string{"--headless", "--path /proj", tt.export, "Linux", "build/game"} {
				if !strings.Contains(stdout, want) {
					t.Errorf("stdout = %q, want it to contain %q", stdout, want)
				}
			}
		})
	}
}

func TestExportCapturesOutputOnNonZeroExit(t *testing.T) {
	bin := writeExecutable(t, "echo out\necho 'ERROR: no template' >&2\nexit 1")

	stdout, stderr, exitCode, err := Export(context.Background(), bin, "/proj", "Linux", "build/game", false)
	if err != nil {
		t.Fatalf("Export: %v, want nil (the exit code reports the failure)", err)
	}
	if exitCode != 1 {
		t.Errorf("exitCode = %d, want 1", exitCode)
	}
	if !strings.Contains(stdout, "out") {
		t.Errorf("stdout = %q", stdout)
	}
	if !strings.Contains(stderr, "ERROR: no template") {
		t.Errorf("stderr = %q", stderr)
	}
}

func TestExportSpawnFailure(t *testing.T) {
	_, _, _, err := Export(context.Background(), filepath.Join(t.TempDir(), "nope"), "/proj", "Linux", "build/game", false)
	if err == nil {
		t.Fatal("expected an error when the executable cannot be started")
	}
}
