package cli

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/lair-sh/lair/internal/app"
	"github.com/lair-sh/lair/internal/daemon"
	"github.com/lair-sh/lair/internal/engine"
)

// runExec is the hidden entry point used by the shims. It resolves the exact
// binary for the current directory and replaces this process with it. When a
// version is missing and stdin is a terminal, it offers to install it.
func runExec(args []string, out, errOut io.Writer) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: lair exec <tool> [args...]")
	}
	tool, userArgs := args[0], args[1:]
	paths, err := mustPaths()
	if err != nil {
		return err
	}
	// A project may pin a different data root in .lairrc (root=...). The shim
	// honours it before resolving any version, so two checkouts can use fully
	// separate runtime sets without touching the shell environment.
	if dir, wdErr := os.Getwd(); wdErr == nil {
		if root := pinnedRoot(dir); root != "" {
			if abs, absErr := filepath.Abs(root); absErr == nil {
				_ = os.Setenv(app.EnvHome, abs)
				if resolved, rerr := app.Resolve(); rerr == nil {
					paths = resolved
				}
			}
		}
	}
	eng, err := engine.New(paths, nil)
	if err != nil {
		return err
	}

	inv, err := eng.ResolveTool(tool, "")
	if err != nil {
		if !isTerminal(os.Stdin) {
			return err
		}
		fmt.Fprintf(errOut, "lair: %v\nInstall the required version now? [y/N] ", err)
		line, _ := bufio.NewReader(os.Stdin).ReadString('\n')
		if strings.ToLower(strings.TrimSpace(line)) != "y" {
			return err
		}
		catalog, cerr := buildCatalog(paths)
		if cerr != nil {
			return cerr
		}
		_ = catalog.LoadFromCache()
		eng.Catalog = catalog
		cliCtx := &ctx{
			paths: paths, catalog: catalog, engine: eng,
			caller: daemon.NewDispatcher(eng, nil), out: out, errOut: errOut,
		}
		// Try a network refresh, but a fresh signed cache is enough offline.
		_, _ = eng.RefreshCatalog(context.Background())
		if ierr := cliCtx.installForTool(tool); ierr != nil {
			return ierr
		}
		inv, err = eng.ResolveTool(tool, "")
		if err != nil {
			return err
		}
	}

	env := append(append([]string(nil), inv.Env...), os.Environ()...)
	// argv[0] must be the resolved binary: PostgreSQL clients and the npm/npx
	// wrappers locate their files relative to it.
	argv := append([]string{inv.Path}, inv.Args...)
	argv = append(argv, userArgs...)
	return execBinary(inv.Path, argv, env)
}

// pinnedRoot reads root= from a project's .lairrc (or the legacy .flockrc).
func pinnedRoot(dir string) string {
	for _, name := range []string{".lairrc", ".flockrc"} {
		body, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			continue
		}
		for _, line := range strings.Split(string(body), "\n") {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "root=") {
				return strings.TrimSpace(strings.TrimPrefix(line, "root="))
			}
		}
	}
	return ""
}

// requestedVersion reads the version a project asks for, so a TTY prompt
// installs the pinned version instead of an unrelated default.
func requestedVersion(tool, dir string) string {
	names := []string{".lairrc", ".flockrc"}
	if tool == "node" {
		names = append(names, ".nvmrc", ".node-version")
	}
	for _, name := range names {
		body, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			continue
		}
		prefix := tool + "="
		for _, line := range strings.Split(string(body), "\n") {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, prefix) {
				if v := strings.TrimSpace(strings.TrimPrefix(line, prefix)); v != "" {
					return v
				}
			}
		}
		if tool == "node" && name != ".lairrc" && name != ".flockrc" {
			v := strings.Fields(strings.TrimSpace(string(body)))
			if len(v) > 0 && !strings.Contains(v[0], "*") && !strings.HasPrefix(v[0], "lts") && v[0] != "node" {
				return v[0]
			}
		}
	}
	return ""
}

// installForTool maps a shim name to the install action that satisfies it.
func (c *ctx) installForTool(tool string) error {
	switch tool {
	case "php", "php-fpm":
		dir, _ := os.Getwd()
		return c.installVersion(daemon.ActionInstallPHP, daemon.InstallRequest{Version: requestedVersion("php", dir)})
	case "composer":
		exe, _ := os.Executable()
		home, _ := os.UserHomeDir()
		var res daemon.BootstrapResult
		if err := c.call(daemon.ActionBootstrap, daemon.BootstrapRequest{
			LairBinary: exe, Home: home,
		}, &res); err != nil {
			return err
		}
		return c.installVersion(daemon.ActionInstallPHP, daemon.InstallRequest{})
	case "node", "npm", "npx":
		dir, _ := os.Getwd()
		return c.installVersion(daemon.ActionInstallNode, daemon.InstallRequest{Version: requestedVersion("node", dir)})
	case "psql", "pg_dump", "pg_restore", "createdb", "dropdb":
		return c.installVersion(daemon.ActionInstallService, daemon.InstallRequest{ID: "postgres"})
	case "mysql", "mysqldump":
		return c.installVersion(daemon.ActionInstallService, daemon.InstallRequest{ID: "mysql"})
	case "mariadb":
		return c.installVersion(daemon.ActionInstallService, daemon.InstallRequest{ID: "mariadb"})
	case "redis-cli":
		return c.installVersion(daemon.ActionInstallService, daemon.InstallRequest{ID: "redis"})
	case "mongosh":
		return c.installVersion(daemon.ActionInstallService, daemon.InstallRequest{ID: "mongodb"})
	default:
		return fmt.Errorf("cannot auto-install %s", tool)
	}
}
