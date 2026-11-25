package cli

import (
	"fmt"

	"github.com/lair-sh/lair/internal/daemon"
)

func (c *ctx) worktree(args []string) error {
	op, rest := "list", args
	if len(args) > 0 {
		op, rest = args[0], args[1:]
	}
	req := daemon.WorktreeRequest{Op: op}
	switch op {
	case "list", "sync", "prune":
	case "provision":
		if len(rest) < 1 {
			return fmt.Errorf("usage: lair worktree provision <ruta>")
		}
		req.Path = rest[0]
	default:
		return fmt.Errorf("unknown worktree subcommand %q (use list|sync|provision|prune)", op)
	}
	var result daemon.WorktreeResult
	if err := c.call(daemon.ActionWorktree, req, &result); err != nil {
		return err
	}
	if result.Output != "" {
		c.printf("%s\n", result.Output)
	}
	for _, w := range result.Worktrees {
		c.printf("%s  %s  %s\n", w.Name, w.Branch, w.Path)
	}
	return nil
}
