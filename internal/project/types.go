// Package project holds the pure rules used to create Laravel projects with
// the official installer. It has no dependency on the daemon or the desktop UI:
// the engine supplies the runtime operations and both interfaces share these
// types and validations.
package project

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/lair-sh/lair/internal/app"
	"github.com/lair-sh/lair/internal/config"
)

// InstallerEnv is the isolated Composer home used by `laravel/installer`. It
// never touches the user's global Composer installation.
func InstallerHome(paths app.Paths) string { return filepath.Join(paths.Tools, "laravel-global") }

// InstallerBin is the installer executable inside the isolated Composer home.
// LAIR_LARAVEL_BIN overrides it (used by tests and custom installations).
func InstallerBin(paths app.Paths) string {
	if custom := strings.TrimSpace(os.Getenv("LAIR_LARAVEL_BIN")); custom != "" {
		return custom
	}
	return filepath.Join(InstallerHome(paths), "vendor", "bin", "laravel")
}

// Supported choices exposed by the wizard.
var (
	Stacks       = []string{"blade", "react", "vue", "svelte", "livewire"}
	Testings     = []string{"pest", "phpunit"}
	NodeManagers = []string{"npm", "pnpm", "bun", "yarn", "none"}
	Databases    = []string{"sqlite", "mysql", "mariadb", "pgsql", "sqlsrv"}
)

// Info describes what the current machine can offer to the wizard.
type Info struct {
	LaravelInstalled bool     `json:"laravel_installed"`
	LaravelVersion   string   `json:"laravel_version,omitempty"`
	InstallerFlags   []string `json:"installer_flags,omitempty"`
	Composer         string   `json:"composer,omitempty"`
	PHP              []string `json:"php"`
	Node             []string `json:"node"`
	PDO              []string `json:"pdo_drivers"`
	NodeManagers     []string `json:"node_managers"`
	Databases        []string `json:"databases"`
	DefaultDatabase  string   `json:"default_database,omitempty"`
	ProjectsDir      string   `json:"projects_dir,omitempty"`
	Hint             string   `json:"hint,omitempty"`
}

// ValidName reports whether name is a safe Laravel project/site name.
func ValidName(name string) bool {
	return projectNameRE.MatchString(name)
}

// DefaultParent is the folder used when neither the client nor the config
// provides one. It is ~/Lair, always under the user's home and never under
// LAIR_HOME.
func DefaultParent() string { return config.DefaultProjectsDir("") }

// CreateRequest is the payload of the `project create` daemon action. The
// password travels in this payload over the local token-protected API, but it
// is never written to a log or included in a command line.
type CreateRequest struct {
	Name         string `json:"name"`
	Parent       string `json:"parent"`
	Secure       bool   `json:"secure"`
	Stack        string `json:"stack"`
	Auth         bool   `json:"auth"`
	Testing      string `json:"testing"`
	Boost        bool   `json:"boost"`
	NodeManager  string `json:"node_manager"`
	Git          bool   `json:"git"`
	Database     string `json:"database"`
	DBName       string `json:"db_name,omitempty"`
	DBUser       string `json:"db_user,omitempty"`
	DBPassword   string `json:"db_password,omitempty"`
	DBHost       string `json:"db_host,omitempty"`
	DBPort       int    `json:"db_port,omitempty"`
	CreateDB     bool   `json:"create_db"`
	StartService bool   `json:"start_service"`
}

// Result extends installer.Status fields so the existing job polling works
// unchanged, while reporting the site that was registered.
// ControlRequest is the payload of the single `project` daemon action. Dir is
// used by set-dir/dirs-add/dirs-remove; From and To by migrate-dirs. The
// fields are flat in JSON so the wire format stays `{op, dir, from, to, ...}`.
type ControlRequest struct {
	Op   string `json:"op,omitempty"`
	Dir  string `json:"dir,omitempty"`
	Path string `json:"path,omitempty"` // alias of dir, used by the documented payloads
	From string `json:"from,omitempty"`
	To   string `json:"to,omitempty"`
	CreateRequest
}

type Result struct {
	ID       string   `json:"id,omitempty"`
	Phase    string   `json:"phase,omitempty"`
	Done     bool     `json:"done,omitempty"`
	Error    string   `json:"error,omitempty"`
	Canceled bool     `json:"canceled,omitempty"`
	Site     string   `json:"site,omitempty"`
	Path     string   `json:"path,omitempty"`
	Info     Info     `json:"info,omitempty"`
	Dirs     []string `json:"dirs,omitempty"`
	Output   string   `json:"output,omitempty"`
}

var (
	projectNameRE = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?$`)
	dbNameRE      = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
)

// Normalize fills optional fields and validates the request before any
// filesystem or network operation is attempted.
func (r *CreateRequest) Normalize() (target string, err error) {
	r.Name = strings.ToLower(strings.TrimSpace(r.Name))
	if !projectNameRE.MatchString(r.Name) {
		return "", fmt.Errorf("nombre de proyecto no válido: usa letras minúsculas, números y guiones (máx. 63)")
	}
	parent := strings.TrimSpace(r.Parent)
	if parent == "" {
		parent = DefaultParent()
		if err := os.MkdirAll(parent, 0o755); err != nil {
			return "", fmt.Errorf("no se pudo crear la carpeta de proyectos %s: %w", parent, err)
		}
	}
	if strings.HasPrefix(parent, "~") {
		if home, e := os.UserHomeDir(); e == nil {
			parent = filepath.Join(home, strings.TrimPrefix(parent, "~"))
		}
	}
	parent, err = filepath.Abs(parent)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(parent)
	if err != nil || !info.IsDir() {
		return "", fmt.Errorf("la carpeta de destino no existe: %s", parent)
	}
	r.Parent = parent

	if !contains(Stacks, r.Stack) {
		return "", fmt.Errorf("interfaz no válida: %s", r.Stack)
	}
	if !contains(Testings, r.Testing) {
		return "", fmt.Errorf("framework de pruebas no válido: %s", r.Testing)
	}
	if !contains(NodeManagers, r.NodeManager) {
		return "", fmt.Errorf("gestor de Node no válido: %s", r.NodeManager)
	}
	if !contains(Databases, r.Database) {
		return "", fmt.Errorf("base de datos no válida: %s", r.Database)
	}

	target = filepath.Join(parent, r.Name)
	if _, err := os.Lstat(target); err == nil {
		return "", fmt.Errorf("la carpeta %s ya existe", target)
	} else if !os.IsNotExist(err) {
		return "", err
	}

	if r.Database != "sqlite" {
		r.DBName = strings.TrimSpace(r.DBName)
		if !dbNameRE.MatchString(r.DBName) {
			return "", fmt.Errorf("nombre de base de datos no válido: usa letras, números y guiones bajos")
		}
		r.DBUser = strings.TrimSpace(r.DBUser)
		if r.DBUser == "" {
			return "", fmt.Errorf("indica el usuario de la base de datos")
		}
		// An empty password is valid: Lair's local MariaDB/MySQL root has no
		// password by default. Do not require one here.
		if strings.TrimSpace(r.DBHost) == "" {
			r.DBHost = "127.0.0.1"
		}
		if r.DBPort <= 0 || r.DBPort > 65535 {
			r.DBPort = defaultPort(r.Database)
		}
	}
	return target, nil
}

func contains(list []string, value string) bool {
	for _, item := range list {
		if item == value {
			return true
		}
	}
	return false
}

func defaultPort(driver string) int {
	switch driver {
	case "mysql", "mariadb":
		return 3306
	case "pgsql":
		return 5432
	case "sqlsrv":
		return 1433
	default:
		return 0
	}
}

// ServiceID maps a Laravel driver to the Lair service that owns it. SQL Server
// is not managed by Lair, so it returns an empty id.
func ServiceID(driver string) string {
	switch driver {
	case "mysql":
		return "mysql"
	case "mariadb":
		return "mariadb"
	case "pgsql":
		return "postgres"
	default:
		return ""
	}
}

// URLScheme returns the scheme used for the new site's APP_URL.
func URLScheme(secure, httpsEnabled bool) string {
	if secure && httpsEnabled {
		return "https"
	}
	return "http"
}
