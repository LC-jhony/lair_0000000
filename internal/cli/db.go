package cli

import (
	"fmt"
	"strings"

	"github.com/lair-sh/lair/internal/daemon"
)

// db exposes the snapshot/restore/import/export surface as one CLI command
// with the same op-based pattern as mail and dumps.
func (c *ctx) db(args []string) error {
	op, rest := "list", args
	if len(args) > 0 {
		op, rest = args[0], args[1:]
	}
	rest, confirm := confirmFlag(rest)
	req := daemon.DBRequest{Op: op, Confirm: confirm}
	switch op {
	case "snapshot", "snapshots", "list", "snapshot:auto":
		if len(rest) > 0 {
			req.Site = rest[0]
		}
	case "restore":
		if len(rest) < 2 {
			return fmt.Errorf("usage: lair db restore <sitio> <snapshot> [--yes]")
		}
		req.Site, req.Name = rest[0], rest[1]
	case "import":
		if len(rest) < 2 {
			return fmt.Errorf("usage: lair db import <sitio> <archivo> [--yes]")
		}
		req.Site, req.File = rest[0], rest[1]
	case "export":
		if len(rest) < 1 {
			return fmt.Errorf("usage: lair db export <sitio> [archivo]")
		}
		req.Site = rest[0]
		if len(rest) > 1 {
			req.File = rest[1]
		}
	default:
		return fmt.Errorf("unknown db subcommand %q (use snapshot|snapshots|restore|import|export)", op)
	}
	var result daemon.DBResult
	if err := c.call(daemon.ActionDB, req, &result); err != nil {
		return err
	}
	if result.Output != "" {
		c.printf("%s\n", result.Output)
	}
	if op == "snapshots" || op == "list" {
		for _, s := range result.Snapshots {
			c.printf("%s  %s  %s  %d bytes  %s\n", s.ID, s.Site, s.Database, s.Bytes, s.Created)
		}
	}
	if strings.HasPrefix(op, "snapshot") && result.Output == "" {
		c.printf("snapshot creado\n")
	}
	return nil
}
