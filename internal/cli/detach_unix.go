//go:build !windows

package cli

import "syscall"

// detachAttr keeps a daemon started by `lair desktop` alive after the CLI exits.
func detachAttr() *syscall.SysProcAttr { return &syscall.SysProcAttr{Setsid: true} }
