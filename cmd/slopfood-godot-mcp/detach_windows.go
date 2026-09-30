//go:build windows

package main

import "syscall"

// DETACHED_PROCESS is not exported by stdlib syscall, so its value is spelled out here.
const (
	createNewProcessGroup = 0x00000200
	detachedProcess       = 0x00000008
)

func detachProcAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{
		CreationFlags: createNewProcessGroup | detachedProcess,
	}
}
