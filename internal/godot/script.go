package godot

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
)

// CheckScript runs the GDScript checker on scriptResPath and returns the
// combined stdout and stderr. The checker reports diagnostics on stderr with a
// non-zero exit, so a non-zero exit is not returned as an error.
func CheckScript(ctx context.Context, executable, projectPath, checkerPath, scriptResPath string) (string, error) {
	args := []string{
		"--headless",
		"--path", projectPath,
		"--script", checkerPath,
		scriptResPath,
	}
	cmd := exec.CommandContext(ctx, executable, args...) //nolint:gosec // executable is operator-controlled via GODOT_PATH.
	var stdoutBuf, stderrBuf bytes.Buffer
	cmd.Stdout = &stdoutBuf
	cmd.Stderr = &stderrBuf

	runErr := cmd.Run()
	stdout, stderr := stdoutBuf.String(), stderrBuf.String()

	var exitErr *exec.ExitError
	if errors.As(runErr, &exitErr) {
		return stdout + stderr, nil
	}
	if runErr != nil {
		return stdout + stderr, fmt.Errorf("run godot: %w", runErr)
	}
	return stdout + stderr, nil
}
