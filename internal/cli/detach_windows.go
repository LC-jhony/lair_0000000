//go:build windows

package cli

import "syscall"

// detachAttr keeps a daemon started by `lair desktop` alive after the CLI exits.
// DETACHED_PROCESS (0x00000008) is not exported by the standard syscall
// package, so it is declared locally with its Win32 value.
const detachedProcess = 0x00000008

func detachAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{
		CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP | detachedProcess,
	}
}
