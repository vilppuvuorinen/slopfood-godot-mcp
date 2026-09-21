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
