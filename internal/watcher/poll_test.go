package watcher

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestRelevant(t *testing.T) {
	tests := []struct {
		name string
		path string
		want bool
	}{
		{"project manifest", "project.godot", true},
		{"script", "scripts/player.gd", true},
		{"scene", "scenes/main.tscn", true},
		{"resource", "assets/icon.tres", true},
		{"import sidecar", "assets/icon.svg.import", true},
		{"empty", "", false},
		{"dot", ".", false},
		{"escape", "../outside.gd", false},
		{"editor cache", ".godot/uid_cache.bin", false},
		{"nested editor cache", "scenes/.godot/editor/layout.cfg", false},
		{"git metadata", ".git/index", false},
		{"svn metadata", ".svn/entries", false},
		{"os junk", "scripts/.DS_Store", false},
		{"editor backup", "scripts/player.gd~", false},
		{"editor swap", "scripts/player.gd.swp", false},
		{"emacs lock", "scripts/.#player.gd", false},
		{"emacs autosave", "scripts/#player.gd#", false},
		{"temp file", "scripts/player.gd.tmp", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := relevant(tt.path); got != tt.want {
				t.Errorf("relevant(%q) = %v, want %v", tt.path, got, tt.want)
			}
		})
	}
}

func TestRelevantPathOutsideRoot(t *testing.T) {
	root := t.TempDir()
	if relevantPath(root, filepath.Join(root, "..", "outside.gd")) {
		t.Error("a path outside the root is relevant")
	}
}

func writeProjectFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func newPollTestNotifier(t *testing.T, root string) <-chan struct{} {
	t.Helper()
	changes := make(chan struct{}, 1)
	p := newPollNotifier(notifierConfig{root: root, interval: 5 * time.Millisecond, events: changes})
	t.Cleanup(p.Close)
	return changes
}

func waitForChange(changes <-chan struct{}, within time.Duration) bool {
	select {
	case <-changes:
		return true
	case <-time.After(within):
		return false
	}
}

func TestPollNotifierSignalsOnNewRelevantFile(t *testing.T) {
	root := t.TempDir()
	writeProjectFile(t, filepath.Join(root, "project.godot"), "config_version=5\n")

	changes := newPollTestNotifier(t, root)

	writeProjectFile(t, filepath.Join(root, "scripts", "player.gd"), "extends Node\n")
	if !waitForChange(changes, time.Second) {
		t.Fatal("new script did not signal a change")
	}
}

func TestPollNotifierSignalsOnModifiedFile(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "player.gd")
	writeProjectFile(t, path, "extends Node\n")

	changes := newPollTestNotifier(t, root)

	writeProjectFile(t, path, "extends Node2D\n")
	if !waitForChange(changes, time.Second) {
		t.Fatal("modified script did not signal a change")
	}
}

func TestPollNotifierSignalsOnDeletedFile(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "player.gd")
	writeProjectFile(t, path, "extends Node\n")

	changes := newPollTestNotifier(t, root)

	if err := os.Remove(path); err != nil {
		t.Fatalf("remove %s: %v", path, err)
	}
	if !waitForChange(changes, time.Second) {
		t.Fatal("deleted script did not signal a change")
	}
}

func TestPollNotifierIgnoresCacheAndJunk(t *testing.T) {
	root := t.TempDir()
	writeProjectFile(t, filepath.Join(root, "project.godot"), "config_version=5\n")

	changes := newPollTestNotifier(t, root)

	writeProjectFile(t, filepath.Join(root, ".godot", "uid_cache.bin"), "cache")
	writeProjectFile(t, filepath.Join(root, ".git", "index"), "git")
	writeProjectFile(t, filepath.Join(root, "player.gd~"), "backup")
	writeProjectFile(t, filepath.Join(root, "player.gd.swp"), "swap")

	if waitForChange(changes, 100*time.Millisecond) {
		t.Fatal("a cache or scratch file signalled a change")
	}
}

func TestPollNotifierStaysQuietWithoutChanges(t *testing.T) {
	root := t.TempDir()
	writeProjectFile(t, filepath.Join(root, "project.godot"), "config_version=5\n")

	changes := newPollTestNotifier(t, root)
	if waitForChange(changes, 100*time.Millisecond) {
		t.Fatal("an unchanged project signalled a change")
	}
}

type fakeNotifier struct {
	closed bool
}

func (f *fakeNotifier) Close() { f.closed = true }

func TestWatcherCoalescesBurst(t *testing.T) {
	events := make(chan struct{}, 1)
	calls := make(chan struct{}, 4)
	cfg := Config{
		Root:     t.TempDir(),
		Debounce: 40 * time.Millisecond,
		Rescan:   func(context.Context) { calls <- struct{}{} },
	}
	notifier := &fakeNotifier{}
	w := newWatcher(cfg, events, notifier)
	defer w.Close()

	events <- struct{}{}
	time.Sleep(10 * time.Millisecond)
	events <- struct{}{}

	if !waitForChange(calls, time.Second) {
		t.Fatal("a burst did not produce a rescan")
	}
	if waitForChange(calls, 3*cfg.Debounce) {
		t.Fatal("a single burst produced more than one rescan")
	}

	events <- struct{}{}
	if !waitForChange(calls, time.Second) {
		t.Fatal("a later change did not produce a rescan")
	}

	w.Close()
	if !notifier.closed {
		t.Error("Close did not close the notifier")
	}
}

// A change is reported without a path; the rescan callback takes none.
func TestWatcherRescanTakesNoPaths(t *testing.T) {
	events := make(chan struct{}, 1)
	calls := make(chan struct{}, 4)
	cfg := Config{
		Root:     t.TempDir(),
		Debounce: 30 * time.Millisecond,
		Rescan:   func(context.Context) { calls <- struct{}{} },
	}
	w := newWatcher(cfg, events, &fakeNotifier{})
	defer w.Close()

	events <- struct{}{}
	if !waitForChange(calls, time.Second) {
		t.Fatal("a change did not produce a rescan")
	}
}
