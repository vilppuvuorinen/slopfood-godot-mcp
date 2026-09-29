//go:build windows

package e2e

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// daemonPIDs returns the PIDs of the detached daemons bound to project. A
// candidate must carry the server binary's name, the daemon subcommand, and the
// project path on its command line; the thin clients share the binary name and
// project but not the daemon subcommand, so they are not counted.
//
// Process enumeration is one CIM query for the whole call rather than one query
// per process: a CIM query is expensive, and waitForDaemon polls this in a loop.
// The one-query shape is why the interval is widened below.
func daemonPIDs(t *testing.T, project string) []int {
	t.Helper()

	// -NoProfile keeps the user's profile out of the query; the CommandLine is
	// the process's full argv, which carries the project path and subcommand.
	output, err := exec.CommandContext(context.Background(), "powershell", "-NoProfile", "-Command",
		"Get-CimInstance Win32_Process | Select-Object ProcessId,CommandLine | ConvertTo-Json -Compress").Output()
	if err != nil {
		t.Fatalf("cim query: %v", err)
	}
	binary := filepath.Base(os.Getenv(binaryEnv))
	pids := make([]int, 0, 4)
	for _, proc := range decodeCIMProcesses(t, output) {
		command := proc.CommandLine
		// project is its own argv token after --project. Match it as a
		// token boundary, not a bare substring: under a parallel run one
		// project's temp path must not match another's daemon.
		if !strings.Contains(command, binary) ||
			!strings.Contains(command, " daemon ") ||
			!strings.Contains(command, " "+project+" ") {
			continue
		}
		pids = append(pids, proc.ProcessID)
	}
	return pids
}

// cimProcess is the subset of a Win32_Process row the helper needs.
type cimProcess struct {
	ProcessID   int
	CommandLine string
}

// decodeCIMProcesses decodes ConvertTo-Json output, which is a single object
// when one process matches and an array otherwise.
func decodeCIMProcesses(t *testing.T, output []byte) []cimProcess {
	t.Helper()
	trimmed := strings.TrimSpace(string(output))
	if trimmed == "" {
		return nil
	}
	var procs []cimProcess
	if err := json.Unmarshal([]byte(trimmed), &procs); err == nil {
		return procs
	}
	var one cimProcess
	if err := json.Unmarshal([]byte(trimmed), &one); err != nil {
		t.Fatalf("decode cim output %q: %v", trimmed, err)
	}
	return []cimProcess{one}
}

// init widens the daemon poll interval on Windows, where each poll runs an
// expensive CIM query.
func init() {
	daemonPollInterval = time.Second
}
