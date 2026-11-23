package project

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/lair-sh/lair/internal/app"
	"github.com/lair-sh/lair/internal/composer"
	"github.com/lair-sh/lair/internal/nvm"
	"github.com/lair-sh/lair/internal/php"
)

// Invocation is the resolved command of a Lair shim, mirrored here so the
// project package does not need to import the engine.
type Invocation struct {
	Path string
	Args []string
	Env  []string
}

// ToolResolver resolves php/composer/node through Lair's own shim logic.
type ToolResolver interface {
	ResolveTool(tool, dir string) (Invocation, error)
}

// PathWithShims prepends Lair's shims to PATH so every php/composer/node call
// made by the Laravel installer goes through Lair.
func PathWithShims(paths app.Paths, extra ...string) string {
	parts := append([]string{paths.Bin}, extra...)
	return strings.Join(parts, string(os.PathListSeparator)) + string(os.PathListSeparator) + os.Getenv("PATH")
}

// InstallCommand is the exact Composer command Lair runs to provide the
// Laravel installer. It is also shown in logs, errors and the wizard hint so
// the user can reproduce it manually if needed.
const InstallCommand = "composer global require laravel/installer"

// EnsureInstaller installs laravel/installer in Lair's isolated Composer home
// when it is missing. It returns the path to the executable.
func EnsureInstaller(ctx context.Context, resolver ToolResolver, paths app.Paths, log func(string)) (string, error) {
	bin := InstallerBin(paths)
	if st, err := os.Stat(bin); err == nil && !st.IsDir() {
		return bin, nil
	}
	if os.Getenv("LAIR_LARAVEL_BIN") != "" {
		return "", fmt.Errorf("LAIR_LARAVEL_BIN apunta a %s, que no existe", bin)
	}
	inv, err := resolver.ResolveTool("composer", "")
	if err != nil {
		return "", err
	}
	home := InstallerHome(paths)
	if err := os.MkdirAll(home, 0o755); err != nil {
		return "", err
	}
	argv := append(append([]string{}, inv.Args...),
		"global", "require", "laravel/installer", "--no-interaction", "--no-progress")
	cmd := exec.CommandContext(ctx, inv.Path, argv...)
	cmd.Dir = home
	cmd.Env = CleanEnv(os.Environ(),
		append(inv.Env,
			"COMPOSER_HOME="+home,
			"COMPOSER_NO_INTERACTION=1",
			"COMPOSER_ALLOW_SUPERUSER=0",
			"PATH="+PathWithShims(paths),
		)...,
	)
	setProcessGroup(cmd)
	out, errOut := newLogWriter(log, ""), newLogWriter(log, "")
	defer func() { out.Flush(); errOut.Flush() }()
	cmd.Stdout, cmd.Stderr = out, errOut
	log("Instalando laravel/installer en " + home + "…")
	log("Comando: " + InstallCommand)
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("%s falló: %w", InstallCommand, err)
	}
	if st, err := os.Stat(bin); err != nil || st.IsDir() {
		return "", fmt.Errorf("laravel/installer se instaló pero no se encontró %s", bin)
	}
	return bin, nil
}

// RunLaravel executes `laravel new` with deterministic flags and DB_* in the
// environment. Output is streamed to the job log with the password redacted.
func RunLaravel(ctx context.Context, paths app.Paths, bin, parent string, r CreateRequest, log func(string)) error {
	cmd := exec.CommandContext(ctx, bin, LaravelArgs(r)...)
	cmd.Dir = parent
	cmd.Env = CleanEnv(os.Environ(),
		append(EnvValues(r, parent),
			"COMPOSER_HOME="+InstallerHome(paths),
			"COMPOSER_NO_INTERACTION=1",
			"COMPOSER_ALLOW_SUPERUSER=0",
			"PATH="+PathWithShims(paths, filepath.Join(InstallerHome(paths), "vendor", "bin")),
		)...,
	)
	out, errOut := newLogWriter(log, r.DBPassword), newLogWriter(log, r.DBPassword)
	defer func() { out.Flush(); errOut.Flush() }()
	cmd.Stdout, cmd.Stderr = out, errOut
	setProcessGroup(cmd)
	done := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			killProcessGroup(cmd)
		case <-done:
		}
	}()
	defer close(done)
	log(CommandLine(bin, r))
	return cmd.Run()
}

// Detect gathers the capabilities shown by the wizard. Failures are reported
// as empty lists rather than errors: the UI stays usable on a minimal machine.
func Detect(resolver ToolResolver, paths app.Paths) Info {
	out := Info{
		PHP: []string{}, Node: []string{}, PDO: []string{},
		NodeManagers: []string{}, Databases: []string{"sqlite"},
	}
	bin := InstallerBin(paths)
	if st, err := os.Stat(bin); err == nil && !st.IsDir() {
		out.LaravelInstalled = true
		out.LaravelVersion = strings.TrimSpace(commandOutput(bin, "--version"))
		out.InstallerFlags = helpFlags(commandOutput(bin, "new", "--help"))
	} else {
		out.Hint = "laravel/installer no está instalado; se instalará con `" + InstallCommand + "` al crear el primer proyecto"
	}
	out.Composer = composer.InstalledVersion(paths.Tools)
	installedPHP, _ := php.Installed(paths)
	for _, info := range installedPHP {
		out.PHP = append(out.PHP, info.Minor)
	}
	if versions := nvm.InstalledVersions(); len(versions) > 0 {
		out.Node = versions
	}
	out.PDO = pdoDrivers(resolver)
	for _, manager := range []string{"npm", "pnpm", "bun", "yarn"} {
		if _, err := exec.LookPath(manager); err == nil {
			out.NodeManagers = append(out.NodeManagers, manager)
		}
	}
	for _, driver := range []string{"mysql", "mariadb", "pgsql"} {
		if directoryHasVersion(paths, ServiceID(driver)) {
			out.Databases = append(out.Databases, driver)
		}
	}
	if contains(out.PDO, "sqlsrv") {
		out.Databases = append(out.Databases, "sqlsrv")
	}
	return out
}

func commandOutput(bin string, args ...string) string {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Env = CleanEnv(os.Environ(), "COMPOSER_NO_INTERACTION=1")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return ""
	}
	return string(out)
}

func helpFlags(text string) []string {
	flags := []string{}
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "--") {
			continue
		}
		flag := strings.TrimSuffix(strings.Fields(line)[0], ",")
		if !contains(flags, flag) {
			flags = append(flags, flag)
		}
	}
	return flags
}

func pdoDrivers(resolver ToolResolver) []string {
	if resolver == nil {
		return []string{}
	}
	inv, err := resolver.ResolveTool("php", "")
	if err != nil {
		return []string{}
	}
	argv := append(append([]string{}, inv.Args...), "-r", `echo implode("\n", PDO::getAvailableDrivers());`)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, inv.Path, argv...)
	cmd.Env = inv.Env
	out, err := cmd.Output()
	if err != nil {
		return []string{}
	}
	drivers := []string{}
	for _, line := range strings.Split(string(out), "\n") {
		if line = strings.TrimSpace(line); line != "" {
			drivers = append(drivers, line)
		}
	}
	return drivers
}

func directoryHasVersion(paths app.Paths, id string) bool {
	entries, err := os.ReadDir(filepath.Join(paths.Services, id))
	if err != nil {
		return false
	}
	for _, entry := range entries {
		if entry.IsDir() {
			return true
		}
	}
	return false
}

// ServiceUp ensures the database service is supervised when the wizard asked
// for it. A no-op for drivers Lair does not manage.
func ServiceUp(start bool, id string, running func(string) bool, startFn func(string) error) error {
	if !start || id == "" || running(id) {
		return nil
	}
	return startFn(id)
}

// CreateManagedDatabase delegates to the engine's existing create-db helper.
func CreateManagedDatabase(driver, name string, create func(string, string) (string, error)) (string, error) {
	id := ServiceID(driver)
	if id == "" {
		return "", fmt.Errorf("SQL Server no es gestionado por Lair; crea la base de datos manualmente antes de continuar")
	}
	return create(id, name)
}

// EnsureSQLiteFile makes sure Laravel's SQLite database file exists.
func EnsureSQLiteFile(parent, name string) error {
	path := filepath.Join(parent, name, "database", "database.sqlite")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	return f.Close()
}

// logWriter turns each line of command output into a log entry, redacting the
// database password before it is written.
type logWriter struct {
	mu       sync.Mutex
	pending  string
	log      func(string)
	password string
}

func newLogWriter(log func(string), password string) *logWriter {
	return &logWriter{log: log, password: password}
}

func (w *logWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.pending += string(p)
	for {
		idx := strings.IndexAny(w.pending, "\r\n")
		if idx < 0 {
			break
		}
		line := strings.TrimRight(w.pending[:idx], "\r")
		w.pending = w.pending[idx+1:]
		if line != "" {
			w.log(Redact(line, w.password))
		}
	}
	return len(p), nil
}

// Flush emits a trailing partial line.
func (w *logWriter) Flush() {
	w.mu.Lock()
	defer w.mu.Unlock()
	if line := strings.TrimSpace(w.pending); line != "" {
		w.log(Redact(line, w.password))
	}
	w.pending = ""
}

var _ io.Writer = (*logWriter)(nil)
