//go:build linux

package watcher

import (
	"path/filepath"
	"testing"
	"time"
)

func TestInotifyNotifierSignalsOnRelevantChange(t *testing.T) {
	root := t.TempDir()
	writeProjectFile(t, filepath.Join(root, "project.godot"), "config_version=5\n")

	changes := make(chan struct{}, 1)
	n, err := newNotifier(notifierConfig{root: root, events: changes})
	if err != nil {
		t.Fatalf("newNotifier: %v", err)
	}
	t.Cleanup(n.Close)

	writeProjectFile(t, filepath.Join(root, "scripts", "player.gd"), "extends Node\n")
	if !waitForChange(changes, 2*time.Second) {
		t.Fatal("new script did not signal a change")
	}
}

func TestInotifyNotifierIgnoresCache(t *testing.T) {
	root := t.TempDir()
	writeProjectFile(t, filepath.Join(root, "project.godot"), "config_version=5\n")

	changes := make(chan struct{}, 1)
	n, err := newNotifier(notifierConfig{root: root, events: changes})
	if err != nil {
		t.Fatalf("newNotifier: %v", err)
	}
	t.Cleanup(n.Close)

	writeProjectFile(t, filepath.Join(root, ".godot", "uid_cache.bin"), "cache")
	if waitForChange(changes, 200*time.Millisecond) {
		t.Fatal("a cache change signalled a rescan")
	}
}
