package godot

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
)

// OperationParams is the JSON object handed to the operations script.
type OperationParams map[string]any

// Export runs
// `<executable> --headless --path <project> --export-release|--export-debug
// <preset> <output>` and returns the captured streams. A non-zero exit is not a
// wrapper failure, matching RunOperation: the caller inspects stderr for
// ERROR markers.
func Export(ctx context.Context, executable, projectPath, presetName, outputPath string, debug bool) (stdout, stderr string, err error) {
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
	cmd := exec.CommandContext(ctx, executable, args...) //nolint:gosec // executable is operator-controlled via GODOT_PATH.
	var stdoutBuf, stderrBuf bytes.Buffer
	cmd.Stdout = &stdoutBuf
	cmd.Stderr = &stderrBuf

	runErr := cmd.Run()
	stdout, stderr = stdoutBuf.String(), stderrBuf.String()

	var exitErr *exec.ExitError
	if errors.As(runErr, &exitErr) {
		return stdout, stderr, nil
	}
	if runErr != nil {
		return stdout, stderr, fmt.Errorf("run godot: %w", runErr)
	}
	return stdout, stderr, nil
}

// RunOperation invokes
// `<executable> --headless --path <project> --script <script> <operation>
// <params-json> --debug-godot`, mirroring the reference server's
// executeOperation.
//
// A non-zero exit is not a wrapper failure: the operations script reports its
// own errors on stderr with "Failed to" markers, which the caller inspects.
// Only a failure to start the process is returned as an error, matching the
// reference's rethrow behavior.
func RunOperation(ctx context.Context, executable, projectPath, scriptPath, operation string, params OperationParams) (stdout, stderr string, err error) {
	payload, err := json.Marshal(params)
	if err != nil {
		return "", "", fmt.Errorf("encode operation params: %w", err)
	}

	args := []string{
		"--headless",
		"--path", projectPath,
		"--script", scriptPath,
		operation,
		string(payload),
		"--debug-godot",
	}
	cmd := exec.CommandContext(ctx, executable, args...) //nolint:gosec // executable is operator-controlled via GODOT_PATH.
	var stdoutBuf, stderrBuf bytes.Buffer
	cmd.Stdout = &stdoutBuf
	cmd.Stderr = &stderrBuf

	runErr := cmd.Run()
	stdout, stderr = stdoutBuf.String(), stderrBuf.String()

	var exitErr *exec.ExitError
	if errors.As(runErr, &exitErr) {
		return stdout, stderr, nil
	}
	if runErr != nil {
		return stdout, stderr, fmt.Errorf("run godot: %w", runErr)
	}
	return stdout, stderr, nil
}
