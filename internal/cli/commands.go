package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/lair-sh/lair/internal/config"
	"github.com/lair-sh/lair/internal/daemon"
	"github.com/lair-sh/lair/internal/engine"
	"github.com/lair-sh/lair/internal/installer"
	"github.com/lair-sh/lair/internal/platform"
	"github.com/lair-sh/lair/internal/shims"
	"github.com/lair-sh/lair/internal/version"
)

// install runs the local bootstrap and renders the one-time privileged plan.
func (c *ctx) install(args []string) error {
	var req daemon.BootstrapRequest
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	req.LairBinary = exe
	if home, err := os.UserHomeDir(); err == nil {
		req.Home = home
	}
	var result daemon.BootstrapResult
	if err := c.call(daemon.ActionBootstrap, req, &result); err != nil {
		return fmt.Errorf("bootstrap: %w", err)
	}
	c.printf("Local setup complete.\n  shims:    %s\n  composer: %s\n\n", result.BinDir, composerLabel(result.ComposerPath))
	for _, warning := range result.Warnings {
		c.eprintf("lair: advertencia: %s\n", warning)
	}
	// MariaDB is the default database of a fresh Lair installation. A missing
	// catalog or network must not undo the local setup, so failures are warnings.
	var db struct {
		ID               string `json:"id"`
		AlreadyInstalled bool   `json:"already_installed"`
	}
	if err := c.call(daemon.ActionEnsureDatabase, nil, &db); err != nil {
		c.eprintf("lair: advertencia: no se pudo instalar MariaDB: %v (instálala después con 'lair services install mariadb')\n", err)
	} else if db.AlreadyInstalled {
		c.printf("Base de datos: ya hay una instalada.\n")
	} else {
		c.printf("Base de datos: instalando %s (por defecto).\n", engine.DefaultDatabaseID)
		// An install started in the offline dispatcher lives in this process: if
		// we returned now the CLI would exit and kill the download. With a daemon
		// the job survives anyway, but waiting gives honest progress in both.
		if err := c.waitJob(db.ID); err != nil {
			c.eprintf("lair: advertencia: falló la instalación de MariaDB: %v (reintenta con 'lair services install mariadb')\n", err)
		}
	}
	c.printf("Abre una terminal nueva (o ejecuta `source ~/.bashrc` / `source ~/.zshrc`) para actualizar PATH.\n")
	c.printf("Comprueba que todo responde con: node -v && npm -v && lair doctor\n\n")

	domain := "test"
	if c.engine != nil {
		domain = c.engine.WebSettings().Domain
	}
	cfg := platform.UnitConfig{
		User: currentUser(), Home: req.Home, DataDir: c.paths.Root,
		ExecPath: exe, Domain: domain, DNSAddress: platform.LegacyDNSAddress,
		DNSBinary: exe,
	}
	plan, err := platform.BuildPlan(cfg)
	if err != nil {
		return fmt.Errorf("privileged plan: %w", err)
	}
	setup := filepath.Join(c.paths.Root, "setup-privileged.sh")
	if err := os.WriteFile(setup, []byte(plan.Script()), 0o700); err != nil {
		return err
	}
	trust := platform.InstallTrust(filepath.Join(c.paths.Certs, "ca.pem"))
	trustSystem := filepath.Join(c.paths.Root, trustSystemScript)
	if err := os.WriteFile(trustSystem, []byte("#!/bin/sh\n"+trust.SystemScript), 0o700); err != nil {
		return err
	}
	trustUser := filepath.Join(c.paths.Root, trustUserScript)
	if err := os.WriteFile(trustUser, []byte("#!/bin/sh\n"+trust.UserScript), 0o700); err != nil {
		return err
	}
	c.printf("One-time privileged setup (elevation is requested once, before anything is replaced):\n")
	c.printf("  sudo %s\n  sudo %s --add\n", setup, trustSystem)
	c.printf("  %s --add        # browser NSS trust, runs as your user\n", trustUser)
	c.printf("  sudo systemctl enable --now lair@%s\n\n", currentUser())
	c.printf("Review these scripts before running them. Then open a new shell to pick up PATH.\n")
	return nil
}

// Names of the generated trust helpers and the privileged reversal script.
const (
	trustSystemScript = "trust-ca.sh"
	trustUserScript   = "trust-ca-user.sh"
	revertPrivileged  = "revert-privileged.sh"
)

func currentUser() string {
	if u, err := user.Current(); err == nil && u.Username != "" {
		return u.Username
	}
	if v := os.Getenv("USER"); v != "" {
		return v
	}
	return os.Getenv("LOGNAME")
}

func composerLabel(phar string) string {
	if phar == "" {
		return "(pendiente: sin red; vuelve a ejecutar 'lair install')"
	}
	return phar
}

// ensureShims regenerates the shims and PATH snippets after an install so that
// `lair node install` alone is enough to get `node`/`npm` on PATH. It never
// fails the install: it only reports a warning if the environment cannot be
// updated.
func (c *ctx) ensureShims() {
	missing, onPath := shimStatus(c.paths.Bin)
	// Go through the daemon so the engine's data root is authoritative also
	// when shims were requested by the TUI or the desktop app. An older daemon
	// may not know the action yet, so keep a local fallback.
	var result struct {
		BinDir string `json:"bin_dir"`
	}
	var err error
	if c.caller != nil {
		err = c.call(daemon.ActionEnsureShims, daemon.EnsureShimsRequest{}, &result)
	} else {
		err = fmt.Errorf("no daemon or engine available")
	}
	if err != nil {
		exe, exeErr := os.Executable()
		if exeErr != nil {
			c.eprintf("lair: no se pudieron regenerar los shims: %v\n", exeErr)
			return
		}
		home, _ := os.UserHomeDir()
		if genErr := shims.Generate(c.paths.Bin, exe); genErr != nil {
			c.eprintf("lair: no se pudieron regenerar los shims: %v\n", genErr)
			return
		}
		if profileErr := shims.EnsureProfiles(home, c.paths.Bin); profileErr != nil {
			c.eprintf("lair: no se pudo actualizar el PATH: %v\n", profileErr)
			return
		}
	}
	if len(missing) > 0 || !onPath {
		c.printf("\nShims y PATH actualizados en %s.\n", c.paths.Bin)
		c.printf("Abre una terminal nueva (o `source ~/.bashrc` / `source ~/.zshrc`) y comprueba con: node -v\n")
	}
}

// shimStatus reports missing shims and whether binDir is already on PATH.
func shimStatus(binDir string) (missing []string, onPath bool) {
	for _, tool := range shims.Tools {
		if _, err := os.Stat(filepath.Join(binDir, tool)); err != nil {
			missing = append(missing, tool)
		}
	}
	onPath = strings.Contains(":"+os.Getenv("PATH")+":", ":"+binDir+":")
	return missing, onPath
}

// uninstall renders the reversal plan and removes local state.
func (c *ctx) uninstall(args []string) error {
	deleteData := hasFlag(args, "--delete-data")
	home, _ := os.UserHomeDir()

	// The reversal scripts invoke the saved CA helpers, so they must live in a
	// directory that survives this command. With --delete-data the whole data
	// root is removed, so keep them in $HOME instead.
	revDir := c.paths.Root
	if deleteData && home != "" {
		revDir = home
	}
	trust := platform.InstallTrust(filepath.Join(c.paths.Certs, "ca.pem"))
	trustSystem := filepath.Join(revDir, trustSystemScript)
	if err := os.WriteFile(trustSystem, []byte("#!/bin/sh\n"+trust.SystemScript), 0o700); err != nil {
		return err
	}
	trustUser := filepath.Join(revDir, trustUserScript)
	if err := os.WriteFile(trustUser, []byte("#!/bin/sh\n"+trust.UserScript), 0o700); err != nil {
		return err
	}

	revert := filepath.Join(revDir, revertPrivileged)
	script := "#!/bin/sh\n# Generated by Lair. Review before running with elevation.\nset -eu\n"
	script += "systemctl disable --now lair@" + currentUser() + " 2>/dev/null || true\n"
	script += "systemctl disable --now lair-dns.service 2>/dev/null || true\n"
	script += "rm -f /etc/systemd/system/lair@.service /etc/systemd/system/lair-dns.service\n"
	script += "rm -rf /etc/systemd/system/lair@" + currentUser() + ".service.d\n"
	script += "rm -f /etc/polkit-1/rules.d/50-lair.rules /etc/systemd/resolved.conf.d/50-lair.conf\n"
	script += "systemctl daemon-reload && systemctl restart systemd-resolved\n"
	// Invoke the saved helper explicitly with --remove. Inlining its source
	// made it read *this* script's arguments: with $1 empty it took the default
	// branch and re-installed the CA instead of removing it.
	script += fmt.Sprintf("if [ -x %q ]; then %q --remove; fi\n", trustSystem, trustSystem)
	if err := os.WriteFile(revert, []byte(script), 0o700); err != nil {
		return err
	}

	req := daemon.UninstallRequest{Home: home, DataDir: c.paths.Root, DeleteData: deleteData}
	if err := c.call(daemon.ActionUninstall, req, nil); err != nil {
		return err
	}
	if deleteData {
		c.printf("Lair and all its data under %s were removed.\n", c.paths.Root)
	} else {
		c.printf("Removed PHP, Composer, Node, all service binaries (MariaDB included), shims, PATH entries and local CA.\n")
		c.printf("Service databases were preserved; use --delete-data to remove them too.\n")
	}
	c.printf("Laravel Valet and Mailcatcher are not installed by Lair; system packages are left untouched.\n")
	c.printf("To finish, review and run (elevation only for the first script):\n")
	c.printf("  1. sudo %s\n", revert)
	c.printf("  2. %s --remove\n", trustUser)
	if deleteData {
		c.printf("The reversal scripts were kept in %s.\n", revDir)
	}
	return nil
}

func (c *ctx) status(args []string) error {
	var st engine.Status
	if err := c.call(daemon.ActionStatus, nil, &st); err != nil {
		return err
	}
	if hasFlag(args, "--json") {
		data, err := json.MarshalIndent(st, "", "  ")
		if err != nil {
			return err
		}
		c.printf("%s\n", data)
		return nil
	}
	c.printf("Lair home: %s\n", st.Home)
	if c.remote {
		c.printf("Daemon:    running (local API)\n")
	} else {
		c.printf("Daemon:    not running (offline view)\n")
	}
	c.printf("\nPHP:\n")
	if len(st.PHP) == 0 {
		c.printf("  (none installed)\n")
	}
	for _, p := range st.PHP {
		active := ""
		if st.Defaults["php"] == p.Minor {
			active = " (default)"
		}
		c.printf("  %-8s %s%s\n", p.Minor, p.Version, active)
	}
	c.printf("\nNode:\n")
	if len(st.Node) == 0 {
		c.printf("  (none installed)\n")
	}
	for _, v := range st.Node {
		active := ""
		if st.Defaults["node"] == v {
			active = " (default)"
		}
		c.printf("  %s%s\n", v, active)
	}
	defaultDB := st.Defaults["database"]
	if defaultDB == "" {
		defaultDB = engine.DefaultDatabaseID
	}
	c.printf("\nServices:\n")
	c.printf("  default database: %s\n", defaultDB)
	ids := make([]string, 0, len(st.Services))
	for id := range st.Services {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		s := st.Services[id]
		state := "stopped"
		if s.Running {
			state = fmt.Sprintf("running (pid %d)", s.PID)
		}
		active := s.Active
		if active == "" {
			active = "-"
		}
		c.printf("  %-12s active=%-10s port=%-6d %s\n", id, active, s.Port, state)
		if s.LastError != "" {
			c.printf("      last error: %s\n", s.LastError)
		}
	}
	c.printf("\nSites:\n")
	if len(st.Sites) == 0 {
		c.printf("  (none parked)\n")
	}
	for _, s := range st.Sites {
		c.printf("  https://%s -> %s\n", s.Name, s.Root)
	}
	if len(st.Jobs) > 0 {
		c.printf("\nJobs:\n")
		for _, j := range st.Jobs {
			c.printf("  %s: %s (%d/%d)\n", j.ID, j.Phase, j.Downloaded, j.Total)
		}
	}
	return nil
}

func (c *ctx) restart() error {
	var out map[string]bool
	if err := c.call(daemon.ActionRestart, nil, &out); err != nil {
		return err
	}
	c.printf("Services restarted.\n")
	return nil
}

func (c *ctx) update() error {
	var result daemon.TextResult
	if err := c.call(daemon.ActionUpdate, nil, &result); err != nil {
		return fmt.Errorf("update: %w", err)
	}
	text := strings.TrimSpace(result.Text)
	if text == "" {
		text = "Catálogo firmado actualizado."
	}
	c.printf("%s\n", text)
	return nil
}

// jobWaitTimeout bounds how long a CLI invocation waits for a background job.
// A variable so tests can shorten the wait without sleeping for minutes.
var jobWaitTimeout = 30 * time.Minute

// waitJob polls a job until completion and surfaces its error.
func (c *ctx) waitJob(id string) error {
	ctx, cancel := context.WithTimeout(context.Background(), jobWaitTimeout)
	defer cancel()
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		var st installer.Status
		if err := c.callContext(ctx, daemon.ActionJob, daemon.JobRequest{ID: id}, &st); err != nil {
			return err
		}
		if st.Done {
			if st.Error != "" {
				return fmt.Errorf("%s", st.Error)
			}
			c.printf("Installed %s.\n", id)
			return nil
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("waiting for %s: %w", id, ctx.Err())
		case <-ticker.C:
		}
	}
}

// installVersion calls an install action and waits for the resulting job.
func (c *ctx) installVersion(action string, req daemon.InstallRequest) error {
	var st installer.Status
	if err := c.call(action, req, &st); err != nil {
		return err
	}
	label := req.Version
	if label == "" {
		label = st.ID
	}
	c.printf("Installing %s...\n", label)
	if err := c.waitJob(st.ID); err != nil {
		return err
	}
	c.ensureShims()
	return nil
}

func (c *ctx) php(args []string) error {
	if len(args) == 0 || isHelp(args[0]) {
		c.printf("usage: lair php install <version> [--from <dir|.tar.gz|.zip>]|list|use|uninstall <minor>\n")
		return nil
	}
	switch args[0] {
	case "install":
		ver, from := parseInstallArgs(args[1:])
		return c.installVersion(daemon.ActionInstallPHP, daemon.InstallRequest{Version: ver, From: from})
	case "list":
		var st engine.Status
		if err := c.call(daemon.ActionStatus, nil, &st); err != nil {
			return err
		}
		if len(st.PHP) == 0 {
			c.printf("No PHP versions installed.\n")
		}
		for _, p := range st.PHP {
			mark := " "
			if st.Defaults["php"] == p.Minor {
				mark = "*"
			}
			c.printf("%s %-8s %s\n", mark, p.Minor, p.Version)
		}
		return nil
	case "use":
		if len(args) < 2 {
			return fmt.Errorf("usage: lair php use <minor>")
		}
		if err := c.call(daemon.ActionPHPUse, daemon.PHPUseRequest{Minor: args[1]}, nil); err != nil {
			return err
		}
		c.printf("Default PHP is now %s.\n", args[1])
		return nil
	case "uninstall":
		if len(args) < 2 {
			return fmt.Errorf("usage: lair php uninstall <minor>")
		}
		if err := c.call(daemon.ActionPHPUninstall, daemon.PHPUseRequest{Minor: args[1]}, nil); err != nil {
			return err
		}
		c.printf("PHP %s uninstalled.\n", args[1])
		return nil
	default:
		return fmt.Errorf("unknown php subcommand %q", args[0])
	}
}

// parseInstallArgs splits a version from the optional --from flag used by
// `php install` and `node install` for local, unsigned builds.
func parseInstallArgs(args []string) (version, from string) {
	for i := 0; i < len(args); i++ {
		if args[i] == "--from" && i+1 < len(args) {
			from = args[i+1]
			i++
			continue
		}
		if strings.HasPrefix(args[i], "--from=") {
			from = strings.TrimPrefix(args[i], "--from=")
			continue
		}
		if version == "" {
			version = args[i]
		}
	}
	return version, from
}

func (c *ctx) node(args []string) error {
	if len(args) == 0 || isHelp(args[0]) {
		c.printf("usage: lair node install <version> [--from <dir|.tar.gz>]|list|default|update|uninstall\n")
		return nil
	}
	switch args[0] {
	case "install":
		ver, from := parseInstallArgs(args[1:])
		return c.installVersion(daemon.ActionInstallNode, daemon.InstallRequest{Version: ver, From: from})
	case "list":
		var st engine.Status
		if err := c.call(daemon.ActionStatus, nil, &st); err != nil {
			return err
		}
		if len(st.Node) == 0 {
			c.printf("No Node versions installed.\n")
		}
		for _, v := range st.Node {
			mark := " "
			if st.Defaults["node"] == v {
				mark = "*"
			}
			c.printf("%s %s\n", mark, v)
		}
		return nil
	case "default":
		if len(args) < 2 {
			return fmt.Errorf("usage: lair node default <version>")
		}
		if err := c.call(daemon.ActionNodeDefault, daemon.NodeDefaultRequest{Version: args[1]}, nil); err != nil {
			return err
		}
		c.printf("Default Node is now %s.\n", args[1])
		return nil
	case "update":
		major := ""
		if len(args) > 1 {
			major = args[1]
		}
		return c.installVersion(daemon.ActionNodeUpdate, daemon.InstallRequest{Version: major})
	case "uninstall":
		if len(args) < 2 {
			return fmt.Errorf("usage: lair node uninstall <major>")
		}
		if err := c.call(daemon.ActionNodeUninstall, daemon.NodeDefaultRequest{Version: args[1]}, nil); err != nil {
			return err
		}
		c.printf("Node %s uninstalled.\n", args[1])
		return nil
	default:
		return fmt.Errorf("unknown node subcommand %q", args[0])
	}
}

// composer installs or revalidates Composer through the same bootstrap action
// used by `lair install`, so there is no second daemon action for one tool.
func (c *ctx) composer(args []string) error {
	if len(args) == 0 || isHelp(args[0]) {
		c.printf("usage: lair composer [install|update|version]\n")
		return nil
	}
	if args[0] == "update" {
		var result daemon.TextResult
		if err := c.call(daemon.ActionComposerUpdate, nil, &result); err != nil {
			return err
		}
		c.printf("Composer: %s\n", strings.TrimSpace(result.Text))
		c.ensureShims()
		return nil
	}
	if args[0] == "version" {
		var st engine.Status
		if err := c.call(daemon.ActionStatus, nil, &st); err != nil {
			return err
		}
		if st.Composer == "" {
			return fmt.Errorf("composer.phar is not installed")
		}
		c.printf("Composer %s\n", st.Composer)
		return nil
	}
	if args[0] != "install" {
		return fmt.Errorf("unknown composer subcommand %q", args[0])
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	req := daemon.BootstrapRequest{LairBinary: exe}
	if home, err := os.UserHomeDir(); err == nil {
		req.Home = home
	}
	var result daemon.BootstrapResult
	if err := c.call(daemon.ActionBootstrap, req, &result); err != nil {
		return err
	}
	c.printf("Composer: %s\n", composerLabel(result.ComposerPath))
	for _, warning := range result.Warnings {
		c.eprintf("lair: advertencia: %s\n", warning)
	}
	c.ensureShims()
	return nil
}

// serviceArgs parses a services subcommand's positional args and flags.
type serviceArgs struct {
	id         string
	version    string
	name       string
	port       int
	deleteData bool
	rest       []string
}

func parseServiceArgs(args []string) (serviceArgs, error) {
	var s serviceArgs
	pos := []string{}
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--port":
			if i+1 >= len(args) {
				return s, fmt.Errorf("--port requires a value")
			}
			port, err := strconv.Atoi(args[i+1])
			if err != nil || port < 1 || port > 65535 {
				return s, fmt.Errorf("invalid port %q (use 1-65535)", args[i+1])
			}
			s.port = port
			i++
		case "--delete-data":
			s.deleteData = true
		default:
			pos = append(pos, args[i])
		}
	}
	if len(pos) > 0 {
		s.id = pos[0]
	}
	if len(pos) > 1 {
		s.version = pos[1]
	}
	if len(pos) > 2 {
		s.name = pos[2]
	}
	s.rest = pos
	return s, nil
}

func (c *ctx) services(args []string) error {
	if len(args) == 0 || isHelp(args[0]) || args[0] == "list" {
		return c.status(nil)
	}
	sub := args[0]
	a, err := parseServiceArgs(args[1:])
	if err != nil {
		return err
	}
	// create-db takes <id> <name>; the generic parser stores the second
	// positional as version, so move it for this subcommand only.
	if sub == "create-db" {
		a.name, a.version = a.version, ""
	}
	switch sub {
	case "install":
		if a.id == "" {
			return fmt.Errorf("usage: lair services install <id> [version] [--port N]")
		}
		return c.installVersion(daemon.ActionInstallService, daemon.InstallRequest{
			ID: a.id, Version: a.version, Port: a.port,
		})
	case "use":
		if a.id == "" || a.version == "" {
			return fmt.Errorf("usage: lair services use <id> <version>")
		}
		if err := c.call(daemon.ActionServiceUse, daemon.ServiceUseRequest{ID: a.id, Version: a.version}, nil); err != nil {
			return err
		}
		c.printf("%s %s is now active.\n", a.id, a.version)
		return nil
	case "update":
		if a.id == "" {
			return fmt.Errorf("usage: lair services update <id> [version]")
		}
		if err := c.installVersion(daemon.ActionInstallService, daemon.InstallRequest{
			ID: a.id, Version: a.version, Port: a.port,
		}); err != nil {
			return err
		}
		var versions daemon.VersionResult
		if err := c.call(daemon.ActionVersions, daemon.VersionRequest{Package: a.id}, &versions); err != nil {
			return err
		}
		// The installer resolves "11"/"11.4"/"latest" to an exact version; do
		// the same here before activating, otherwise service-use gets a
		// partial version and silently fails to switch.
		pick, err := version.ExactOrLatest(a.version, versions.Versions)
		if err != nil {
			return err
		}
		if err := c.call(daemon.ActionServiceUse, daemon.ServiceUseRequest{ID: a.id, Version: pick}, nil); err != nil {
			return err
		}
		return c.call(daemon.ActionServiceRestart, daemon.ServiceRequest{ID: a.id}, nil)
	case "uninstall":
		if a.id == "" {
			return fmt.Errorf("usage: lair services uninstall <id> [version] [--delete-data]")
		}
		req := daemon.ServiceUninstallRequest{ID: a.id, Version: a.version, DeleteData: a.deleteData}
		if err := c.call(daemon.ActionServiceUninstall, req, nil); err != nil {
			return err
		}
		c.printf("Removed %s %s.\n", a.id, a.version)
		if !a.deleteData {
			c.printf("Data was preserved; use --delete-data to remove it.\n")
		}
		return nil
	case "default":
		if a.id == "" {
			return fmt.Errorf("usage: lair services default mariadb|mysql|postgres")
		}
		if err := c.call(daemon.ActionDBDefault, daemon.DBDefaultRequest{ID: a.id}, nil); err != nil {
			return err
		}
		c.printf("%s is now the default database.\n", a.id)
		return nil
	case "create-db":
		if a.id == "" || a.name == "" {
			return fmt.Errorf("usage: lair services create-db <id> <name>")
		}
		var out daemon.TextResult
		if err := c.call(daemon.ActionCreateDB, daemon.CreateDBRequest{ID: a.id, Name: a.name}, &out); err != nil {
			return err
		}
		if out.Text != "" {
			c.printf("%s", out.Text)
		}
		c.printf("Database %s ready in %s.\n", a.name, a.id)
		return nil
	case "restart", "stop":
		if a.id == "" {
			return fmt.Errorf("usage: lair services %s <id>", sub)
		}
		action := daemon.ActionServiceRestart
		if sub == "stop" {
			action = daemon.ActionServiceStop
		}
		if err := c.call(action, daemon.ServiceRequest{ID: a.id}, nil); err != nil {
			return err
		}
		c.printf("%s %s.\n", capitalize(sub), a.id)
		return nil
	default:
		return fmt.Errorf("unknown services subcommand %q", sub)
	}
}

func (c *ctx) park() error {
	dir, err := os.Getwd()
	if err != nil {
		return err
	}
	var out daemon.TextResult
	if err := c.call(daemon.ActionPark, daemon.PathRequest{Path: dir}, &out); err != nil {
		return err
	}
	c.printf("Parked %s at https://%s\n", dir, out.Text)
	return nil
}

func (c *ctx) link(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: lair link <name> [path]")
	}
	path := "."
	if len(args) > 1 {
		path = args[1]
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	if err := c.call(daemon.ActionLink, daemon.LinkRequest{Name: args[0], Path: abs}, nil); err != nil {
		return err
	}
	c.printf("Linked https://%s -> %s\n", strings.TrimSuffix(args[0], "."), abs)
	return nil
}

func (c *ctx) unlink(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: lair unlink <name>")
	}
	if err := c.call(daemon.ActionUnlink, daemon.LogsRequest{Name: args[0]}, nil); err != nil {
		return err
	}
	c.printf("Unlinked %s.\n", args[0])
	return nil
}

func (c *ctx) secure(args []string) error {
	name := ""
	if len(args) > 0 {
		name = args[0]
	} else {
		dir, _ := os.Getwd()
		var st engine.Status
		if err := c.call(daemon.ActionStatus, nil, &st); err != nil {
			return err
		}
		best := ""
		for _, s := range st.Sites {
			if s.Root == dir || strings.HasPrefix(dir, s.Root+string(os.PathSeparator)) {
				if len(s.Root) > len(best) {
					best, name = s.Root, s.Name
				}
			}
		}
		if name == "" {
			return fmt.Errorf("no site found for %s; run 'lair park' or pass a name", dir)
		}
	}
	if err := c.call(daemon.ActionSecure, daemon.LogsRequest{Name: name}, nil); err != nil {
		return err
	}
	c.printf("HTTPS certificate ready for %s.\n", name)
	c.printf("If this is the first site, re-run 'sudo %s --add' to trust the local CA.\n",
		filepath.Join(c.paths.Root, "trust-ca.sh"))
	return nil
}

func (c *ctx) pin(args []string) error {
	if len(args) < 2 {
		return fmt.Errorf("usage: lair pin php|node <version>")
	}
	if args[0] != "php" && args[0] != "node" {
		return fmt.Errorf("pin accepts only php or node")
	}
	dir, _ := os.Getwd()
	req := daemon.PinRequest{Tool: args[0], Value: args[1], Dir: dir}
	if err := c.call(daemon.ActionPin, req, nil); err != nil {
		return err
	}
	c.printf("Pinned %s=%s in .lairrc\n", args[0], args[1])
	return nil
}

func (c *ctx) isolate(args []string) error {
	if len(args) < 2 {
		return fmt.Errorf("usage: lair isolate php|node <version|--off>")
	}
	if args[0] != "php" && args[0] != "node" {
		return fmt.Errorf("isolate accepts only php or node")
	}
	dir, _ := os.Getwd()
	value := args[1]
	if value == "--off" || value == "off" {
		value = ""
	}
	req := daemon.IsolateRequest{Tool: args[0], Value: value, Dir: dir}
	if err := c.call(daemon.ActionIsolate, req, nil); err != nil {
		return err
	}
	if value == "" {
		c.printf("Cleared %s isolation for this site.\n", args[0])
	} else {
		c.printf("Isolated %s %s for this site.\n", args[0], value)
	}
	return nil
}

// ext reports required PHP extensions or installs one with the project's PHP.
func (c *ctx) ext(args []string) error {
	dir, _ := os.Getwd()
	if len(args) == 0 || isHelp(args[0]) {
		var info daemon.ExtensionInfo
		if err := c.call(daemon.ActionExtRequired, daemon.PathRequest{Path: dir}, &info); err != nil {
			return err
		}
		if len(info.Required) == 0 {
			c.printf("No ext-* requirements found in this project.\n")
			return nil
		}
		c.printf("Required extensions: %s\n", strings.Join(info.Required, ", "))
		if len(info.Missing) > 0 {
			c.printf("Missing: %s (install with 'lair ext add <name>')\n", strings.Join(info.Missing, ", "))
		}
		return nil
	}
	if args[0] != "add" || len(args) < 2 {
		return fmt.Errorf("usage: lair ext [add <name>]")
	}
	var result daemon.TextResult
	if err := c.call(daemon.ActionExtAdd, daemon.ExtRequest{Ext: args[1], Dir: dir}, &result); err != nil {
		return err
	}
	if text := strings.TrimSpace(result.Text); text != "" {
		c.printf("%s\n", text)
	}
	return nil
}

func (c *ctx) unpin(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: lair unpin php|node|root")
	}
	dir, _ := os.Getwd()
	if err := c.call(daemon.ActionUnpin, daemon.PinRequest{Tool: args[0], Dir: dir}, nil); err != nil {
		return err
	}
	c.printf("Removed %s pin from .lairrc.\n", args[0])
	return nil
}

func (c *ctx) unisolate(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: lair unisolate php|node")
	}
	dir, _ := os.Getwd()
	if err := c.call(daemon.ActionUnisolate, daemon.IsolateRequest{Tool: args[0], Dir: dir}, nil); err != nil {
		return err
	}
	c.printf("Cleared %s isolation for this site.\n", args[0])
	return nil
}

func (c *ctx) isolated() error {
	dir, _ := os.Getwd()
	var result map[string]string
	if err := c.call(daemon.ActionIsolated, daemon.PathRequest{Path: dir}, &result); err != nil {
		return err
	}
	for _, tool := range []string{"php", "node"} {
		if v := result[tool]; v != "" {
			c.printf("%s=%s\n", tool, v)
		}
	}
	return nil
}

func (c *ctx) npmUpdate() error {
	dir, _ := os.Getwd()
	var result daemon.TextResult
	if err := c.call(daemon.ActionNpmUpdate, daemon.PathRequest{Path: dir}, &result); err != nil {
		return err
	}
	c.printf("npm updated to %s\n", strings.TrimSpace(result.Text))
	return nil
}

func (c *ctx) job(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: lair job retry <id>")
	}
	if args[0] != "retry" || len(args) < 2 {
		return fmt.Errorf("usage: lair job retry <id>")
	}
	var st installer.Status
	if err := c.call(daemon.ActionJobRetry, daemon.JobRequest{ID: args[1]}, &st); err != nil {
		return err
	}
	return c.waitJob(st.ID)
}

func (c *ctx) logs(args []string) error {
	if len(args) == 0 {
		var list daemon.NameList
		if err := c.call(daemon.ActionLogsList, nil, &list); err != nil {
			return err
		}
		for _, name := range list.Items {
			c.printf("%s\n", name)
		}
		return nil
	}
	var out daemon.LogsResult
	if err := c.call(daemon.ActionLogs, daemon.LogsRequest{Name: args[0]}, &out); err != nil {
		return err
	}
	c.printf("%s", out.Text)
	return nil
}

// doctor reports the state of a Lair installation from the shell's point of
// view. It is the first command to run when `node -v` or `php -v` is not found.
func (c *ctx) doctor(args []string) error {
	c.printf("Lair doctor\n")
	if hasFlag(args, "--fix") {
		c.ensureShims()
		c.printf("\n(Reparación aplicada: shims y PATH regenerados.)\n")
	}
	c.printf("  home:    %s\n", c.paths.Root)
	c.printf("  bin:     %s\n", c.paths.Bin)
	if c.remote {
		c.printf("  daemon:  running (local API)\n")
	} else {
		c.printf("  daemon:  not running (offline view)\n")
	}

	var st engine.Status
	if err := c.call(daemon.ActionStatus, nil, &st); err != nil {
		c.eprintf("lair: no se pudo leer el estado: %v\n", err)
	}

	missing, onPath := shimStatus(c.paths.Bin)
	c.printf("\nPATH:\n")
	if onPath {
		c.printf("  OK: %s ya está en PATH\n", c.paths.Bin)
	} else {
		c.printf("  FALTA: %s no está en PATH\n", c.paths.Bin)
	}
	c.printf("\nShims:\n")
	if len(missing) == 0 {
		c.printf("  OK: los %d shims están presentes\n", len(shims.Tools))
	} else {
		c.printf("  FALTAN %d: %s\n", len(missing), strings.Join(missing, ", "))
		c.printf("  Solución: lair doctor --fix (o ejecuta cualquier 'lair ... install')\n")
	}

	c.printf("\nVersiones instaladas:\n")
	if len(st.PHP) == 0 {
		c.printf("  PHP:  (ninguna)\n")
	} else {
		names := make([]string, 0, len(st.PHP))
		for _, p := range st.PHP {
			names = append(names, p.Minor)
		}
		c.printf("  PHP:  %s\n", strings.Join(names, ", "))
	}
	if len(st.Node) == 0 {
		c.printf("  Node: (ninguna)\n")
	} else {
		c.printf("  Node: %s (default: %s)\n", strings.Join(st.Node, ", "), st.Defaults["node"])
	}

	c.printf("\nDNS y puertos web:\n")
	defaults := config.DefaultWeb()
	domain := st.Web.Domain
	if domain == "" {
		domain = defaults.Domain
	}
	ips, dnsErr := platform.ResolveHost("lair." + domain)
	setup := filepath.Join(c.paths.Root, "setup-privileged.sh")
	if advice := platform.DNSAdvice(setup, domain, ips, dnsErr); advice != "" {
		c.printf("  FALTA: %s\n", advice)
	} else {
		c.printf("  OK: lair.%s -> 127.0.0.1\n", domain)
	}
	bind := st.Web.Bind
	if bind == "" {
		bind = defaults.Bind
	}
	httpPort, httpsPort := st.Web.HTTPPort, st.Web.HTTPSPort
	if httpPort == 0 {
		httpPort = defaults.HTTPPort
	}
	if httpsPort == 0 {
		httpsPort = defaults.HTTPSPort
	}
	for _, port := range []int{httpPort, httpsPort} {
		// When a daemon is running and reports no web error, the configured
		// ports are held by Lair itself; probing them would always report a
		// false "address in use".
		if c.remote && st.Web.Error == "" {
			c.printf("  OK: %s:%d lo está sirviendo el daemon\n", bind, port)
			continue
		}
		if err := platform.CheckWebPort(port, bind); err != nil {
			c.printf("  FALTA: %v\n", err)
			c.printf("  Solución: sudo setcap cap_net_bind_service=+ep %s o usa Configuración de Lair en la pestaña Sitios con puertos altos\n",
				filepath.Join(c.paths.Bin, "lair"))
		} else {
			c.printf("  OK: puerto %d disponible en %s\n", port, bind)
		}
	}
	c.printf("  Nota: 80/443 requieren CAP_NET_BIND_SERVICE; usa Configuración de Lair en la pestaña Sitios con puertos altos o sudo setcap cap_net_bind_service=+ep %s\n",
		filepath.Join(c.paths.Bin, "lair"))

	if !onPath || len(missing) > 0 {
		c.printf("\nPara usar node/npm en esta terminal sin abrir una nueva:\n")
		c.printf("  export PATH=\"%s:$PATH\"\n", c.paths.Bin)
	}
	return nil
}

// capitalize uppercases only the first rune; strings.Title is deprecated and
// also uppercases letters after punctuation, which is not wanted here.
func capitalize(s string) string {
	if s == "" {
		return s
	}
	r := []rune(s)
	r[0] = []rune(strings.ToUpper(string(r[0])))[0]
	return string(r)
}

func hasFlag(args []string, flag string) bool {
	for _, a := range args {
		if a == flag {
			return true
		}
	}
	return false
}
