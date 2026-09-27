//go:build windows

package main

import "syscall"

// detachProcAttr returns no special attributes on Windows: detaching is
// best-effort there and the platform-specific creation flags are not wired up.
func detachProcAttr() *syscall.SysProcAttr {
	return nil
}
