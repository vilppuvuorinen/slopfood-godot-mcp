//go:build !windows

package main

import (
	"os"
	"syscall"
)

// shutdownSignals lists the signals that stop a role cleanly. SIGTERM is
// available off Windows only; Windows has no equivalent to catch.
func shutdownSignals() []os.Signal {
	return []os.Signal{os.Interrupt, syscall.SIGTERM}
}
