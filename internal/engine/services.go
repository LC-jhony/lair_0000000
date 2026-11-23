package engine

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/lair-sh/lair/internal/app"
	"github.com/lair-sh/lair/internal/config"
	"github.com/lair-sh/lair/internal/installer"
	"github.com/lair-sh/lair/internal/services"
	"github.com/lair-sh/lair/internal/supervisor"
	"github.com/lair-sh/lair/internal/version"
)

// ServiceView is the status of one service id.
type ServiceView struct {
	ID        string   `json:"id"`
	Active    string   `json:"active"`
	Port      int      `json:"port"`
	Installed []string `json:"installed"`
	Running   bool     `json:"running"`
	PID       int      `json:"pid"`
	LastError string   `json:"last_error,omitempty"`
	// Availability is relative to the machine running the daemon: a service
	// with no signed artifact for the current platform can still be listed so
	// the UI can explain why it cannot be installed here.
	Available         bool   `json:"available"`
	Platform          string `json:"platform"`
	UnavailableReason string `json:"unavailable_reason,omitempty"`
}

func servicesList() []string {
	// sqlite3 is a catalog CLI tool, shown alongside services so SQLite is
	// discoverable from the same tab, but it has no supervisor or data dir.
	return append(services.IDs(), SQLiteToolID)
}

func (e *Engine) installedServiceVersions(id string) []string {
	dir := filepath.Join(e.Paths.Services, id)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return []string{}
	}
	out := []string{}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name := entry.Name()
		idx := strings.LastIndex(name, "-build")
		if idx <= 0 {
			continue
		}
		ver := name[:idx]
		if _, err := version.Parse(ver); err == nil {
			out = append(out, ver)
		}
	}
	version.SortNewest(out)
	return out
}

// serviceAvailability reports whether the signed catalog has a build for the
// current platform. The reason code lets the UI explain the absence instead
// of silently hiding the service.
func (e *Engine) serviceAvailability(id string) (bool, string) {
	if ok, reason := services.Availability(id, app.Platform()); !ok {
		return false, reason
	}
	versions, err := e.available(id)
	if err != nil || len(versions) == 0 {
		return false, "no-build"
	}
	return true, ""
}

func (e *Engine) serviceView(c *config.Config, id string) ServiceView {
	if id == SQLiteToolID {
		available, reason := e.serviceAvailability(id)
		return ServiceView{
			ID: id, Active: e.installedToolVersion(id),
			Installed: e.installedToolVersions(id),
			Available: available, UnavailableReason: reason,
			Platform: app.Platform(),
		}
	}
	port := c.Ports[id]
	if port == 0 {
		port = services.DefaultPort(id)
	}
	v := ServiceView{
		ID: id, Active: c.Defaults[id], Port: port,
		Installed: e.installedServiceVersions(id),
		Available: true, Platform: app.Platform(),
	}
	v.Available, v.UnavailableReason = e.serviceAvailability(id)
	if st, ok := e.Supervisors.State(id); ok {
		v.Running, v.PID, v.LastError = st.Running, st.PID, st.LastError
	}
	return v
}

// expectedFor returns the executables that must exist after extraction.
func expectedFor(id string) []string {
	switch id {
	case "postgres":
		return []string{"bin/postgres", "bin/initdb", "bin/pg_ctl", "bin/psql"}
	case "mysql":
		return []string{"bin/mysqld"}
	case "mariadb":
		return []string{"bin/mariadbd", "scripts/mariadb-install-db"}
	case "mongodb":
		return []string{"bin/mongod"}
	case "redis":
		return []string{"bin/redis-server"}
	case "valkey":
		return []string{"bin/valkey-server"}
	case "meilisearch", "typesense", "rustfs":
		name := map[string]string{
			"meilisearch": "meilisearch", "typesense": "typesense-server", "rustfs": "rustfs",
		}[id]
		if strings.HasPrefix(app.Platform(), "windows-") {
			name += ".exe"
		}
		return []string{"bin/" + name}
	default:
		return nil
	}
}

// InstallService downloads and initialises a service version. Installing a new
// version never changes the active one unless it is the first installation.
func (e *Engine) InstallService(ctx context.Context, id, request string, port int) (*installer.Job, error) {
	if id == SQLiteToolID {
		return e.InstallTool(ctx, id, request)
	}
	if !serviceKnown(id) {
		return nil, fmt.Errorf("unknown service %q", id)
	}
	available, err := e.available(id)
	if err != nil {
		return nil, err
	}
	picked, err := version.ExactOrLatest(request, available)
	if err != nil {
		return nil, err
	}
	art, err := e.artifact(id, picked)
	if err != nil {
		return nil, err
	}
	// PostgreSQL owns its conventional port; changing it after install would
	// break the documented Laravel defaults, so the installer never offers it.
	if id == "postgres" {
		port = services.DefaultPort(id)
	} else if port == 0 {
		port = services.DefaultPort(id)
	}
	// One active version per port: reject a port already assigned to a
	// different service in config, before touching the network or disk.
	if c, err := e.Config.View(); err == nil {
		for otherID, otherPort := range c.Ports {
			if otherID != id && otherPort == port {
				return nil, fmt.Errorf("port %d is already assigned to %s; choose another with --port", port, otherID)
			}
		}
	}
	for _, aux := range services.AuxPorts(id, port) {
		if aux <= 0 || aux > 65535 {
			return nil, fmt.Errorf("port %d makes %s auxiliary port %d invalid; choose another --port", port, id, aux)
		}
		if conflict := services.CheckPort(aux); conflict != nil {
			return nil, fmt.Errorf("%s needs auxiliary port %d: %s", id, aux, conflict.String())
		}
	}
	if conflict := services.CheckPort(port); conflict != nil {
		// Installing a newer version of a service that is already running on
		// its own port must work (that is what `services update` does). Stop
		// our supervised instance, then re-check in case the port is still
		// held by something else.
		if st, ok := e.Supervisors.State(id); ok && st.Running {
			_ = e.StopService(id)
			// A graceful shutdown releases the port asynchronously.
			for i := 0; i < 20; i++ {
				conflict = services.CheckPort(port)
				if conflict == nil {
					break
				}
				time.Sleep(100 * time.Millisecond)
			}
		}
		if conflict != nil {
			return nil, fmt.Errorf("%s", conflict.String())
		}
	}
	dest := services.InstallDir(e.Paths, id, picked, art.Build)
	job, err := e.Installer.Start(ctx, installer.Request{
		ID: id + "-" + picked, Package: id, Version: picked, Platform: app.Platform(),
		Artifact: art, Dest: dest, Expected: expectedFor(id),
		ConfigKey: id + "/" + picked,
		OnSuccess: func(dir string) error {
			return e.initService(ctx, id, picked, art.Build, port)
		},
	})
	if err != nil {
		return nil, err
	}
	return job, nil
}

// initService prepares data and records defaults. It runs inside the install
// job after the binary is in place.
func (e *Engine) initService(ctx context.Context, id, ver, build string, port int) error {
	data := services.DataPath(e.Paths, id, ver)
	// Decide BEFORE EnsureDataDir writes my.cnf: an empty datadir must be
	// initialised even though the config file now makes it non-empty.
	needInit := services.InitNeeded(id, data)
	if err := services.EnsureDataDir(e.Paths, id, data); err != nil {
		return fmt.Errorf("prepare data dir: %w", err)
	}
	if id == "mysql" || id == "mariadb" {
		auto := filepath.Join(data, "mysqld-auto.cnf")
		if err := services.ValidateConfigFile(auto); err != nil {
			return err
		}
	}
	if needInit {
		argv, workDir, err := services.InitCommand(e.Paths, id, ver, build)
		if err != nil {
			return err
		}
		if len(argv) > 0 {
			// Database init can prompt or wait forever on a broken binary.
			// Bound it and honour cancellation of the parent install job.
			initCtx, cancel := context.WithTimeout(ctx, 10*time.Minute)
			defer cancel()
			cmd := exec.CommandContext(initCtx, argv[0], argv[1:]...)
			cmd.Dir = workDir
			cmd.Env = append(os.Environ(), "HOME="+e.Paths.Root, "LC_ALL=C")
			out, err := cmd.CombinedOutput()
			if err != nil {
				return fmt.Errorf("initialise %s data: %w (%s)", id, err, strings.TrimSpace(string(out)))
			}
		}
	}
	return e.Config.Update(func(c *config.Config) error {
		if c.Defaults[id] == "" {
			c.Defaults[id] = ver
			c.Ports[id] = port
		}
		return nil
	})
}

// UseService changes the active version of an installed service. If the
// service is being supervised, it is restarted with the new spec so the old
// binary/data directory is not left running under the new default.
func (e *Engine) UseService(id, ver string) error {
	installed := e.installedServiceVersions(id)
	for _, v := range installed {
		if v == ver {
			_, supervised := e.Supervisors.State(id)
			if err := e.Config.Update(func(c *config.Config) error {
				c.Defaults[id] = ver
				return nil
			}); err != nil {
				return err
			}
			if supervised {
				spec, err := e.ServiceSpec(id)
				if err != nil {
					return err
				}
				return e.Supervisors.Restart(id, spec)
			}
			return nil
		}
	}
	return fmt.Errorf("%s %s is not installed", id, ver)
}

// ServiceSpec builds the runtime spec for the active version of a service.
func (e *Engine) ServiceSpec(id string) (services.Spec, error) {
	c, err := e.Config.View()
	if err != nil {
		return services.Spec{}, err
	}
	ver := c.Defaults[id]
	if ver == "" {
		return services.Spec{}, fmt.Errorf("%s has no active version", id)
	}
	build := c.Installed[id+"/"+ver]
	if build == "" {
		// Fall back to the build marker on disk.
		dirs, _ := os.ReadDir(filepath.Join(e.Paths.Services, id))
		for _, d := range dirs {
			if strings.HasPrefix(d.Name(), ver+"-build") {
				build = strings.TrimPrefix(d.Name(), ver+"-build")
			}
		}
	}
	return services.Build(e.Paths, id, ver, c.Ports[id], build)
}

// StartService supervises the active version.
func (e *Engine) StartService(id string) error {
	spec, err := e.ServiceSpec(id)
	if err != nil {
		return err
	}
	return e.Supervisors.Start(id, spec)
}

// RestartService restarts a supervised service, optionally with a fresh spec.
func (e *Engine) RestartService(id string) error {
	spec, err := e.ServiceSpec(id)
	if err != nil {
		return err
	}
	return e.Supervisors.Restart(id, spec)
}

// StopService stops a supervised service.
func (e *Engine) StopService(id string) error { return e.Supervisors.Stop(id) }

// UninstallService removes a version, preserving data unless deleteData is
// set. If the active version is removed, the newest remaining becomes active.
func (e *Engine) UninstallService(id, ver string, deleteData bool) error {
	if !serviceKnown(id) {
		return fmt.Errorf("unknown service %q", id)
	}
	c, err := e.Config.View()
	if err != nil {
		return err
	}
	if ver == "" {
		ver = c.Defaults[id]
	}
	installed := e.installedServiceVersions(id)
	found := false
	for _, v := range installed {
		if v == ver {
			found = true
		}
	}
	if !found {
		return fmt.Errorf("%s %s is not installed", id, ver)
	}
	// Read build from config to locate the exact directory.
	build := c.Installed[id+"/"+ver]
	if build == "" {
		entries, _ := os.ReadDir(filepath.Join(e.Paths.Services, id))
		for _, d := range entries {
			if strings.HasPrefix(d.Name(), ver+"-build") {
				build = strings.TrimPrefix(d.Name(), ver+"-build")
			}
		}
	}
	_, supervised := e.Supervisors.State(id)
	if build != "" {
		if err := os.RemoveAll(services.InstallDir(e.Paths, id, ver, build)); err != nil {
			return err
		}
	}
	if deleteData {
		if err := os.RemoveAll(services.DataPath(e.Paths, id, ver)); err != nil {
			return err
		}
	}
	if err := e.Config.Update(func(c *config.Config) error {
		delete(c.Installed, id+"/"+ver)
		if c.Defaults[id] == ver {
			c.Defaults[id] = ""
			remaining := e.installedServiceVersions(id)
			if len(remaining) > 0 {
				c.Defaults[id] = remaining[0]
			}
		}
		return nil
	}); err != nil {
		return err
	}
	// Never leave a supervised process running from a directory we just
	// deleted: restart it on the remaining version, or stop it entirely.
	if supervised {
		active, err := e.Config.View()
		if err != nil || active.Defaults[id] == "" {
			return e.Supervisors.Stop(id)
		}
		spec, err := e.ServiceSpec(id)
		if err != nil {
			return err
		}
		return e.Supervisors.Restart(id, spec)
	}
	return nil
}

// CreateDatabase runs the service's create-db command with a default timeout.
func (e *Engine) CreateDatabase(id, name string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	return e.CreateDatabaseContext(ctx, id, name)
}

// CreateDatabaseContext is CreateDatabase with caller-controlled cancellation.
func (e *Engine) CreateDatabaseContext(ctx context.Context, id, name string) (string, error) {
	spec, err := e.ServiceSpec(id)
	if err != nil {
		return "", err
	}
	argv, err := services.CreateDBCommand(spec, name)
	if err != nil {
		return "", err
	}
	out, err := exec.CommandContext(ctx, argv[0], argv[1:]...).CombinedOutput()
	return string(out), err
}

// DefaultDatabaseID is the database installed when Lair is set up for the first
// time. SQLite is the default: it needs no service, no port and no credentials,
// and its CLI is installed from the signed catalog.
const DefaultDatabaseID = "sqlite"

func isDatabaseService(id string) bool {
	switch id {
	case "sqlite", "mariadb", "mysql", "postgres":
		return true
	}
	return false
}

// DefaultDatabase returns the configured default database id.
func (e *Engine) DefaultDatabase() string {
	c, err := e.Config.View()
	if err == nil && isDatabaseService(c.Defaults["database"]) {
		return c.Defaults["database"]
	}
	return DefaultDatabaseID
}

// SetDefaultDatabase marks an installed database service as the default used
// by new Laravel projects.
func (e *Engine) SetDefaultDatabase(id string) error {
	if !isDatabaseService(id) {
		return fmt.Errorf("%q is not a database service", id)
	}
	if id == "sqlite" {
		if e.installedToolVersion(SQLiteToolID) == "" && len(e.installedToolVersions(SQLiteToolID)) == 0 {
			return fmt.Errorf("sqlite3 CLI is not installed")
		}
	} else if len(e.installedServiceVersions(id)) == 0 {
		return fmt.Errorf("%s is not installed", id)
	}
	return e.Config.Update(func(c *config.Config) error {
		c.Defaults["database"] = id
		return nil
	})
}

// EnsureDefaultDatabase installs the default database when none is installed
// yet. SQLite is just the catalog CLI plus a per-project database file, so it
// is handled here as a tool job. Other databases are supervised services.
// The function is idempotent and may return a nil job when nothing is needed.
func (e *Engine) EnsureDefaultDatabase(ctx context.Context) (*installer.Job, error) {
	id := e.DefaultDatabase()
	// Respect an existing database service: upgrading an installation must
	// never silently switch its projects to SQLite. The configured preference
	// wins; the fixed order is only a fallback.
	for _, candidate := range []string{id, "mariadb", "mysql", "postgres"} {
		if len(e.installedServiceVersions(candidate)) > 0 {
			if candidate != id {
				_ = e.Config.Update(func(c *config.Config) error {
					c.Defaults["database"] = candidate
					return nil
				})
			}
			return nil, nil
		}
	}
	if id == "sqlite" {
		if e.installedToolVersion(SQLiteToolID) == "" && len(e.installedToolVersions(SQLiteToolID)) == 0 {
			if err := e.Config.Update(func(c *config.Config) error {
				c.Defaults["database"] = "sqlite"
				return nil
			}); err != nil {
				return nil, err
			}
			return e.InstallTool(ctx, SQLiteToolID, "")
		}
		return nil, nil
	}
	if err := e.Config.Update(func(c *config.Config) error {
		c.Defaults["database"] = id
		return nil
	}); err != nil {
		return nil, err
	}
	return e.InstallService(ctx, id, "", 0)
}

func serviceKnown(id string) bool {
	for _, known := range services.IDs() {
		if known == id {
			return true
		}
	}
	return false
}

// SupervisorRegistry manages one supervisor per running service.
type SupervisorRegistry struct {
	mu      sync.Mutex
	sups    map[string]*supervisor.Supervisor
	cancels map[string]context.CancelFunc
	runner  supervisor.Runner
	logDir  string
}

func NewSupervisorRegistry(logDir string) *SupervisorRegistry {
	return &SupervisorRegistry{
		sups:    map[string]*supervisor.Supervisor{},
		cancels: map[string]context.CancelFunc{},
		runner:  &supervisor.OSRunner{LogDir: logDir},
		logDir:  logDir,
	}
}

// SetRunner injects a runner (used by tests).
func (r *SupervisorRegistry) SetRunner(runner supervisor.Runner) { r.runner = runner }

func (r *SupervisorRegistry) Start(id string, spec services.Spec) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.sups[id]; ok {
		return nil
	}
	sup := supervisor.New(spec, r.runner)
	ctx, cancel := context.WithCancel(context.Background())
	r.sups[id] = sup
	r.cancels[id] = cancel
	go func() { _ = sup.Run(ctx) }()
	return nil
}

func (r *SupervisorRegistry) Restart(id string, spec services.Spec) error {
	_ = r.Stop(id)
	return r.Start(id, spec)
}

func (r *SupervisorRegistry) Stop(id string) error {
	r.mu.Lock()
	sup, ok := r.sups[id]
	cancel, hasCancel := r.cancels[id]
	delete(r.sups, id)
	delete(r.cancels, id)
	r.mu.Unlock()
	if !ok {
		return nil
	}
	if hasCancel {
		cancel()
	}
	return sup.Stop()
}

func (r *SupervisorRegistry) State(id string) (supervisor.State, bool) {
	r.mu.Lock()
	sup, ok := r.sups[id]
	r.mu.Unlock()
	if !ok {
		return supervisor.State{}, false
	}
	return sup.State(), true
}

func (r *SupervisorRegistry) StopAll() {
	r.mu.Lock()
	ids := make([]string, 0, len(r.sups))
	for id := range r.sups {
		ids = append(ids, id)
	}
	r.mu.Unlock()
	for _, id := range ids {
		_ = r.Stop(id)
	}
}
