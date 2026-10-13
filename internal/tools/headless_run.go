package tools

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/vilppuvuorinen/slopfood-godot-mcp/internal/mcp"
)

// Timeouts bound one-shot headless runs. A --script entrypoint (extends
// SceneTree/MainLoop) or a game loop can both run forever, and neither can be
// preempted, so on expiry the child engine is killed rather than left to consume
// the daemon-wide queue deadline.
const (
	runScriptFileTimeout = 60 * time.Second
	runProjectTimeout    = 60 * time.Second
)

const (
	exitCodeDescription = "Process exit code; -1 when the run was stopped at the timeout"
	timedOutDescription = "True when the run did not finish before the timeout and the engine was stopped"
)

// headlessOutcome is the raw result shared by the one-shot headless runners.
type headlessOutcome struct {
	stdout   string
	stderr   string
	exitCode int
	timedOut bool
}

// finishHeadlessRun classifies a one-shot run into an outcome, or returns a tool
// failure for a spawn error. A non-zero exit is a domain result, not a failure,
// so only a timeout or a failure to start is treated specially.
func finishHeadlessRun(ctx context.Context, runErr error, stdout, stderr string, exitCode int, label string) (headlessOutcome, *mcp.ToolResult) {
	if runErr != nil && errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return headlessOutcome{stdout: stdout, stderr: stderr, exitCode: -1, timedOut: true}, nil
	}
	if runErr != nil {
		failure := mcp.ErrorResult(label+": "+runErr.Error(), headlessSolutions...)
		return headlessOutcome{}, &failure
	}
	return headlessOutcome{stdout: stdout, stderr: stderr, exitCode: exitCode}, nil
}

// headlessRunText renders the human-readable outcome, keeping the raw streams
// visible alongside the structured result.
func headlessRunText(subject string, outcome headlessOutcome) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s: exit code %d", subject, outcome.exitCode)
	if outcome.timedOut {
		b.WriteString(" (stopped at the timeout)")
	}
	b.WriteString(".")
	if stdout := strings.TrimRight(outcome.stdout, "\n"); stdout != "" {
		b.WriteString("\n\nstdout:\n")
		b.WriteString(stdout)
	}
	if stderr := strings.TrimRight(outcome.stderr, "\n"); stderr != "" {
		b.WriteString("\n\nstderr:\n")
		b.WriteString(stderr)
	}
	return b.String()
}

// userArgs prefixes the Godot `--` separator so the engine forwards the
// arguments to the script or game (OS.get_cmdline_user_args) instead of parsing
// them itself.
func userArgs(args []string) []string {
	if len(args) == 0 {
		return nil
	}
	out := make([]string, 0, len(args)+1)
	out = append(out, "--")
	return append(out, args...)
}
