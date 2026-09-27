//go:build !windows

package main

import "syscall"

// detachProcAttr returns the child attributes that put a detached daemon in its
// own session, so it survives the parent's terminal and signal group.
func detachProcAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{Setsid: true}
}
