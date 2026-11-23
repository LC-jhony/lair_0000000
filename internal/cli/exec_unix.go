//go:build !windows

package cli

import (
	"os"
	"syscall"

	"github.com/mattn/go-isatty"
)

// isTerminal reports whether f is an interactive TTY. A plain character-device
// check is not enough: /dev/null is a character device too, and a TUI started
// there fails while creating Bubble Tea's input reader.
func isTerminal(f *os.File) bool {
	return isatty.IsTerminal(f.Fd()) || isatty.IsCygwinTerminal(f.Fd())
}

// execBinary replaces the current process with the resolved binary. argv[0]
// must be that binary's path so self-locating tools work from the shims.
func execBinary(path string, argv, env []string) error {
	return syscall.Exec(path, argv, env)
}
