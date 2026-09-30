package godot

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
)

// CheckScripts runs the checker over every target in one engine process. The
// checker brackets each target with a marker so the caller can attribute the
// diagnostics on stderr to the script being loaded.
func CheckScripts(ctx context.Context, executable, projectPath, checkerPath string, scriptResPaths []string) (stdout, stderr string, exitCode int, err error) {
	args := make([]string, 0, 5+len(scriptResPaths))
	args = append(args,
		"--headless",
		"--path", projectPath,
		"--script", checkerPath,
	)
	args = append(args, scriptResPaths...)
	return runScript(ctx, executable, args)
}

// Godot can parse project.godot during startup and report an error on stderr while still exiting 0, so
// the exit code and stderr are both returned and only a failure to start is an error.
func ValidateProject(ctx context.Context, executable, projectPath, scriptPath string) (stdout, stderr string, exitCode int, err error) {
	args := []string{
		"--headless",
		"--path", projectPath,
		"--script", scriptPath,
	}
	return runScript(ctx, executable, args)
}

// Godot can report a failure on stderr while exiting 0, and vice versa, so callers combine both signals.
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
