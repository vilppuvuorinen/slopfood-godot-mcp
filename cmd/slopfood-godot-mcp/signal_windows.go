//go:build windows

package main

import "os"

// shutdownSignals lists the signals that stop a role cleanly. Windows has no
// SIGTERM, so only the interrupt is caught.
func shutdownSignals() []os.Signal {
	return []os.Signal{os.Interrupt}
}
