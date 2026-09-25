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
// wrapper failure: Godot's editor calls quit(EXIT_FAILURE) when the export
// fails, so the exit code is returned for the caller to interpret. Only a
// failure to start the process is returned as an error.
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
	cmd := exec.CommandContext(ctx, executable, args...) //nolint:gosec // executable is operator-controlled via GODOT_PATH.
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

// Import runs `<executable> --headless --path <project> --import`, which starts
// the editor, waits for the initial filesystem scan, and quits. The scan is what
// generates `.uid` sidecars for un-UID'd scripts and shaders: a runtime
// ResourceSaver.save does not write them, so this is the only headless path that
// materializes UIDs.
//
// A non-zero exit is not a wrapper failure: Godot's editor calls
// quit(EXIT_FAILURE) on scan/import errors, so the exit code is returned for the
// caller to interpret. Only a failure to start the process is returned as an
// error.
func Import(ctx context.Context, executable, projectPath string) (stdout, stderr string, exitCode int, err error) {
	args := []string{
		"--headless",
		"--path", projectPath,
		"--import",
	}
	return runScript(ctx, executable, args)
}

// RunOperation invokes
// `<executable> --headless --path <project> --script <script> <operation>
// <params-json>`, mirroring the reference server's executeOperation.
//
// A non-zero exit is not a wrapper failure: the operations script calls quit(1)
// on every error and quit() on success, so the exit code is returned for the
// caller to interpret. Only a failure to start the process is returned as an
// error, matching the reference's rethrow behavior.
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
	cmd := exec.CommandContext(ctx, executable, args...) //nolint:gosec // executable is operator-controlled via GODOT_PATH.
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
