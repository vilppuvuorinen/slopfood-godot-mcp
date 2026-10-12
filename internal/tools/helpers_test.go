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

func (s *oneShotSession) Run(ctx context.Context, project, operation string, params map[string]any) (godot.RunResult, error) {
	scriptPath, err := godotops.OperationsScript()
	if err != nil {
		return godot.RunResult{}, fmt.Errorf("materialize operations script: %w", err)
	}
	payload, err := json.Marshal(params)
	if err != nil {
		return godot.RunResult{}, fmt.Errorf("encode operation params: %w", err)
	}

	cmd := exec.CommandContext(ctx, godot.Path(), //nolint:gosec // engine path is test-controlled via GODOT_PATH.
		"--headless", "--path", project, "--script", scriptPath, operation, string(payload))
	var stdoutBuf, stderrBuf bytes.Buffer
	cmd.Stdout = &stdoutBuf
	cmd.Stderr = &stderrBuf

	runErr := cmd.Run()
	stdout, stderr := stdoutBuf.String(), stderrBuf.String()

	var exitErr *exec.ExitError
	if errors.As(runErr, &exitErr) {
		return godot.RunResult{Stdout: stdout, Stderr: stderr, ExitCode: exitErr.ExitCode()}, nil
	}
	if runErr != nil {
		return godot.RunResult{Stdout: stdout, Stderr: stderr}, fmt.Errorf("run godot operation: %w", runErr)
	}
	return godot.RunResult{Stdout: stdout, Stderr: stderr}, nil
}

func (s *oneShotSession) RunIsolated(ctx context.Context, project, operation string, params map[string]any) (godot.RunResult, error) {
	return s.Run(ctx, project, operation, params)
}

func (s *oneShotSession) RunHeadless(ctx context.Context, project, scriptPath string, extraArgs ...string) (stdout, stderr string, exitCode int, err error) {
	return godot.RunScript(ctx, godot.Path(), project, scriptPath, extraArgs...) //nolint:wrapcheck // godot errors are already caller-facing.
}

func (s *oneShotSession) Export(ctx context.Context, project, presetName, outputPath string, debug bool) (stdout, stderr string, exitCode int, err error) {
	return godot.Export(ctx, godot.Path(), project, presetName, outputPath, debug) //nolint:wrapcheck // godot errors are already caller-facing.
}

func (s *oneShotSession) Version(ctx context.Context) (string, error) {
	return godot.Version(ctx, godot.Path()) //nolint:wrapcheck // godot errors are already caller-facing.
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

// payloadSession is an Engine fake that returns a fixed bridge payload so tests
// can exercise the structured-result path without a real engine.
type payloadSession struct {
	fakeSession
	result godot.RunResult
	err    error
}

func (s *payloadSession) Run(context.Context, string, string, map[string]any) (godot.RunResult, error) {
	return s.result, s.err
}

func callToolPayload(t *testing.T, build func(*toolset) mcp.Tool, project, args, payload string) mcp.ToolResult {
	t.Helper()
	tool := build(newToolset(project, &payloadSession{result: godot.RunResult{Payload: payload}}))
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
