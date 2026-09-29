//go:build !windows

package e2e

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
)

// daemonPIDs returns the PIDs of the detached daemons bound to project. A
// candidate must carry the server binary's name, the daemon subcommand, and the
// project path on its command line; the thin clients share the binary name and
// project but not the daemon subcommand, so they are not counted.
func daemonPIDs(t *testing.T, project string) []int {
	t.Helper()

	output, err := exec.CommandContext(context.Background(), "ps", "-eo", "pid=,args=").Output() //nolint:gosec // ps is a fixed host tool, not user input.
	if err != nil {
		t.Fatalf("ps: %v", err)
	}
	binary := filepath.Base(os.Getenv(binaryEnv))
	pids := make([]int, 0, 4)
	for _, line := range strings.Split(string(output), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 3 {
			continue
		}
		pid, perr := strconv.Atoi(fields[0])
		if perr != nil {
			continue
		}
		command := fields[1:]
		// project is its own argv token after --project. Match it exactly, not
		// as a substring: under a parallel run one project's temp path could
		// otherwise match another's daemon.
		if !slices.Contains(command, "daemon") || !slices.ContainsFunc(command, func(arg string) bool {
			return strings.Contains(arg, binary)
		}) || !slices.Contains(command, project) {
			continue
		}
		pids = append(pids, pid)
	}
	return pids
}
