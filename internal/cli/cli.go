// Package cli implements the `lair` command line interface. Every command
// talks to the daemon over the local API when it is running, and falls back to
// the same engine in-process otherwise (bootstrap, offline use and tests).
package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/lair-sh/lair/internal/app"
	"github.com/lair-sh/lair/internal/daemon"
	"github.com/lair-sh/lair/internal/engine"
	"github.com/lair-sh/lair/internal/manifest"
)

// defaultBaseURLs are the signed-manifest mirrors, tried in order.
var defaultBaseURLs = manifest.DefaultBaseURLs

// ctx carries the resolved environment for one CLI invocation.
type ctx struct {
	paths   app.Paths
	catalog *manifest.Catalog
	caller  daemon.Caller
	engine  *engine.Engine
	remote  bool
	out     io.Writer
	errOut  io.Writer
}

// newCtx resolves paths, loads the catalog cache and connects to the daemon
// when available.
func newCtx(out, errOut io.Writer) (*ctx, error) {
	paths, err := app.Resolve()
	if err != nil {
		return nil, err
	}
	catalog, err := buildCatalog(paths)
	if err != nil {
		return nil, err
	}

	c := &ctx{paths: paths, catalog: catalog, out: out, errOut: errOut}
	sock, _ := paths.SocketPath("api")
	if daemon.Available(sock) {
		if tok, err := daemon.ReadToken(filepath.Join(paths.Run, "api.token")); err == nil {
			c.caller = &daemon.Client{Socket: sock, Token: tok}
			c.remote = true
		}
	}
	if c.caller == nil {
		_ = catalog.LoadFromCache()
		eng, err := engine.New(paths, catalog)
		if err != nil {
			return nil, err
		}
		c.engine = eng
		c.caller = daemon.NewDispatcher(eng, nil)
	}
	return c, nil
}

func (c *ctx) call(action string, payload, out any) error {
	return c.callContext(context.Background(), action, payload, out)
}

// callContext is the context-aware form used by long polls (job waiting) so a
// stalled daemon cannot hang the CLI forever.
func (c *ctx) callContext(ctx context.Context, action string, payload, out any) error {
	return c.caller.Call(ctx, action, payload, out)
}

func (c *ctx) printf(format string, args ...any)  { fmt.Fprintf(c.out, format, args...) }
func (c *ctx) eprintf(format string, args ...any) { fmt.Fprintf(c.errOut, format, args...) }

// Run dispatches the CLI. It returns an error the main package renders.
func Run(args []string) error {
	return run(args, os.Stdout, os.Stderr)
}

func run(args []string, out, errOut io.Writer) error {
	if len(args) == 0 || isHelp(args[0]) {
		usage(out)
		return nil
	}
	cmd, rest := args[0], args[1:]

	// These commands do not need a daemon connection.
	if cmd == "daemon" {
		return runDaemon(rest, out, errOut)
	}
	if cmd == "exec" {
		return runExec(rest, out, errOut)
	}
	if cmd == "nvm" {
		return runNVM(rest, out, errOut)
	}
	if cmd == "desktop" || cmd == "ui" {
		return runDesktop(rest, out, errOut)
	}
	if cmd == "tui" || cmd == "dashboard" {
		return runTUI(rest, out, errOut)
	}

	c, err := newCtx(out, errOut)
	if err != nil {
		return err
	}
	switch cmd {
	case "install":
		return c.install(rest)
	case "uninstall":
		return c.uninstall(rest)
	case "status":
		return c.status(rest)
	case "doctor":
		return c.doctor(rest)
	case "restart":
		return c.restart()
	case "update":
		return c.update()
	case "php":
		return c.php(rest)
	case "node":
		return c.node(rest)
	case "composer":
		return c.composer(rest)
	case "project":
		return c.project(rest)
	case "migrate-dirs":
		return c.projectMigrateDirs(rest)
	case "import-valet":
		return c.projectImportValet()
	case "new":
		return c.newProject(rest)
	case "services", "service":
		return c.services(rest)
	case "park":
		return c.park()
	case "link":
		return c.link(rest)
	case "unlink":
		return c.unlink(rest)
	case "secure":
		return c.secure(rest)
	case "pin":
		return c.pin(rest)
	case "unpin":
		return c.unpin(rest)
	case "isolate":
		return c.isolate(rest)
	case "unisolate":
		return c.unisolate(rest)
	case "isolated":
		return c.isolated()
	case "use":
		return c.php(append([]string{"use"}, rest...))
	case "ext":
		return c.ext(rest)
	case "npm":
		if len(rest) > 0 && rest[0] == "update" {
			return c.npmUpdate()
		}
		return fmt.Errorf("usage: lair npm update")
	case "job":
		return c.job(rest)
	case "logs":
		return c.logs(rest)
	case "mail":
		return c.mail(rest)
	case "dumps":
		return c.dumps(rest)
	case "debugger":
		return c.debugger(rest)
	case "help", "--help", "-h":
		usage(out)
		return nil
	default:
		return fmt.Errorf("unknown command %q (try 'lair help')", cmd)
	}
}

func isHelp(s string) bool { return s == "help" || s == "--help" || s == "-h" }

func usage(w io.Writer) {
	fmt.Fprint(w, `lair - native PHP/Laravel development environments

Usage:
  lair install                       Install shims, Composer, CA and show the privileged setup plan
  lair uninstall [--delete-data]     Remove all Lair-installed runtimes, services and shims (databases kept unless --delete-data)
  lair status [--json]               Show PHP, Node, services, sites and jobs
  lair doctor [--fix]                Diagnose (and repair) PATH, shims and installed tools
  lair restart                       Restart all supervised services
  lair update                        Refresh the signed binary catalog
  lair php install [v] [--from P]    Install a PHP minor (local dir/.tar.gz/.zip with --from)
  lair php list                      List installed PHP versions
  lair php use <minor>               Set the default PHP minor line
  lair php uninstall <minor>         Remove a PHP minor (never the default)
  lair node install [v] [--from P]   Install the newest release of a Node major
  lair node list                     List installed Node versions
  lair node default <major>          Set the default Node version
  lair node update|uninstall <major> Update or remove a Node major
  lair composer install|update|version  Install, re-download/verify or show Composer
  lair npm update                    Install the newest npm compatible with the default Node
  lair ext [add <name>]              Show or install PHP ext-* requirements
  lair job retry <id>                Retry a failed install job
  lair new <name> [options]           Create a Laravel project (canonical command; alias of project new)
  lair project info                  Show Laravel installer, runtimes and database capabilities
  lair project dir [path]            Show or set the default folder for new sites (~/Lair)
  lair project dirs [list|add|remove|default]  Manage all project folders
  lair migrate-dirs [from to]        Move ~/lair to ~/Lair (or an explicit pair) and rewrite paths
  lair import-valet                  Read-only import of ~/.valet/config.json and ~/.valet/Sites
  lair project new <name> [options]  Create a Laravel project, database and *.test site
  lair nvm install <version|lts>     Install a Node version with nvm (all Node commands go through lair)
  lair nvm ls | current | use         Inspect or select an nvm version
  lair nvm exec <version> <cmd>      Run a command on an installed version
  lair services [list]               List services and their status
  lair services install <id> [ver] [--port N]
  lair services use <id> <version>   Change the active version
  lair services update <id> [ver]    Install newest, activate and restart
  lair services uninstall <id> [ver] [--delete-data]
  lair services create-db <id> <name>
  lair services default sqlite|mariadb|mysql|postgres  Set the default database
  lair services restart|stop <id>
  lair park                          Park the current directory as <name>.test
  lair link <name> [path]            Park a directory under an explicit host
  lair unlink <name>                 Remove a parked site
  lair secure [name]                 Issue a local HTTPS certificate for a site
  lair pin php|node|root <value>     Write a pin in .lairrc (root= overrides the data root)
  lair unpin php|node|root           Remove a pin from .lairrc
  lair isolate php|node <version>    Pin a version for a parked site (--off to clear)
  lair unisolate php|node            Clear a site isolation
  lair isolated                      Show active site isolations
  lair logs [name]                   Show a log file (or list logs)
  lair mail status|start|stop|list|clear [inbox]|open <id>|raw <id>|env  Local SMTP catcher
  lair dumps status|capture-on|capture-off|options|list|clear|ingest   PHP dumps
  lair debugger status|detect-on|detect-off|breakpoints-on|breakpoints-off
  lair desktop                       Show the local bridge details for the native app (alias: lair ui)
  lair tui | lair dashboard          Interactive CLI dashboard (Charmbracelet)
  lair daemon --foreground           Run the daemon in the foreground (--no-desktop, --desktop-port N)
`)
}
