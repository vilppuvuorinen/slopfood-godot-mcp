package e2e

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// The daemon watches the project and emits import sidecars for new sources
// without an explicit rescan_project call.
func TestAutoRescanEmitsSidecarForNewAsset(t *testing.T) {
	t.Parallel()
	root := newProject(t)
	connectProject(t, root)

	svg := filepath.Join(root, "icon.svg")
	writeE2EFile(t, svg, `<svg xmlns="http://www.w3.org/2000/svg" width="16" height="16"><rect width="16" height="16" fill="red"/></svg>`)

	sidecar := svg + ".import"
	waitForFile(t, sidecar, 20*time.Second)
	requireContains(t, readFile(t, sidecar), `importer="texture"`, `uid="uid://`)
}

// A hand-edited import sidecar is reconciled by the watcher without an explicit
// rescan_project call.
func TestAutoRescanAppliesSidecarEdit(t *testing.T) {
	t.Parallel()
	root := newProject(t)
	connectProject(t, root)

	svg := filepath.Join(root, "icon.svg")
	writeE2EFile(t, svg, `<svg xmlns="http://www.w3.org/2000/svg" width="16" height="16"><rect width="16" height="16" fill="red"/></svg>`)

	sidecar := svg + ".import"
	waitForFile(t, sidecar, 20*time.Second)

	edited := setImportParam(readFile(t, sidecar), "compress/mode", "1")
	writeE2EFile(t, sidecar, edited)

	waitForContains(t, sidecar, "compress/mode=1", 20*time.Second)
}

func waitForFile(t *testing.T, path string, within time.Duration) {
	t.Helper()
	if !pollUntil(within, func() bool {
		info, err := os.Stat(path)
		return err == nil && info.Size() > 0
	}) {
		t.Fatalf("file %s did not appear within %s", path, within)
	}
}

func waitForContains(t *testing.T, path, want string, within time.Duration) {
	t.Helper()
	if !pollUntil(within, func() bool {
		data, err := os.ReadFile(path)
		return err == nil && strings.Contains(string(data), want)
	}) {
		t.Fatalf("%s did not contain %q within %s", path, want, within)
	}
}

func pollUntil(within time.Duration, pred func() bool) bool {
	deadline := time.Now().Add(within)
	for time.Now().Before(deadline) {
		if pred() {
			return true
		}
		time.Sleep(50 * time.Millisecond)
	}
	return pred()
}
