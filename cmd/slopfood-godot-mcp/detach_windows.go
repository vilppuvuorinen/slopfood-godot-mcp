//go:build windows

package main

import "syscall"

// createNewProcessGroup and detachedProcess are the Windows creation flags that
// put the daemon in its own process group and detach it from the client's
// console, so it survives the client stopping. DETACHED_PROCESS is not exported
// by the stdlib syscall package, so its documented value is spelled out here.
const (
	createNewProcessGroup = 0x00000200
	detachedProcess       = 0x00000008
)

// detachProcAttr returns the child attributes that detach a daemon on Windows.
// It is best-effort: the flags keep the child off the client's console and out
// of its process group, but Windows offers no exact equivalent of Setsid.
func detachProcAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{
		CreationFlags: createNewProcessGroup | detachedProcess,
	}
}
