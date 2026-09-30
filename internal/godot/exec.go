package godot

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
)

type OperationParams map[string]any

// A non-zero exit is not an error: Godot's editor calls quit(EXIT_FAILURE) when the export fails.
func Export(ctx context.Context, executable, projectPath, presetName, outputPath string, debug bool) (stdout, stderr string, exitCode int, err error) {
	exportFlag := "--export-release"
	if debug {
		exportFlag = "--export-debug"
	}
	args := []string{
		"--headless",
		"--path", projectPath,
		exportFlag,
		presetName,
		outputPath,
	}
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

// A non-zero exit is not an error: the script reports its own status.
func RunOperation(ctx context.Context, executable, projectPath, scriptPath, operation string, params OperationParams) (stdout, stderr string, exitCode int, err error) {
	payload, err := json.Marshal(params)
	if err != nil {
		return "", "", 0, fmt.Errorf("encode operation params: %w", err)
	}

	args := []string{
		"--headless",
		"--path", projectPath,
		"--script", scriptPath,
		operation,
		string(payload),
	}
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
