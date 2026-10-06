// Package watcher reports relevant changes under a Godot project so the daemon
// can queue a rescan without waiting for the next tool call.
package watcher

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"sort"
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
	// paths holds the project-relative paths seen during the burst, sorted;
	// an empty slice means only the fact of a change was known.
	Rescan func(ctx context.Context, paths []string)
	// Debounce coalesces a burst into one rescan; zero uses the default.
	Debounce time.Duration
	// Interval is the poll period on platforms without inotify; zero uses the default.
	Interval time.Duration
}

// Watcher coalesces relevant project changes and drives rescans.
type Watcher struct {
	rescan   func(ctx context.Context, paths []string)
	debounce time.Duration
	events   chan struct{}
	notifier notifier
	changes  *changeSet
	// flushReqs carries flush requests to the loop, which waits out the current
	// debounce and scan so a flush never overlaps one.
	flushReqs chan chan struct{}

	ctx       context.Context
	cancel    context.CancelFunc
	wg        sync.WaitGroup
	closeOnce sync.Once
}

// changeSet accumulates the project-relative paths seen within a burst and
// tracks whether anything is pending at all. The notifier records paths as they
// arrive while a separate event signal wakes the loop, so paths cannot be lost
// when the buffered signal is full. A directory change carries no path, so the
// pending flag is what lets a flush run a scan without a concrete path.
type changeSet struct {
	mu    sync.Mutex
	paths map[string]struct{}
	dirty bool
}

func newChangeSet() *changeSet {
	return &changeSet{paths: make(map[string]struct{})}
}

func (c *changeSet) add(path string) {
	c.mu.Lock()
	c.dirty = true
	if path != "" {
		c.paths[path] = struct{}{}
	}
	c.mu.Unlock()
}

// mark records that a change happened without naming a path.
func (c *changeSet) mark() {
	c.mu.Lock()
	c.dirty = true
	c.mu.Unlock()
}

// isDirty reports whether anything is pending without clearing it.
func (c *changeSet) isDirty() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.dirty
}

// take reports whether anything is pending and returns the accumulated paths
// sorted, clearing both the paths and the pending flag.
func (c *changeSet) take() ([]string, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.dirty {
		return nil, false
	}
	c.dirty = false
	if len(c.paths) == 0 {
		return nil, true
	}
	out := make([]string, 0, len(c.paths))
	for path := range c.paths {
		out = append(out, path)
	}
	c.paths = make(map[string]struct{})
	sort.Strings(out)
	return out, true
}

// The platform seam: Linux uses inotify, other platforms poll the filesystem.
type notifier interface {
	Close()
}

type notifierConfig struct {
	root     string
	interval time.Duration
	events   chan<- struct{}
	// record captures a changed project-relative path; it may be nil.
	record func(path string)
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
	changes := newChangeSet()
	events := make(chan struct{}, 1)
	n, err := newNotifier(notifierConfig{root: cfg.Root, interval: cfg.Interval, events: events, record: changes.add})
	if err != nil {
		return nil, err
	}
	cfg.Debounce = debounce
	return newWatcher(cfg, events, n, changes), nil
}

func newWatcher(cfg Config, events chan struct{}, n notifier, changes *changeSet) *Watcher {
	ctx, cancel := context.WithCancel(context.Background())
	w := &Watcher{
		rescan:    cfg.Rescan,
		debounce:  cfg.Debounce,
		events:    events,
		notifier:  n,
		changes:   changes,
		flushReqs: make(chan chan struct{}),
		ctx:       ctx,
		cancel:    cancel,
	}
	w.wg.Add(1)
	go w.loop()
	return w
}

// Flush blocks until any pending burst has been debounced and scanned, so the
// project is settled when it returns. It is a no-op on an idle watcher. The
// scan is submitted through the rescan callback, which the daemon routes onto
// the shared queue, so Flush must be called off the queue worker goroutine to
// avoid a self-deadlock.
func (w *Watcher) Flush(ctx context.Context) error {
	done := make(chan struct{})
	select {
	case w.flushReqs <- done:
	case <-w.ctx.Done():
		return nil
	case <-ctx.Done():
		return fmt.Errorf("watcher flush: %w", ctx.Err())
	}
	select {
	case <-done:
		return nil
	case <-w.ctx.Done():
		return nil
	case <-ctx.Done():
		return fmt.Errorf("watcher flush: %w", ctx.Err())
	}
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
			w.changes.mark()
		case done := <-w.flushReqs:
			// Let the burst debounce and scan naturally, so a flush waits out
			// the same settle the watcher would do on its own. Draining a
			// buffered signal first keeps a request from slipping ahead of it.
			select {
			case <-w.events:
				w.changes.mark()
			default:
			}
			if w.changes.isDirty() {
				if !w.settle() {
					close(done)
					return
				}
				w.scan()
			}
			close(done)
			continue
		}
		if !w.settle() {
			return
		}
		w.scan()
	}
}

// scan runs a rescan for whatever is pending, or does nothing when the changes
// were already consumed by a barrier.
func (w *Watcher) scan() {
	paths, dirty := w.changes.take()
	if !dirty {
		return
	}
	w.rescan(w.ctx, paths)
}

// settle waits out a burst of changes, resetting the timer as new events
// arrive. It reports false when the watcher stops.
func (w *Watcher) settle() bool {
	timer := time.NewTimer(w.debounce)
	defer timer.Stop()
	for {
		select {
		case <-w.ctx.Done():
			return false
		case <-w.events:
			w.changes.mark()
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
