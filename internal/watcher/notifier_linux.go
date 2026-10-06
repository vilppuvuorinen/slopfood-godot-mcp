//go:build linux

package watcher

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
)

// watcherMask is the event set registered on every project directory. Close-write
// and moved-to cover saves and atomic replaces; create/moved-to also drive the
// recursive watch of new directories.
const watcherMask = syscall.IN_CREATE | syscall.IN_CLOSE_WRITE | syscall.IN_MOVED_TO |
	syscall.IN_DELETE | syscall.IN_MOVED_FROM | syscall.IN_ATTRIB |
	syscall.IN_DELETE_SELF | syscall.IN_MOVE_SELF

// inotifyNotifier watches every directory under the project with raw inotify.
// inotify has no recursive mode, so new directories are added as they appear.
// A wake pipe breaks the epoll wait on Close since a blocked read on an inotify
// fd is not reliable to cancel by closing it.
type inotifyNotifier struct {
	root    string
	events  chan<- struct{}
	record  func(path string)
	fd      int
	epfd    int
	wakeR   int
	wakeW   int
	watches map[uint32]string
	done    chan struct{}

	closeOnce   sync.Once
	releaseOnce sync.Once
}

func newNotifier(cfg notifierConfig) (notifier, error) {
	fd, err := syscall.InotifyInit1(syscall.IN_CLOEXEC | syscall.IN_NONBLOCK)
	if err != nil {
		return nil, fmt.Errorf("watcher: inotify init: %w", err)
	}
	epfd, err := syscall.EpollCreate1(syscall.EPOLL_CLOEXEC)
	if err != nil {
		_ = syscall.Close(fd)
		return nil, fmt.Errorf("watcher: epoll create: %w", err)
	}
	pipe := []int{0, 0}
	if err := syscall.Pipe2(pipe, syscall.O_CLOEXEC|syscall.O_NONBLOCK); err != nil {
		_ = syscall.Close(fd)
		_ = syscall.Close(epfd)
		return nil, fmt.Errorf("watcher: wake pipe: %w", err)
	}
	n := &inotifyNotifier{
		root:    cfg.root,
		events:  cfg.events,
		record:  cfg.record,
		fd:      fd,
		epfd:    epfd,
		wakeR:   pipe[0],
		wakeW:   pipe[1],
		watches: make(map[uint32]string),
		done:    make(chan struct{}),
	}
	if err := n.register(fd); err != nil {
		n.release()
		return nil, err
	}
	if err := n.register(pipe[0]); err != nil {
		n.release()
		return nil, err
	}
	if err := n.addWatch(cfg.root); err != nil {
		n.release()
		return nil, err
	}
	n.watchTree(cfg.root)
	go n.run()
	return n, nil
}

func (n *inotifyNotifier) register(fd int) error {
	event := syscall.EpollEvent{Events: syscall.EPOLLIN, Fd: int32(fd)} //nolint:gosec // fd is a small, non-negative descriptor.
	if err := syscall.EpollCtl(n.epfd, syscall.EPOLL_CTL_ADD, fd, &event); err != nil {
		return fmt.Errorf("watcher: epoll add fd %d: %w", fd, err)
	}
	return nil
}

func (n *inotifyNotifier) addWatch(dir string) error {
	wd, err := syscall.InotifyAddWatch(n.fd, dir, watcherMask)
	if err != nil {
		return fmt.Errorf("watcher: watch %s: %w", dir, err)
	}
	n.watches[uint32(wd)] = dir //nolint:gosec // wd is non-negative after the error check.
	return nil
}

// watchTree adds a watch for every directory under root, pruning ignored
// directories. Per-directory failures are skipped: the directory may have been
// removed between the walk and the watch.
func (n *inotifyNotifier) watchTree(root string) {
	_ = filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return nil //nolint:nilerr // a racing delete must not abort the walk.
		}
		if !entry.IsDir() {
			return nil
		}
		if path != n.root && isIgnoredDir(entry.Name()) {
			return fs.SkipDir
		}
		_ = n.addWatch(path)
		return nil
	})
}

func (n *inotifyNotifier) forgetTree(root string) {
	prefix := root + string(filepath.Separator)
	for wd, dir := range n.watches {
		if dir == root || strings.HasPrefix(dir, prefix) {
			delete(n.watches, wd)
		}
	}
}

func (n *inotifyNotifier) run() {
	defer close(n.done)
	events := make([]syscall.EpollEvent, 8)
	buffer := make([]byte, 64*1024)
	for {
		count, err := syscall.EpollWait(n.epfd, events, -1)
		if err != nil {
			if errors.Is(err, syscall.EINTR) {
				continue
			}
			return
		}
		for i := range count {
			if int(events[i].Fd) == n.wakeR {
				return
			}
			n.drain(int(events[i].Fd), buffer)
		}
	}
}

func (n *inotifyNotifier) drain(fd int, buffer []byte) {
	for {
		read, err := syscall.Read(fd, buffer)
		if err != nil || read <= 0 {
			return
		}
		n.dispatch(buffer[:read])
	}
}

// dispatch walks the packed event stream. The header is decoded little-endian:
// every Linux target this project builds for (amd64, arm64) is little-endian.
func (n *inotifyNotifier) dispatch(data []byte) {
	for offset := 0; offset+syscall.SizeofInotifyEvent <= len(data); {
		wd := binary.LittleEndian.Uint32(data[offset:])
		mask := binary.LittleEndian.Uint32(data[offset+4:])
		nameLen := int(binary.LittleEndian.Uint32(data[offset+12:]))
		offset += syscall.SizeofInotifyEvent
		var name string
		if nameLen > 0 && offset+nameLen <= len(data) {
			name = strings.TrimRight(string(data[offset:offset+nameLen]), "\x00")
		}
		offset += nameLen
		n.handle(wd, mask, name)
	}
}

func (n *inotifyNotifier) handle(wd uint32, mask uint32, name string) {
	if mask&syscall.IN_Q_OVERFLOW != 0 {
		n.signal()
		return
	}
	dir, ok := n.watches[wd]
	if !ok {
		return
	}
	if mask&syscall.IN_IGNORED != 0 {
		delete(n.watches, wd)
		return
	}
	path := dir
	if name != "" {
		path = filepath.Join(dir, name)
	}
	if mask&syscall.IN_ISDIR != 0 {
		n.handleDir(path, mask)
		return
	}
	n.report(path)
}

func (n *inotifyNotifier) handleDir(path string, mask uint32) {
	switch {
	case mask&(syscall.IN_CREATE|syscall.IN_MOVED_TO) != 0:
		if !isIgnoredDir(filepath.Base(path)) {
			n.watchTree(path)
		}
	case mask&(syscall.IN_DELETE|syscall.IN_MOVED_FROM|syscall.IN_DELETE_SELF) != 0:
		n.forgetTree(path)
	}
	// A directory carries no sidecar of its own; signal so the burst still
	// triggers a full scan when nothing else piled on.
	if relevantPath(n.root, path) {
		n.signal()
	}
}

// report records a relevant changed file as a project-relative path and signals.
func (n *inotifyNotifier) report(path string) {
	rel, err := filepath.Rel(n.root, path)
	if err != nil || !relevant(rel) {
		return
	}
	if n.record != nil {
		n.record(filepath.ToSlash(rel))
	}
	n.signal()
}

func (n *inotifyNotifier) signal() {
	select {
	case n.events <- struct{}{}:
	default:
	}
}

func (n *inotifyNotifier) release() {
	n.releaseOnce.Do(func() {
		_ = syscall.Close(n.fd)
		_ = syscall.Close(n.epfd)
		_ = syscall.Close(n.wakeR)
		_ = syscall.Close(n.wakeW)
	})
}

func (n *inotifyNotifier) Close() {
	n.closeOnce.Do(func() {
		_, _ = syscall.Write(n.wakeW, []byte{0})
		<-n.done
		n.release()
	})
}
