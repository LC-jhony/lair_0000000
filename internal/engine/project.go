package engine

import (
	"context"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/lair-sh/lair/internal/config"
	"github.com/lair-sh/lair/internal/installer"
	"github.com/lair-sh/lair/internal/pki"
	"github.com/lair-sh/lair/internal/project"
)

// projectTools adapts the engine's shim resolver to the project package.
type projectTools struct{ e *Engine }

func (p projectTools) ResolveTool(tool, dir string) (project.Invocation, error) {
	inv, err := p.e.ResolveTool(tool, dir)
	if err != nil {
		return project.Invocation{}, err
	}
	return project.Invocation{Path: inv.Path, Args: inv.Args, Env: inv.Env}, nil
}

// ProjectInfo reports the capabilities of this machine for the new-project
// wizard: installed runtimes, PDO drivers, Node managers and database engines.
func (e *Engine) ProjectInfo() project.Info {
	info := project.Detect(projectTools{e}, e.Paths)
	c, err := e.Config.View()
	if err == nil {
		info.ProjectsDir = e.projectsDir(c)
		// Prefer the configured engine when it is actually installed, so the
		// wizard asks for DB credentials instead of silently using SQLite.
		preferred := strings.TrimSpace(c.Defaults["database"])
		for _, driver := range info.Databases {
			if driver == preferred {
				info.DefaultDatabase = preferred
				break
			}
		}
	} else {
		info.ProjectsDir = project.DefaultParent()
	}
	return info
}

// projectsDir returns the configured primary folder for new projects,
// falling back to ~/Lair. It must never be Lair's own data root.
func (e *Engine) projectsDir(c *config.Config) string {
	if c != nil {
		if dir := strings.TrimSpace(c.ProjectsDir); dir != "" && dir != e.Paths.Root && !e.isInsideLairHome(dir) {
			return dir
		}
	}
	return project.DefaultParent()
}

// configDirs returns the normalised ProjectDirs list, always with at least
// the default folder and never containing LAIR_HOME.
func (e *Engine) configDirs(c *config.Config) []string {
	dirs := append([]string(nil), c.ProjectDirs...)
	out := make([]string, 0, len(dirs)+1)
	for _, dir := range dirs {
		dir = expandHome(strings.TrimSpace(dir))
		if dir == "" || e.isInsideLairHome(dir) {
			continue
		}
		out = append(out, dir)
	}
	if len(out) == 0 {
		out = append(out, config.DefaultProjectsDir(""))
	}
	return out
}

// expandHome resolves a leading ~ to the current user's home.
func expandHome(dir string) string {
	if !strings.HasPrefix(dir, "~") {
		return dir
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return dir
	}
	return filepath.Join(home, strings.TrimPrefix(dir, "~"))
}

// isInsideLairHome reports whether dir is LAIR_HOME or below it.
func (e *Engine) isInsideLairHome(dir string) bool {
	abs, err := filepath.Abs(expandHome(dir))
	if err != nil {
		return false
	}
	rel, err := filepath.Rel(e.Paths.Root, abs)
	if err != nil {
		return false
	}
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator)))
}

// assertProjectsDir rejects a parent inside LAIR_HOME. Projects belong to the
// user, not to Lair's runtime state.
func (e *Engine) assertProjectsDir(dir string) error {
	if strings.HasPrefix(dir, "~") {
		home, err := os.UserHomeDir()
		if err != nil {
			return err
		}
		dir = filepath.Join(home, strings.TrimPrefix(dir, "~"))
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return err
	}
	rel, err := filepath.Rel(e.Paths.Root, abs)
	if err != nil {
		return err
	}
	inside := rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator)))
	if inside {
		return fmt.Errorf("la carpeta de proyectos no puede estar dentro de LAIR_HOME (%s)", e.Paths.Root)
	}
	return nil
}

// ProjectControl implements the single `project` daemon action shared by the
// desktop app, the TUI and the CLI. `info` is synchronous; `create` returns a
// job that is polled with the existing `job`, `logs` and `job-cancel` actions.
func (e *Engine) ProjectControl(ctx context.Context, op, dir string, req project.CreateRequest) (project.Result, error) {
	return e.ProjectControlWith(ctx, project.ControlRequest{Op: op, Dir: dir, CreateRequest: req})
}

// ProjectControlWith is the full form of the project action. It accepts the
// flat {op, dir, from, to, ...} payload without adding a new daemon action:
// the desktop app and the TUI keep talking to `project` only.
func (e *Engine) ProjectControlWith(ctx context.Context, req project.ControlRequest) (project.Result, error) {
	op := strings.ToLower(strings.TrimSpace(req.Op))
	if strings.TrimSpace(req.Dir) == "" {
		req.Dir = req.Path
	}
	switch op {
	case "", "info":
		return project.Result{Info: e.ProjectInfo()}, nil
	case "dirs-list", "dirs": // `dirs` is the compatibility alias of `dirs-list`.
		return e.projectDirs()
	case "dirs-add":
		return e.projectDirsAdd(req.Dir)
	case "dirs-remove":
		return e.projectDirsRemove(req.Dir)
	case "dirs-default", "set-dir":
		return e.projectSetDir(req.Dir)
	case "migrate-dirs":
		return e.projectDirsMigrate(req.From, req.To)
	case "import-valet":
		return e.projectImportValet()
	case "sync":
		if err := e.SyncProjectDirs(); err != nil {
			return project.Result{}, err
		}
		return project.Result{}, nil
	case "set-db":
		if err := e.SetSiteDatabase(req.Name, req.Database); err != nil {
			return project.Result{}, err
		}
		return project.Result{Path: req.Name}, nil
	case "create":
		c, err := e.Config.View()
		if err != nil {
			return project.Result{}, err
		}
		if strings.TrimSpace(req.Parent) == "" {
			req.Parent = e.projectsDir(c)
			if err := os.MkdirAll(req.Parent, 0o755); err != nil {
				return project.Result{}, fmt.Errorf("no se pudo crear la carpeta de proyectos %s: %w", req.Parent, err)
			}
		}
		if err := e.assertProjectsDir(req.Parent); err != nil {
			return project.Result{}, err
		}
		target, err := req.Normalize()
		if err != nil {
			return project.Result{}, err
		}
		domain := e.WebSettings().Domain
		site := strings.ToLower(req.Name) + "." + domain
		id := "project-" + req.Name
		job, err := e.Installer.Run(ctx, installer.RunRequest{
			ID: id, Package: "project", Version: req.Name,
			Run: func(ctx context.Context, log func(string)) error {
				return e.createProject(ctx, req.CreateRequest, target, site, log)
			},
		})
		if err != nil {
			return project.Result{}, err
		}
		st := job.Status()
		return project.Result{
			ID: st.ID, Phase: st.Phase, Done: st.Done,
			Error: st.Error, Canceled: st.Canceled,
			Site: site, Path: target,
		}, nil
	default:
		return project.Result{}, fmt.Errorf("unknown project operation %q", op)
	}
}

// SyncProjectDirs registers every Laravel project under the configured
// projects folder as a <name>.test site. It is idempotent and never replaces a
// site that was linked explicitly somewhere else.
func (e *Engine) SyncProjectDirs() error {
	c, err := e.Config.View()
	if err != nil {
		return err
	}
	domain := e.WebSettings().Domain
	changed := false
	for _, configured := range e.configDirs(c) {
		dir, err := filepath.Abs(configured)
		if err != nil {
			return err
		}
		entries, err := os.ReadDir(dir)
		if err != nil {
			// A missing projects folder simply means there is nothing to discover.
			if os.IsNotExist(err) {
				continue
			}
			return err
		}
		for _, entry := range entries {
			if !entry.IsDir() || !project.ValidName(entry.Name()) {
				continue
			}
			path := filepath.Join(dir, entry.Name())
			if !looksLikeProject(path) {
				continue
			}
			site := strings.ToLower(entry.Name()) + "." + domain
			err := e.Config.Update(func(cfg *config.Config) error {
				if current, ok := cfg.Sites[site]; ok && current.Path != "" {
					return nil
				}
				cfg.Sites[site] = config.Site{Path: path, Enabled: true}
				changed = true
				return nil
			})
			if err != nil {
				return err
			}
		}
	}
	if changed {
		e.reloadSites()
		e.syncWeb()
	}
	return nil
}

func looksLikeProject(path string) bool {
	for _, name := range []string{"artisan", "composer.json"} {
		if info, err := os.Stat(filepath.Join(path, name)); err == nil && !info.IsDir() {
			return true
		}
	}
	return false
}

// projectSetDir persists the default parent for new projects.
func (e *Engine) projectSetDir(dir string) (project.Result, error) {
	dir = strings.TrimSpace(dir)
	if dir == "" {
		return project.Result{}, fmt.Errorf("indica la carpeta de proyectos")
	}
	if strings.HasPrefix(dir, "~") {
		home, err := os.UserHomeDir()
		if err != nil {
			return project.Result{}, err
		}
		dir = filepath.Join(home, strings.TrimPrefix(dir, "~"))
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return project.Result{}, err
	}
	info, err := os.Stat(abs)
	if err != nil || !info.IsDir() {
		return project.Result{}, fmt.Errorf("la carpeta de proyectos no existe: %s", abs)
	}
	if err := e.assertProjectsDir(abs); err != nil {
		return project.Result{}, err
	}
	if err := e.Config.Update(func(c *config.Config) error {
		c.ProjectsDir = abs
		dirs := e.configDirs(c)
		dirs[0] = abs
		c.ProjectDirs = dirs
		return nil
	}); err != nil {
		return project.Result{}, err
	}
	return project.Result{Path: abs}, nil
}

// projectDirs reports the configured list of project folders.
func (e *Engine) projectDirs() (project.Result, error) {
	c, err := e.Config.View()
	if err != nil {
		return project.Result{}, err
	}
	return project.Result{Dirs: e.configDirs(c)}, nil
}

// projectDirsAdd validates (or creates) a folder and appends it to
// ProjectDirs. Folders inside LAIR_HOME are rejected: projects belong to the
// user, not to Lair's runtime state.
func (e *Engine) projectDirsAdd(raw string) (project.Result, error) {
	dir := expandHome(strings.TrimSpace(raw))
	if dir == "" {
		return project.Result{}, fmt.Errorf("indica la carpeta de proyectos")
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return project.Result{}, err
	}
	if e.isInsideLairHome(abs) {
		return project.Result{}, fmt.Errorf("la carpeta de proyectos no puede estar dentro de LAIR_HOME (%s)", e.Paths.Root)
	}
	if info, err := os.Stat(abs); err == nil {
		if !info.IsDir() {
			return project.Result{}, fmt.Errorf("la ruta no es una carpeta: %s", abs)
		}
	} else if os.IsNotExist(err) {
		if err := os.MkdirAll(abs, 0o755); err != nil {
			return project.Result{}, fmt.Errorf("no se pudo crear la carpeta de proyectos %s: %w", abs, err)
		}
	} else {
		return project.Result{}, err
	}
	var dirs []string
	if err := e.Config.Update(func(c *config.Config) error {
		dirs = e.configDirs(c)
		for _, existing := range dirs {
			if samePath(existing, abs) {
				return nil
			}
		}
		dirs = append(dirs, abs)
		c.ProjectDirs = dirs
		c.ProjectsDir = dirs[0]
		return nil
	}); err != nil {
		return project.Result{}, err
	}
	return project.Result{Dirs: dirs}, nil
}

// projectDirsRemove forgets a folder without deleting anything from disk.
func (e *Engine) projectDirsRemove(raw string) (project.Result, error) {
	target := expandHome(strings.TrimSpace(raw))
	if target == "" {
		return project.Result{}, fmt.Errorf("indica la carpeta de proyectos")
	}
	abs, err := filepath.Abs(target)
	if err != nil {
		return project.Result{}, err
	}
	var dirs []string
	if err := e.Config.Update(func(c *config.Config) error {
		current := e.configDirs(c)
		kept := make([]string, 0, len(current))
		for _, dir := range current {
			if !samePath(dir, abs) {
				kept = append(kept, dir)
			}
		}
		if len(kept) == 0 {
			return fmt.Errorf("no se puede quitar la única carpeta de proyectos")
		}
		dirs = kept
		c.ProjectDirs = kept
		c.ProjectsDir = kept[0]
		return nil
	}); err != nil {
		return project.Result{}, err
	}
	return project.Result{Dirs: dirs}, nil
}

// projectDirsMigrate moves a projects folder and rewrites every configured
// path under it (ProjectDirs and Site paths). If any config write fails the
// folder is moved back, so a failed migration never leaves a half state.
func (e *Engine) projectDirsMigrate(rawFrom, rawTo string) (project.Result, error) {
	from := expandHome(strings.TrimSpace(rawFrom))
	to := expandHome(strings.TrimSpace(rawTo))
	// The documented CLI call takes no arguments: it migrates the legacy
	// ~/lair folder to ~/Lair. Explicit {from,to} payloads do the same in a
	// caller-chosen direction.
	if from == "" || to == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return project.Result{}, err
		}
		if from == "" {
			from = filepath.Join(home, config.LegacyProjectsDirName)
		}
		if to == "" {
			to = config.DefaultProjectsDir(home)
		}
	}
	fromAbs, err := filepath.Abs(from)
	if err != nil {
		return project.Result{}, err
	}
	toAbs, err := filepath.Abs(to)
	if err != nil {
		return project.Result{}, err
	}
	if samePath(fromAbs, toAbs) {
		return project.Result{}, fmt.Errorf("origen y destino son la misma carpeta: %s", toAbs)
	}
	if e.isInsideLairHome(toAbs) {
		return project.Result{}, fmt.Errorf("la carpeta de proyectos no puede estar dentro de LAIR_HOME (%s)", e.Paths.Root)
	}
	if _, err := os.Lstat(toAbs); err == nil {
		return project.Result{}, fmt.Errorf("el destino ya existe: %s", toAbs)
	} else if !os.IsNotExist(err) {
		return project.Result{}, err
	}

	moved := false
	if info, err := os.Lstat(fromAbs); err == nil {
		if !info.IsDir() {
			return project.Result{}, fmt.Errorf("el origen no es una carpeta: %s", fromAbs)
		}
		if err := moveDir(fromAbs, toAbs); err != nil {
			return project.Result{}, err
		}
		moved = true
	} else if !os.IsNotExist(err) {
		return project.Result{}, err
	}

	var dirs []string
	err = e.Config.Update(func(c *config.Config) error {
		dirs = e.configDirs(c)
		for i, dir := range dirs {
			if rel, ok := relativeUnder(dir, fromAbs); ok {
				dirs[i] = filepath.Join(toAbs, rel)
			}
		}
		c.ProjectDirs = dirs
		c.ProjectsDir = dirs[0]
		for name, site := range c.Sites {
			if rel, ok := relativeUnder(site.Path, fromAbs); ok {
				site.Path = filepath.Join(toAbs, rel)
				c.Sites[name] = site
			}
		}
		return nil
	})
	if err != nil {
		if moved {
			_ = os.Rename(toAbs, fromAbs)
		}
		return project.Result{}, err
	}
	// Laravel stores an absolute SQLite path in .env; rewrite it after the move
	// so migrated projects keep working instead of failing with 500.
	if current, viewErr := e.Config.View(); viewErr == nil {
		for _, site := range current.Sites {
			if _, ok := relativeUnder(site.Path, toAbs); !ok {
				continue
			}
			for _, file := range []string{".env", ".env.example"} {
				path := filepath.Join(site.Path, file)
				data, readErr := os.ReadFile(path)
				if readErr != nil {
					continue
				}
				text := string(data)
				if !strings.Contains(text, fromAbs) {
					continue
				}
				_ = os.WriteFile(path, []byte(strings.ReplaceAll(text, fromAbs, toAbs)), 0o600)
			}
		}
	}
	// Re-route parked sites so their new roots take effect immediately.
	e.reloadSites()
	verb := "La carpeta se movió"
	if !moved {
		verb = "No había carpeta que mover; se actualizaron las rutas"
	}
	return project.Result{
		Path:   toAbs,
		Dirs:   dirs,
		Output: fmt.Sprintf("%s de %s a %s.", verb, fromAbs, toAbs),
	}, nil
}

// projectImportValet reads a Valet installation without modifying it. Only
// WebSettings.Domain and ProjectDirs change; parked sites are left untouched.
func (e *Engine) projectImportValet() (project.Result, error) {
	c, err := e.Config.View()
	if err != nil {
		return project.Result{}, err
	}
	registered := map[string]bool{}
	for name := range c.Sites {
		registered[strings.ToLower(name)] = true
	}
	v, err := project.ReadValet("", registered)
	if err != nil {
		return project.Result{}, err
	}
	var dirs, added []string
	domainChanged := false
	if err := e.Config.Update(func(cfg *config.Config) error {
		dirs = e.configDirs(cfg)
		for _, p := range v.Paths {
			abs, aerr := filepath.Abs(p)
			if aerr != nil || e.isInsideLairHome(abs) {
				continue
			}
			info, serr := os.Stat(abs)
			if serr != nil || !info.IsDir() {
				continue
			}
			found := false
			for _, dir := range dirs {
				if samePath(dir, abs) {
					found = true
					break
				}
			}
			if !found {
				dirs = append(dirs, abs)
				added = append(added, abs)
			}
		}
		cfg.ProjectDirs = dirs
		cfg.ProjectsDir = dirs[0]
		if v.Domain != "" && cfg.Web.Domain != v.Domain {
			cfg.Web.Domain = v.Domain
			domainChanged = true
		}
		return nil
	}); err != nil {
		return project.Result{}, err
	}
	if domainChanged {
		e.applyWebToServer(e.WebSettings())
	}
	return project.Result{
		Dirs:   dirs,
		Output: project.ValetOutput(v, added, domainChanged),
	}, nil
}

// samePath compares two already-expanded paths without touching the disk.
func samePath(a, b string) bool { return filepath.Clean(a) == filepath.Clean(b) }

// relativeUnder returns the slash-separated path of file below base. It
// returns ("", true) when file == base and ("", false) when unrelated.
func relativeUnder(file, base string) (string, bool) {
	abs, err := filepath.Abs(expandHome(file))
	if err != nil {
		return "", false
	}
	rel, err := filepath.Rel(base, abs)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return "", false
	}
	if rel == "." {
		return "", true
	}
	return rel, true
}

// moveDir renames a folder, falling back to an explicit copy when the source
// and the destination live on different filesystems. A failed copy removes
// the partial destination, so the caller can roll the migration back.
func moveDir(from, to string) error {
	if err := os.Rename(from, to); err == nil {
		return nil
	}
	if err := os.MkdirAll(to, 0o755); err != nil {
		return err
	}
	if err := copyTree(from, to); err != nil {
		_ = os.RemoveAll(to)
		return fmt.Errorf("no se pudo mover %s a %s: %w", from, to, err)
	}
	if err := os.RemoveAll(from); err != nil {
		return fmt.Errorf("no se pudo eliminar la carpeta original %s: %w", from, err)
	}
	return nil
}

// copyTree copies the contents of from into an existing to, preserving modes,
// symlinks and empty folders. It is only used for cross-device migrations.
func copyTree(from, to string) error {
	return filepath.WalkDir(from, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(from, path)
		if err != nil {
			return err
		}
		dest := filepath.Join(to, rel)
		if entry.IsDir() {
			return os.MkdirAll(dest, 0o755)
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			target, err := os.Readlink(path)
			if err != nil {
				return err
			}
			return os.Symlink(target, dest)
		}
		src, err := os.Open(path)
		if err != nil {
			return err
		}
		defer src.Close()
		out, err := os.OpenFile(dest, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, info.Mode().Perm())
		if err != nil {
			return err
		}
		if _, err := io.Copy(out, src); err != nil {
			out.Close()
			return err
		}
		return out.Close()
	})
}

// databaseServiceID maps a Laravel database driver to the supervised service
// that owns it. MySQL has no signed build in Lair's catalog, so the `mysql`
// PDO driver falls back to an installed MariaDB service (same protocol).
func (e *Engine) databaseServiceID(driver string) string {
	id := project.ServiceID(driver)
	if id != "mysql" || len(e.installedServiceVersions("mysql")) > 0 {
		return id
	}
	if len(e.installedServiceVersions("mariadb")) == 0 {
		return "" // let ServiceSpec report that no engine is available
	}
	return "mariadb"
}

// createProject runs the ordered stages. A failed creation removes the folder
// it may have created, so a retry never finds a half-built project.
func (e *Engine) createProject(ctx context.Context, req project.CreateRequest, target, site string, log func(string)) (err error) {
	created := false
	defer func() {
		if err != nil && created {
			_ = os.RemoveAll(target)
			log("Se eliminó la carpeta incompleta " + target)
		}
	}()

	log("Preparando el instalador de Laravel…")
	// Refreshes ca-bundle.pem (system roots + local CA) before Composer runs,
	// so packagist and local *.test sites are both trusted.
	if _, _, caErr := pki.EnsureCA(e.Paths.Certs); caErr != nil {
		log(fmt.Sprintf("Aviso: no se pudo preparar el bundle de CA: %v", caErr))
	}
	bin, err := project.EnsureInstaller(ctx, projectTools{e}, e.Paths, log)
	if err != nil {
		return err
	}

	serviceID := e.databaseServiceID(req.Database)
	if req.Database == "mysql" && serviceID == "mariadb" {
		log("MySQL no tiene build en Lair: se usa MariaDB para el driver mysql.")
		// Keep the Laravel default port in sync with the installed service.
		if c, err := e.Config.View(); err == nil {
			if port := c.Ports["mariadb"]; port > 0 && req.DBPort == 3306 {
				req.DBPort = port
			}
		}
	}

	if req.Database != "sqlite" && req.CreateDB {
		log("Preparando la base de datos…")
		if serviceID != "" {
			if req.StartService {
				log("Arrancando " + serviceID + " si está parado…")
				if err := e.ensureServiceRunning(ctx, serviceID); err != nil {
					return err
				}
			}
			out, dbErr := e.createDatabaseWithCredentials(ctx, serviceID, req, log)
			if dbErr != nil {
				return dbErr
			}
			if text := strings.TrimSpace(project.Redact(out, req.DBPassword)); text != "" {
				log(text)
			}
		} else {
			log("Aviso: SQL Server no es gestionado por Lair; crea la base de datos manualmente.")
		}
	}

	log("Creando el proyecto con laravel new…")
	created = true
	if err = project.RunLaravel(ctx, e.Paths, bin, req.Parent, req, log); err != nil {
		return err
	}
	if err = ctx.Err(); err != nil {
		return err
	}

	// The installer creates database/database.sqlite itself. Ensure it exists
	// only AFTER `laravel new` succeeds: creating the folder beforehand makes
	// the installer reject the target with "Application already exists!".
	// This is a best-effort fallback, so it must never delete a project that
	// was created correctly.
	if req.Database == "sqlite" {
		if sqliteErr := project.EnsureSQLiteFile(req.Parent, req.Name); sqliteErr != nil {
			log(fmt.Sprintf("Aviso: no se pudo preparar SQLite: %v", sqliteErr))
		}
	}

	log("Guardando la configuración de la base de datos…")
	web := e.WebSettings()
	appURL := project.AppURL(req.Name, web.Domain, web, req.Secure)
	values := project.EnvFileValues(req, appURL, req.Parent, false)
	if patchErr := project.PatchEnvFile(filepath.Join(target, ".env"), values); patchErr != nil {
		log(fmt.Sprintf("Aviso: no se pudo actualizar .env: %v", patchErr))
	}
	// The committed example never receives the real password.
	example := project.EnvFileValues(req, appURL, req.Parent, true)
	if patchErr := project.PatchEnvFile(filepath.Join(target, ".env.example"), example); patchErr != nil {
		log(fmt.Sprintf("Aviso: no se pudo actualizar .env.example: %v", patchErr))
	}
	log("Registrando el sitio en Lair…")
	if err = e.Link(site, target); err != nil {
		return err
	}
	if err = e.SetSiteDatabase(site, req.Database); err != nil {
		log(fmt.Sprintf("Aviso: no se pudo guardar la base de datos del sitio: %v", err))
	}
	if req.Secure {
		if err = e.Secure(site); err != nil {
			return err
		}
	}
	log("Proyecto listo: " + appURL + " -> " + target)
	return nil
}

// ensureServiceRunning starts a supervised service and waits until its state
// is actually running, so the CREATE DATABASE statement reaches a live server.
func (e *Engine) ensureServiceRunning(ctx context.Context, id string) error {
	if st, ok := e.Supervisors.State(id); ok && st.Running {
		return nil
	}
	if err := e.StartService(id); err != nil {
		return fmt.Errorf("no se pudo arrancar %s: %w", id, err)
	}
	deadline := time.Now().Add(10 * time.Second)
	for {
		if st, ok := e.Supervisors.State(id); ok && st.Running {
			return nil
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("%s no respondió a tiempo tras arrancarlo", id)
		}
		time.Sleep(150 * time.Millisecond)
	}
}

// createDatabaseWithCredentials uses the Lair-managed client with the
// credentials from the wizard. Secrets are passed by environment, never in
// argv or in the job log.
func (e *Engine) createDatabaseWithCredentials(ctx context.Context, id string, req project.CreateRequest, log func(string)) (string, error) {
	spec, err := e.ServiceSpec(id)
	if err != nil {
		return "", fmt.Errorf("no se puede crear la base de datos: %w", err)
	}
	var bin string
	var argv, env []string
	switch id {
	case "postgres":
		bin = filepath.Join(spec.Dir, "bin", "psql")
		env = []string{
			"PGPASSWORD=" + req.DBPassword,
			"PGHOST=" + req.DBHost,
			"PGPORT=" + strconv.Itoa(req.DBPort),
			"PGUSER=" + req.DBUser,
		}
		argv = []string{"-v", "ON_ERROR_STOP=1", "-c", fmt.Sprintf(`CREATE DATABASE "%s"`, req.DBName)}
	case "mysql", "mariadb":
		client := "mysql"
		if id == "mariadb" {
			client = "mariadb"
		}
		bin = filepath.Join(spec.Dir, "bin", client)
		env = []string{"MYSQL_PWD=" + req.DBPassword}
		argv = []string{
			"-h", req.DBHost, "-P", strconv.Itoa(req.DBPort), "-u", req.DBUser,
			"-e", fmt.Sprintf("CREATE DATABASE IF NOT EXISTS `%s` CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci", req.DBName),
		}
	default:
		return "", fmt.Errorf("creación de base de datos no soportada para %s", id)
	}
	cmd := exec.CommandContext(ctx, bin, argv...)
	cmd.Env = append(os.Environ(), env...)
	out, cmdErr := cmd.CombinedOutput()
	if text := strings.TrimSpace(project.Redact(string(out), req.DBPassword)); text != "" {
		log(text)
	}
	if cmdErr != nil {
		return string(out), fmt.Errorf("no se pudo crear la base de datos %s: %w", req.DBName, cmdErr)
	}
	return string(out), nil
}
