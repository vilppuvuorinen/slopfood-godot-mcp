//go:build !windows

package e2e

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// The socket is 0600 and its directory 0700, the filesystem gate alongside the daemon's peer UID check.
func TestDaemonSocketPermissions(t *testing.T) {
	t.Parallel()
	project := newProject(t)
	runtimeDir := newRuntimeDir(t)

	session, err := connectInRuntime(t, runtimeDir, daemonProjectArgs(project, "--idle-timeout", "1s")...)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	if _, listErr := session.ListTools(context.Background(), nil); listErr != nil {
		t.Fatalf("tools/list: %v", listErr)
	}

	socket := socketPath(t, runtimeDir)
	info, statErr := os.Stat(socket)
	if statErr != nil {
		t.Fatalf("stat socket: %v", statErr)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Errorf("socket mode = %o, want 600", got)
	}
	dirInfo, dirErr := os.Stat(filepath.Dir(socket))
	if dirErr != nil {
		t.Fatalf("stat socket dir: %v", dirErr)
	}
	if got := dirInfo.Mode().Perm(); got != 0o700 {
		t.Errorf("socket dir mode = %o, want 700", got)
	}
}
