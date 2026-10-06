//go:build !linux

package watcher

// newNotifier polls the filesystem where inotify is unavailable.
func newNotifier(cfg notifierConfig) (notifier, error) {
	return newPollNotifier(cfg), nil
}
