package godot

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
)

// CheckScript runs the GDScript checker on scriptResPath and returns the
// checker's stdout, stderr, and exit code. Godot logs diagnostics to stderr and
// can still exit 0 when the target script fails to compile, so a non-zero exit
// is not returned as an error.
func CheckScript(ctx context.Context, executable, projectPath, checkerPath, scriptResPath string) (stdout, stderr string, exitCode int, err error) {
	args := []string{
		"--headless",
		"--path", projectPath,
		"--script", checkerPath,
		scriptResPath,
	}
	return runScript(ctx, executable, args)
}

// ValidateProject boots the engine against a project with a no-op probe script
// and returns the engine's stdout, stderr, and exit code separately. Godot
// parses project.godot during startup and logs a parse error to stderr while
// still exiting 0 when the file is malformed, so the exit code and stderr are
// both returned for the caller to combine. The probe script only lets startup
// run and quits; the operations bridge cannot be used here because it now only
// speaks its socket transport. Only a failure to start the process is an error.
func ValidateProject(ctx context.Context, executable, projectPath, scriptPath string) (stdout, stderr string, exitCode int, err error) {
	args := []string{
		"--headless",
		"--path", projectPath,
		"--script", scriptPath,
	}
	return runScript(ctx, executable, args)
}

// runScript runs the executable with args and returns stdout, stderr, and the
// exit code separately. A non-zero exit is not an error: Godot can report a
// failure on stderr while still exiting 0, and vice versa, so callers combine
// both signals.
func runScript(ctx context.Context, executable string, args []string) (stdout, stderr string, exitCode int, err error) {
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
