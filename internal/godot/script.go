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
	return runScript(ctx, executable, args)
}

// ValidateProject runs the operations-script validate_project operation against
// a project and returns the combined stdout and stderr. Godot parses
// project.godot during startup and logs a parse error to stderr (while still
// exiting 0) when the file is malformed, so the combined output is returned for
// the caller to inspect. Only a failure to start the process is an error.
func ValidateProject(ctx context.Context, executable, projectPath, scriptPath string) (string, error) {
	args := []string{
		"--headless",
		"--path", projectPath,
		"--script", scriptPath,
		"validate_project",
		"{}",
	}
	return runScript(ctx, executable, args)
}

// runScript runs the executable with args and returns the combined stdout and
// stderr. A non-zero exit is not an error: the callers interpret Godot's exit
// codes themselves.
func runScript(ctx context.Context, executable string, args []string) (string, error) {
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
