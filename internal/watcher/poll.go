package watcher

import (
	"io/fs"
	"path/filepath"
	"sync"
	"time"
)

// fileStamp identifies a file's content revision cheaply; a file missing from
// the snapshot has no stamp at all.
type fileStamp struct {
	size int64
	mod  int64
}

// pollNotifier detects changes by periodically snapshotting the project tree.
// It is the fallback on platforms without inotify (Windows and macOS).
type pollNotifier struct {
	root     string
	interval time.Duration
	events   chan<- struct{}
	record   func(path string)
	stop     chan struct{}
	done     chan struct{}

	baseline  map[string]fileStamp
	closeOnce sync.Once
}

func newPollNotifier(cfg notifierConfig) *pollNotifier {
	interval := cfg.interval
	if interval <= 0 {
		interval = defaultPollInterval
	}
	p := &pollNotifier{
		root:     cfg.root,
		interval: interval,
		events:   cfg.events,
		record:   cfg.record,
		stop:     make(chan struct{}),
		done:     make(chan struct{}),
		baseline: snapshotProject(cfg.root),
	}
	go p.run()
	return p
}

func (p *pollNotifier) Close() {
	p.closeOnce.Do(func() {
		close(p.stop)
		<-p.done
	})
}

func (p *pollNotifier) run() {
	defer close(p.done)
	ticker := time.NewTicker(p.interval)
	defer ticker.Stop()
	for {
		select {
		case <-p.stop:
			return
		case <-ticker.C:
			current := snapshotProject(p.root)
			if snapshotsEqual(p.baseline, current) {
				continue
			}
			p.reportChanges(p.baseline, current)
			p.baseline = current
			select {
			case p.events <- struct{}{}:
			default:
			}
		}
	}
}

// reportChanges records the project-relative paths that appeared, changed, or
// disappeared between two snapshots so the burst can be reconciled by path.
func (p *pollNotifier) reportChanges(before, after map[string]fileStamp) {
	if p.record == nil {
		return
	}
	for path, stamp := range after {
		if prev, ok := before[path]; ok && prev == stamp {
			continue
		}
		p.recordPath(path)
	}
	for path := range before {
		if _, ok := after[path]; !ok {
			p.recordPath(path)
		}
	}
}

func (p *pollNotifier) recordPath(path string) {
	rel, err := filepath.Rel(p.root, path)
	if err != nil {
		return
	}
	p.record(filepath.ToSlash(rel))
}

// snapshotProject stamps every relevant file under root. Walk and stat failures
// are skipped: a racing delete must not abort the whole snapshot.
func snapshotProject(root string) map[string]fileStamp {
	stamps := make(map[string]fileStamp)
	_ = filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return nil //nolint:nilerr // a racing delete must not abort the whole snapshot.
		}
		if entry.IsDir() {
			if path != root && isIgnoredDir(entry.Name()) {
				return fs.SkipDir
			}
			return nil
		}
		if !relevantPath(root, path) {
			return nil
		}
		info, infoErr := entry.Info()
		if infoErr != nil {
			return nil //nolint:nilerr // a file removed mid-walk is simply absent from the snapshot.
		}
		stamps[path] = fileStamp{size: info.Size(), mod: info.ModTime().UnixNano()}
		return nil
	})
	return stamps
}

func snapshotsEqual(a, b map[string]fileStamp) bool {
	if len(a) != len(b) {
		return false
	}
	for path, stamp := range a {
		if b[path] != stamp {
			return false
		}
	}
	return true
}
