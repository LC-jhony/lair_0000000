package cli

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/x/term"
	"github.com/lair-sh/lair/internal/daemon"
	"github.com/lair-sh/lair/internal/project"
)

// projectFlag reads --name value (and --name=value) from an argument list.
func projectFlag(args []string, name string) string {
	prefix := name + "="
	for i, arg := range args {
		if strings.HasPrefix(arg, prefix) {
			return strings.TrimPrefix(arg, prefix)
		}
		if arg == name && i+1 < len(args) {
			return args[i+1]
		}
	}
	return ""
}

// projectValueFlags lists options whose value is the following argument. The
// name parser must skip those values: `lair new --database mariadb blog` has
// `blog` as the project name, not `mariadb`.
func projectValueFlags() map[string]bool {
	return map[string]bool{
		"--parent": true, "--database": true, "--db-name": true, "--db-user": true,
		"--db-password": true, "--db-host": true, "--db-port": true,
		"--stack": true, "--testing": true, "--node-manager": true,
	}
}

func projectNameArg(args []string) string {
	values := projectValueFlags()
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if strings.HasPrefix(arg, "-") {
			flag := strings.SplitN(arg, "=", 2)[0]
			if values[flag] && !strings.Contains(arg, "=") && i+1 < len(args) {
				i++ // the value is not the project name
			}
			continue
		}
		return arg
	}
	return ""
}

func (c *ctx) project(args []string) error {
	if len(args) == 0 || isHelp(args[0]) {
		c.projectUsage()
		return nil
	}
	switch args[0] {
	case "info":
		var out daemon.ProjectResult
		if err := c.call(daemon.ActionProject, daemon.ProjectRequest{Op: "info"}, &out); err != nil {
			return err
		}
		info := out.Info
		c.printf("Instalador Laravel: %s\n", yesNo(info.LaravelInstalled))
		if info.LaravelVersion != "" {
			c.printf("  %s\n", strings.TrimSpace(info.LaravelVersion))
		}
		if info.Hint != "" {
			c.printf("  %s\n", info.Hint)
		}
		c.printf("PHP: %s  Composer: %s  Node (nvm): %s\n",
			orNone(info.PHP), orDash(info.Composer), orNone(info.Node))
		c.printf("PDO: %s\n", orNone(info.PDO))
		c.printf("Gestores Node: %s\n", orNone(info.NodeManagers))
		c.printf("Bases de datos: %s\n", orNone(info.Databases))
		if info.DefaultDatabase != "" {
			c.printf("Motor configurado: %s\n", info.DefaultDatabase)
		}
		c.printf("Carpeta de proyectos: %s\n", orDash(info.ProjectsDir))
		return nil
	case "sync":
		var out daemon.ProjectResult
		if err := c.call(daemon.ActionProject, daemon.ProjectRequest{Op: "sync"}, &out); err != nil {
			return fmt.Errorf("project sync: %w", err)
		}
		c.printf("Proyectos sincronizados.\n")
		return nil
	case "dir":
		if hasHelp(args[1:]) {
			c.printf("usage: lair project dir [<ruta>]\n")
			return nil
		}
		if len(args) < 2 {
			var out daemon.ProjectResult
			if err := c.call(daemon.ActionProject, daemon.ProjectRequest{Op: "info"}, &out); err != nil {
				return err
			}
			c.printf("%s\n", orDash(out.Info.ProjectsDir))
			return nil
		}
		var out daemon.ProjectResult
		if err := c.call(daemon.ActionProject, daemon.ProjectRequest{Op: "dirs-default", Dir: args[1]}, &out); err != nil {
			return fmt.Errorf("project dir: %w", err)
		}
		c.printf("Carpeta de proyectos: %s\n", out.Path)
		return nil
	case "dirs":
		return c.projectDirs(args[1:])
	case "migrate-dirs":
		if hasHelp(args[1:]) {
			c.printf("usage: lair migrate-dirs [<desde> <hacia>]\n")
			return nil
		}
		return c.projectMigrateDirs(args[1:])
	case "import-valet":
		if hasHelp(args[1:]) {
			c.printf("usage: lair import-valet\n")
			return nil
		}
		return c.projectImportValet()
	case "new", "create":
		return c.projectNew(args[1:])
	default:
		return fmt.Errorf("unknown project subcommand %q", args[0])
	}
}

// projectDirs manages the list of folders Lair scans for projects. All
// operations go through the single `project` action so the CLI, the TUI and
// the desktop app share the exact same wire contract.
func (c *ctx) projectDirs(args []string) error {
	if hasHelp(args) {
		c.printf("usage: lair project dirs [list|add <ruta>|remove <ruta>|default <ruta>]\n")
		return nil
	}
	op := "dirs-list"
	dir := ""
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		op, args = args[0], args[1:]
	}
	switch op {
	case "list":
		op = "dirs-list"
	case "add", "remove", "default":
		if len(args) == 0 {
			return fmt.Errorf("usage: lair project dirs %s <ruta>", op)
		}
		dir = args[0]
		op = "dirs-" + op
	}
	var out daemon.ProjectResult
	if err := c.call(daemon.ActionProject, daemon.ProjectRequest{Op: op, Dir: dir}, &out); err != nil {
		return fmt.Errorf("project dirs %s: %w", op, err)
	}
	if len(out.Dirs) == 0 {
		c.printf("(sin carpetas)\n")
		return nil
	}
	for _, d := range out.Dirs {
		c.printf("%s\n", d)
	}
	return nil
}

// projectMigrateDirs moves a legacy projects folder and rewrites the tracked
// paths. It never deletes the destination if it already exists.
func (c *ctx) projectMigrateDirs(args []string) error {
	if len(args) != 0 && len(args) != 2 {
		return fmt.Errorf("usage: lair migrate-dirs [<desde> <hacia>]")
	}
	var out daemon.ProjectResult
	req := daemon.ProjectRequest{Op: "migrate-dirs"}
	if len(args) == 2 {
		req.From, req.To = args[0], args[1]
	}
	if err := c.call(daemon.ActionProject, req, &out); err != nil {
		return err
	}
	if out.Output != "" {
		c.printf("%s\n", out.Output)
	}
	c.printf("Destino: %s\n", out.Path)
	return nil
}

// projectImportValet reads ~/.valet without invoking or changing Valet.
func (c *ctx) projectImportValet() error {
	var out daemon.ProjectResult
	if err := c.call(daemon.ActionProject, daemon.ProjectRequest{Op: "import-valet"}, &out); err != nil {
		return err
	}
	if out.Output != "" {
		c.printf("%s\n", out.Output)
	}
	return nil
}

// newProject is the canonical CLI entry point for creating a Laravel project.
// `lair project new` remains as a compatibility alias.
func (c *ctx) newProject(args []string) error {
	if len(args) == 0 || hasHelp(args) {
		c.printf("usage: lair new <name> [opciones]\n")
		c.printf("Alias de `lair project new`. Opciones:\n")
		c.projectUsage()
		return nil
	}
	return c.projectNew(args)
}

func (c *ctx) projectNew(args []string) error {
	if hasHelp(args) {
		c.printf("usage: lair project new <name> [options]\n")
		c.projectUsage()
		return nil
	}
	name := projectNameArg(args)
	if name == "" {
		return fmt.Errorf("usage: lair project new <name> [options]")
	}
	// An empty parent is resolved by the daemon to the configured projects
	// folder (default ~/Lair). Never fall back to the current directory or
	// LAIR_HOME here.
	parent := projectFlag(args, "--parent")
	driver := projectFlag(args, "--database")
	if driver == "" {
		driver = "sqlite"
	}
	password := projectFlag(args, "--db-password")
	if password == "" {
		password = os.Getenv("LAIR_DB_PASSWORD")
	}
	if driver != "sqlite" && password == "" && isTerminal(os.Stdin) {
		c.printf("Contraseña de la base de datos: ")
		data, err := term.ReadPassword(os.Stdin.Fd())
		c.printf("\n")
		if err != nil {
			return err
		}
		password = strings.TrimSpace(string(data))
	}
	port := 0
	if rawPort := projectFlag(args, "--db-port"); rawPort != "" {
		parsed, err := strconv.Atoi(rawPort)
		if err != nil || parsed < 1 || parsed > 65535 {
			return fmt.Errorf("invalid --db-port %q", rawPort)
		}
		port = parsed
	}
	req := daemon.ProjectRequest{
		Op: "create",
		CreateRequest: project.CreateRequest{
			Name: name, Parent: parent,
			Secure:       hasFlag(args, "--secure"),
			Stack:        orDefault(projectFlag(args, "--stack"), "blade"),
			Auth:         !hasFlag(args, "--no-auth"),
			Testing:      orDefault(projectFlag(args, "--testing"), "pest"),
			Boost:        hasFlag(args, "--boost"),
			NodeManager:  orDefault(projectFlag(args, "--node-manager"), "npm"),
			Git:          hasFlag(args, "--git"),
			Database:     driver,
			DBName:       orDefault(projectFlag(args, "--db-name"), strings.ReplaceAll(name, "-", "_")+"_dev"),
			DBUser:       orDefault(projectFlag(args, "--db-user"), defaultDBUser(driver)),
			DBPassword:   password,
			DBHost:       orDefault(projectFlag(args, "--db-host"), "127.0.0.1"),
			DBPort:       port,
			CreateDB:     !hasFlag(args, "--no-create-db"),
			StartService: !hasFlag(args, "--no-start"),
		},
	}
	var result daemon.ProjectResult
	if err := c.call(daemon.ActionProject, req, &result); err != nil {
		return err
	}
	c.printf("Creando %s… (trabajo %s)\n", name, result.ID)
	if err := c.waitProjectJob(result.ID); err != nil {
		return err
	}
	c.printf("Proyecto listo: %s -> %s\n", result.Site, result.Path)
	c.printf("Registro: lair logs %s-install.log\n", result.ID)
	return nil
}

// waitProjectJob polls without the installer-specific wording of waitJob.
func (c *ctx) waitProjectJob(id string) error {
	ctx, cancel := context.WithTimeout(context.Background(), jobWaitTimeout)
	defer cancel()
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()
	for {
		var st struct {
			Done     bool   `json:"done"`
			Error    string `json:"error"`
			Canceled bool   `json:"canceled"`
		}
		if err := c.callContext(ctx, daemon.ActionJob, daemon.JobRequest{ID: id}, &st); err != nil {
			return err
		}
		if st.Done {
			if st.Error != "" {
				return fmt.Errorf("%s", st.Error)
			}
			if st.Canceled {
				return fmt.Errorf("creación cancelada")
			}
			return nil
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("waiting for %s: %w", id, ctx.Err())
		case <-ticker.C:
		}
	}
}

func (c *ctx) projectUsage() {
	c.printf("usage: lair project info\n")
	c.printf("       lair project dir [RUTA]       Muestra o fija la carpeta por defecto (~/Lair)\n")
	c.printf("       lair project dirs [list|add RUTA|remove RUTA|default RUTA]\n")
	c.printf("       lair project migrate-dirs [DESDE HACIA] Mueve ~/lair a ~/Lair (o un par explícito)\n")
	c.printf("       lair project import-valet    Importa solo lectura ~/.valet/config.json y ~/.valet/Sites\n")
	c.printf("       lair project sync             Vuelve a escanear las carpetas de proyectos\n")
	c.printf("       lair project new <name> [--parent DIR] [--secure] [--stack blade|react|vue|svelte|livewire]\n")
	c.printf("         [--no-auth] [--testing pest|phpunit] [--boost] [--node-manager npm|pnpm|bun|yarn|none]\n")
	c.printf("         [--git] [--database sqlite|mysql|mariadb|pgsql|sqlsrv] [--db-name NAME] [--db-user USER]\n")
	c.printf("         [--db-password PASS] [--db-host HOST] [--db-port N] [--no-create-db] [--no-start]\n")
	c.printf("La contraseña se pide sin eco si no se da --db-password (o LAIR_DB_PASSWORD).\n")
}

func yesNo(value bool) string {
	if value {
		return "sí"
	}
	return "no"
}

func orNone(values []string) string {
	if len(values) == 0 {
		return "—"
	}
	return strings.Join(values, ", ")
}

func orDefault(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

// defaultDBUser follows the user created by Lair's service initialisation:
// MariaDB/MySQL use a passwordless root, PostgreSQL's initdb creates postgres.
func defaultDBUser(driver string) string {
	if driver == "pgsql" {
		return "postgres"
	}
	return "root"
}
