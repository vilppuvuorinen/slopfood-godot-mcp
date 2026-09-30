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

// daemonPIDs matches the server binary, the daemon subcommand, and the project path; thin clients lack the daemon subcommand.
func daemonPIDs(t *testing.T, project string) []int {
	t.Helper()

	output, err := exec.CommandContext(context.Background(), "powershell", "-NoProfile", "-Command",
		"Get-CimInstance Win32_Process | Select-Object ProcessId,CommandLine | ConvertTo-Json -Compress").Output()
	if err != nil {
		t.Fatalf("cim query: %v", err)
	}
	binary := filepath.Base(os.Getenv(binaryEnv))
	pids := make([]int, 0, 4)
	for _, proc := range decodeCIMProcesses(t, output) {
		command := proc.CommandLine
		// Match the project as a token boundary, not a bare substring: parallel runs could cross-match.
		if !strings.Contains(command, binary) ||
			!strings.Contains(command, " daemon ") ||
			!strings.Contains(command, " "+project+" ") {
			continue
		}
		pids = append(pids, proc.ProcessID)
	}
	return pids
}

type cimProcess struct {
	ProcessID   int
	CommandLine string
}

// ConvertTo-Json yields a single object for one match and an array otherwise.
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

func init() {
	daemonPollInterval = time.Second
}
