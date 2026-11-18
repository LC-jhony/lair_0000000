//go:build windows

package cli

import (
	"os"
	"os/exec"

	"github.com/mattn/go-isatty"
)

// isTerminal reports whether f is an interactive Windows console.
func isTerminal(f *os.File) bool { return isatty.IsTerminal(f.Fd()) }

// execBinary falls back to a child process on Windows (no execve).
func execBinary(path string, argv, env []string) error {
	cmd := exec.Command(path, argv[1:]...)
	cmd.Env = env
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	return cmd.Run()
}
