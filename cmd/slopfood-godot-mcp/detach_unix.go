//go:build !windows

package main

import "syscall"

// Puts the daemon in its own session so it survives the parent's terminal and signal group.
func detachProcAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{Setsid: true}
}
