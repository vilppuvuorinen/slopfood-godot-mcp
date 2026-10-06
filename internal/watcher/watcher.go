// Package watcher reports relevant changes under a Godot project so the daemon
// can queue a rescan without waiting for the next tool call.
package watcher

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const (
	defaultDebounce     = 500 * time.Millisecond
	defaultPollInterval = 2 * time.Second
)

// Config configures a Watcher.
type Config struct {
	// Root is the bound project directory.
	Root string
	// Rescan runs once per settled burst of changes and blocks until the
	// rescan finishes; the daemon uses it to submit onto the shared queue.
	Rescan func(ctx context.Context)
	// Debounce coalesces a burst into one rescan; zero uses the default.
	Debounce time.Duration
	// Interval is the poll period on platforms without inotify; zero uses the default.
	Interval time.Duration
}

// Watcher coalesces relevant project changes and drives rescans.
type Watcher struct {
	rescan   func(ctx context.Context)
	debounce time.Duration
	events   chan struct{}
	notifier notifier

	ctx       context.Context
	cancel    context.CancelFunc
	wg        sync.WaitGroup
	closeOnce sync.Once
}

// The platform seam: Linux uses inotify, other platforms poll the filesystem.
type notifier interface {
	Close()
}

type notifierConfig struct {
	root     string
	interval time.Duration
	events   chan<- struct{}
}

func New(cfg Config) (*Watcher, error) {
	if strings.TrimSpace(cfg.Root) == "" {
		return nil, errors.New("watcher: empty root")
	}
	if cfg.Rescan == nil {
		return nil, errors.New("watcher: nil rescan")
	}
	debounce := cfg.Debounce
	if debounce <= 0 {
		debounce = defaultDebounce
	}
	events := make(chan struct{}, 1)
	n, err := newNotifier(notifierConfig{root: cfg.Root, interval: cfg.Interval, events: events})
	if err != nil {
		return nil, err
	}
	cfg.Debounce = debounce
	return newWatcher(cfg, events, n), nil
}

func newWatcher(cfg Config, events chan struct{}, n notifier) *Watcher {
	ctx, cancel := context.WithCancel(context.Background())
	w := &Watcher{
		rescan:   cfg.Rescan,
		debounce: cfg.Debounce,
		events:   events,
		notifier: n,
		ctx:      ctx,
		cancel:   cancel,
	}
	w.wg.Add(1)
	go w.loop()
	return w
}

// Close stops the watcher and blocks until its rescan returns.
func (w *Watcher) Close() {
	w.closeOnce.Do(func() {
		w.cancel()
		w.notifier.Close()
		w.wg.Wait()
	})
}

func (w *Watcher) loop() {
	defer w.wg.Done()
	for {
		select {
		case <-w.ctx.Done():
			return
		case <-w.events:
		}
		if !w.settle() {
			return
		}
		w.rescan(w.ctx)
	}
}

// settle waits out a burst of changes, returning false when the watcher stops.
func (w *Watcher) settle() bool {
	timer := time.NewTimer(w.debounce)
	defer timer.Stop()
	for {
		select {
		case <-w.ctx.Done():
			return false
		case <-w.events:
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			timer.Reset(w.debounce)
		case <-timer.C:
			return true
		}
	}
}

// Directories whose contents never need a rescan: the editor's own cache and
// version-control metadata. Rescans rewrite the cache, so watching it would loop.
var ignoredDirs = map[string]struct{}{
	".godot": {},
	".git":   {},
	".svn":   {},
	".hg":    {},
}

// Editor and OS scratch files that are not project sources.
var ignoredExts = map[string]struct{}{
	".tmp":  {},
	".swp":  {},
	".swx":  {},
	".orig": {},
	".rej":  {},
	".bak":  {},
}

func isIgnoredDir(name string) bool {
	_, ok := ignoredDirs[name]
	return ok
}

func isIgnoredName(name string) bool {
	if name == "" {
		return true
	}
	switch name {
	case ".DS_Store", "Thumbs.db", "desktop.ini":
		return true
	}
	if strings.HasPrefix(name, ".#") || strings.HasSuffix(name, "~") {
		return true
	}
	if strings.HasPrefix(name, "#") && strings.HasSuffix(name, "#") {
		return true
	}
	_, ok := ignoredExts[strings.ToLower(filepath.Ext(name))]
	return ok
}

// relevant reports whether a project-relative path should trigger a rescan.
func relevant(rel string) bool {
	if rel == "" {
		return false
	}
	rel = filepath.ToSlash(filepath.Clean(rel))
	if rel == "." || strings.HasPrefix(rel, "../") {
		return false
	}
	for _, part := range strings.Split(rel, "/") {
		if isIgnoredDir(part) {
			return false
		}
	}
	return !isIgnoredName(filepath.Base(rel))
}

func relevantPath(root, path string) bool {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return false
	}
	return relevant(rel)
}
