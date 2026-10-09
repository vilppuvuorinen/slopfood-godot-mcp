package godot

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
)

func newCommand(ctx context.Context, executable string, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, executable, args...) //nolint:gosec // executable is operator-controlled via GODOT_PATH.
	applyEngineEnv(cmd)
	return cmd
}

// runEngine runs one engine process and returns its separated streams and exit
// code. A non-zero exit is not an error: the scripts report their own failures
// that way, so only a failure to start the process is an error.
func runEngine(ctx context.Context, executable string, args ...string) (stdout, stderr string, exitCode int, err error) {
	cmd := newCommand(ctx, executable, args...)
	var stdoutBuf, stderrBuf bytes.Buffer
	cmd.Stdout = &stdoutBuf
	cmd.Stderr = &stderrBuf

	runErr := cmd.Run()
	stdout, stderr = stdoutBuf.String(), stderrBuf.String()

	var exitErr *exec.ExitError
	if errors.As(runErr, &exitErr) {
		return stdout, stderr, exitErr.ExitCode(), nil
	}
	if runErr != nil {
		return stdout, stderr, 0, fmt.Errorf("run godot: %w", runErr)
	}
	return stdout, stderr, 0, nil
}
