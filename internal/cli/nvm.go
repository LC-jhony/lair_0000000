package cli

import (
	"context"
	"fmt"
	"io"

	"github.com/lair-sh/lair/internal/nvm"
)

// runNVM is the only entry point to nvm: `lair nvm <subcommand> ...`. It does
// not need the daemon and sources nvm.sh itself in a bash subprocess.
func runNVM(args []string, out, errOut io.Writer) error {
	if len(args) == 0 || isHelp(args[0]) {
		usageNVM(out)
		return nil
	}
	if err := nvm.ValidateArgs(args); err != nil {
		return err
	}
	result, err := nvm.Exec(context.Background(), args...)
	if result != "" {
		fmt.Fprint(out, result)
	}
	if err != nil {
		return fmt.Errorf("nvm %s: %w", args[0], err)
	}
	return nil
}

func usageNVM(w io.Writer) {
	fmt.Fprint(w, `lair nvm - Node version management through nvm

Usage:
  lair nvm install <version|lts|node>   Install a Node version with nvm
  lair nvm use <version|lts|system>     Use an installed version in this shell context
  lair nvm ls                           List installed versions
  lair nvm current                      Show the active version
  lair nvm exec <version> <command>...  Run a command with an installed version
  lair nvm uninstall <version>          Remove an installed version

Examples:
  lair nvm install 26
  lair nvm install lts/*
  lair nvm install 22.16.0
`)
}
