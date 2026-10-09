package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/vilppuvuorinen/slopfood-godot-mcp/internal/godot"
	"github.com/vilppuvuorinen/slopfood-godot-mcp/internal/godotops"
	"github.com/vilppuvuorinen/slopfood-godot-mcp/internal/mcp"
)

func makeProject(t *testing.T, dir string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", dir, err)
	}
	if err := os.WriteFile(filepath.Join(dir, "project.godot"), []byte("config_version=5\n"), 0o644); err != nil { //nolint:gosec // test fixture.
		t.Fatalf("write project.godot: %v", err)
	}
}

func seedFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil { //nolint:gosec // test fixture.
		t.Fatalf("write %s: %v", path, err)
	}
}

type oneShotSession struct{}

func (s *oneShotSession) Run(ctx context.Context, project, operation string, params map[string]any) (stdout, stderr string, exitCode int, err error) {
	scriptPath, err := godotops.OperationsScript()
	if err != nil {
		return "", "", 0, fmt.Errorf("materialize operations script: %w", err)
	}
	payload, err := json.Marshal(params)
	if err != nil {
		return "", "", 0, fmt.Errorf("encode operation params: %w", err)
	}

	cmd := exec.CommandContext(ctx, godot.Path(), //nolint:gosec // engine path is test-controlled via GODOT_PATH.
		"--headless", "--path", project, "--script", scriptPath, operation, string(payload))
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
		return stdout, stderr, 0, fmt.Errorf("run godot operation: %w", runErr)
	}
	return stdout, stderr, 0, nil
}

func callToolArgs(t *testing.T, build func(*toolset) mcp.Tool, project, args string) mcp.ToolResult {
	t.Helper()
	tool := build(newToolset(project, &oneShotSession{}))
	out, err := tool.Handler(context.Background(), json.RawMessage(args))
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	result, ok := out.(mcp.ToolResult)
	if !ok {
		t.Fatalf("result type %T, want mcp.ToolResult", out)
	}
	return result
}

func jsonString(s string) string {
	data, err := json.Marshal(s)
	if err != nil {
		panic(err)
	}
	return string(data)
}
