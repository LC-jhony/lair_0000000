package engine

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Database snapshots are plain SQL/SQLite dump files under <LAIR_HOME>/snapshots
// with a JSON sidecar. No dump bytes travel over the daemon API: callers pass a
// validated name and the engine works with paths it owns.

// DBRequest is the wire payload of the `db` daemon action.
type DBRequest struct {
	Op       string `json:"op"` // list|snapshot|snapshots|restore|import|export|snapshot:auto
	Site     string `json:"site,omitempty"`
	Database string `json:"database,omitempty"`
	Name     string `json:"name,omitempty"` // snapshot id
	File     string `json:"file,omitempty"` // validated import/export path
	Confirm  bool   `json:"confirm,omitempty"`
}

// SnapshotInfo describes one stored snapshot.
type SnapshotInfo struct {
	ID       string `json:"id"`
	Site     string `json:"site"`
	Database string `json:"database"`
	Driver   string `json:"driver"`
	File     string `json:"file"`
	Bytes    int64  `json:"bytes"`
	SHA256   string `json:"sha256"`
	Created  string `json:"created"`
	Auto     bool   `json:"auto"`
}

// DBResult is returned by every db operation.
type DBResult struct {
	Output    string         `json:"output,omitempty"`
	Snapshots []SnapshotInfo `json:"snapshots,omitempty"`
}

type snapshotMeta struct {
	Version  int    `json:"version"`
	ID       string `json:"id"`
	Site     string `json:"site"`
	Database string `json:"database"`
	Driver   string `json:"driver"`
	Created  string `json:"created"`
	Auto     bool   `json:"auto"`
}

var dbSafeRE = regexp.MustCompile(`^[A-Za-z0-9_.-]+$`)

func dbSafe(value string) bool { return dbSafeRE.MatchString(value) && value != "." && value != ".." }

// dbTarget holds everything needed to run a client against one site database.
type dbTarget struct {
	Site     string
	Driver   string
	Service  string
	Database string
	User     string
	Password string
	Port     int
	Dir      string
	SQLite   string // absolute database file when driver=sqlite
}

func (e *Engine) dbTarget(name string) (dbTarget, error) {
	name = strings.ToLower(strings.TrimSuffix(strings.TrimSpace(name), "."))
	var t dbTarget
	c, err := e.Config.View()
	if err != nil {
		return t, err
	}
	site, ok := c.Sites[name]
	if !ok || site.Path == "" {
		return t, fmt.Errorf("sitio desconocido %q", name)
	}
	env, _, err := readEnvFile(filepath.Join(site.Path, ".env"))
	if err != nil {
		return t, err
	}
	t.Site = name
	t.Driver = strings.TrimSpace(env["DB_CONNECTION"])
	if t.Driver == "" {
		t.Driver = site.Database
	}
	if t.Driver == "" {
		t.Driver = "mariadb"
	}
	t.Database = strings.TrimSpace(env["DB_DATABASE"])
	if t.Database == "" {
		t.Database = name
	}
	t.User = strings.TrimSpace(env["DB_USERNAME"])
	t.Password = env["DB_PASSWORD"]
	t.Service = e.databaseServiceID(t.Driver)
	if t.Driver == "sqlite" {
		file := t.Database
		if !filepath.IsAbs(file) {
			if strings.HasPrefix(file, "database/") || strings.HasPrefix(file, "database"+string(filepath.Separator)) {
				file = filepath.Join(site.Path, file)
			} else {
				file = filepath.Join(site.Path, "database", file)
			}
		}
		abs, err := filepath.Abs(file)
		if err != nil {
			return t, err
		}
		t.SQLite = abs
		return t, nil
	}
	spec, err := e.ServiceSpec(t.Service)
	if err != nil {
		return t, err
	}
	t.Dir = spec.Dir
	t.Port = c.Ports[t.Service]
	if raw := strings.TrimSpace(env["DB_PORT"]); raw != "" {
		if parsed, convErr := strconv.Atoi(raw); convErr == nil && parsed > 0 {
			t.Port = parsed
		}
	}
	if t.Port == 0 {
		t.Port = spec.Port
	}
	return t, nil
}

func (t dbTarget) client(name string) string { return filepath.Join(t.Dir, "bin", name) }

func (t dbTarget) env() []string {
	out := []string{}
	if t.Driver == "pgsql" || t.Service == "postgres" {
		if t.User != "" {
			out = append(out, "PGUSER="+t.User)
		}
		if t.Password != "" {
			out = append(out, "PGPASSWORD="+t.Password)
		}
	} else {
		if t.User != "" {
			out = append(out, "MYSQL_PWD="+t.Password)
		}
	}
	return out
}

func (t dbTarget) hostArgs() []string {
	if t.Driver == "pgsql" || t.Service == "postgres" {
		user := t.User
		if user == "" {
			user = "postgres"
		}
		return []string{"-h", "127.0.0.1", "-p", strconv.Itoa(t.Port), "-U", user, "-d", t.Database}
	}
	user := t.User
	if user == "" {
		user = "root"
	}
	return []string{"-h", "127.0.0.1", "-P", strconv.Itoa(t.Port), "-u", user, t.Database}
}

func (e *Engine) snapshotDir() string {
	c, err := e.Config.View()
	if err == nil && c.Snapshots.Dir != "" {
		return c.Snapshots.Dir
	}
	return e.Paths.Snapshots
}

// snapshotID builds a stable, sortable id. Names are validated before they
// reach the filesystem, so no traversal or absolute path is possible.
func snapshotID(site, label string) string {
	stamp := time.Now().UTC().Format("20060102-150405.000000000")
	if label == "" {
		label = "manual"
	}
	return site + "-" + label + "-" + stamp
}

func (e *Engine) snapshotPath(id string) (dump, meta string) {
	dir := e.snapshotDir()
	return filepath.Join(dir, id+".sql"), filepath.Join(dir, id+".json")
}

// writeSnapshot runs the engine's dump tool into a new 0600 file and records a
// JSON sidecar. Returns the completed snapshot for the caller to report.
func (e *Engine) writeSnapshot(ctx context.Context, t dbTarget, label string, auto bool) (SnapshotInfo, error) {
	// For SQLite the "database" is a file path, not a name; only the site
	// (used in the snapshot id) must be a safe token.
	if !dbSafe(t.Site) || (t.Driver != "sqlite" && !dbSafe(t.Database)) {
		return SnapshotInfo{}, fmt.Errorf("nombre de sitio o base de datos no válido")
	}
	id := snapshotID(t.Site, label)
	dumpPath, metaPath := e.snapshotPath(id)
	if err := os.MkdirAll(e.snapshotDir(), 0o700); err != nil {
		return SnapshotInfo{}, err
	}
	f, err := os.OpenFile(dumpPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return SnapshotInfo{}, err
	}
	run := func(bin string, args []string, stdin io.Reader) error {
		cmd := exec.CommandContext(ctx, bin, args...)
		cmd.Env = append(cmd.Environ(), t.env()...)
		cmd.Stdin = stdin
		cmd.Stdout = f
		cmd.Stderr = os.Stderr
		return cmd.Run()
	}
	var dumpErr error
	switch {
	case t.Driver == "sqlite":
		src, err := os.Open(t.SQLite)
		if err != nil {
			dumpErr = err
			break
		}
		_, dumpErr = io.Copy(f, src)
		_ = src.Close()
	case t.Driver == "pgsql" || t.Service == "postgres":
		args := append([]string{"--no-owner", "--no-privileges"}, t.hostArgs()...)
		dumpErr = run(t.client("pg_dump"), args, nil)
	default:
		tool := "mysqldump"
		if _, statErr := os.Stat(t.client("mariadb-dump")); statErr == nil {
			tool = "mariadb-dump"
		}
		args := append([]string{"--single-transaction", "--routines", "--triggers"}, t.hostArgs()...)
		dumpErr = run(t.client(tool), args, nil)
	}
	if cerr := f.Close(); dumpErr == nil {
		dumpErr = cerr
	}
	if dumpErr != nil {
		_ = os.Remove(dumpPath)
		return SnapshotInfo{}, fmt.Errorf("snapshot: %w", dumpErr)
	}
	info, err := os.Stat(dumpPath)
	if err != nil {
		return SnapshotInfo{}, err
	}
	sum, err := fileSHA256(dumpPath)
	if err != nil {
		return SnapshotInfo{}, err
	}
	meta := snapshotMeta{Version: 1, ID: id, Site: t.Site, Database: t.Database,
		Driver: t.Driver, Created: time.Now().UTC().Format(time.RFC3339), Auto: auto}
	data, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return SnapshotInfo{}, err
	}
	if err := os.WriteFile(metaPath, data, 0o600); err != nil {
		return SnapshotInfo{}, err
	}
	snapshot := SnapshotInfo{ID: id, Site: t.Site, Database: t.Database, Driver: t.Driver,
		File: dumpPath, Bytes: info.Size(), SHA256: sum, Created: meta.Created, Auto: auto}
	if err := e.pruneSnapshots(); err != nil {
		return snapshot, err
	}
	return snapshot, nil
}

func fileSHA256(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func (e *Engine) pruneSnapshots() error {
	snaps, err := e.listSnapshotInfo()
	if err != nil {
		return err
	}
	keep := 5
	if c, err := e.Config.View(); err == nil && c.Snapshots.Keep > 0 {
		keep = c.Snapshots.Keep
	}
	if len(snaps) <= keep {
		return nil
	}
	sort.Slice(snaps, func(i, j int) bool { return snaps[i].Created > snaps[j].Created })
	for _, old := range snaps[keep:] {
		dump, meta := e.snapshotPath(old.ID)
		_ = os.Remove(dump)
		_ = os.Remove(meta)
	}
	return nil
}

func (e *Engine) listSnapshotInfo() ([]SnapshotInfo, error) {
	entries, err := os.ReadDir(e.snapshotDir())
	if os.IsNotExist(err) {
		return []SnapshotInfo{}, nil
	}
	if err != nil {
		return nil, err
	}
	out := []SnapshotInfo{}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(e.snapshotDir(), entry.Name()))
		if err != nil {
			continue
		}
		var meta snapshotMeta
		if json.Unmarshal(data, &meta) != nil || !dbSafe(meta.ID) {
			continue
		}
		dump, _ := e.snapshotPath(meta.ID)
		st, err := os.Stat(dump)
		if err != nil {
			continue
		}
		sum, _ := fileSHA256(dump)
		out = append(out, SnapshotInfo{ID: meta.ID, Site: meta.Site, Database: meta.Database,
			Driver: meta.Driver, File: dump, Bytes: st.Size(), SHA256: sum,
			Created: meta.Created, Auto: meta.Auto})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Created > out[j].Created })
	return out, nil
}

func (e *Engine) findSnapshot(id string) (SnapshotInfo, error) {
	if !dbSafe(id) {
		return SnapshotInfo{}, fmt.Errorf("id de snapshot no válido")
	}
	snaps, err := e.listSnapshotInfo()
	if err != nil {
		return SnapshotInfo{}, err
	}
	for _, s := range snaps {
		if s.ID == id {
			return s, nil
		}
	}
	return SnapshotInfo{}, fmt.Errorf("snapshot %q no encontrado", id)
}

// validateImportPath accepts only files under the snapshots dir or under the
// user's home, and rejects symlinks in the final component.
func (e *Engine) validateImportPath(raw string) (string, error) {
	if raw == "" {
		return "", fmt.Errorf("falta la ruta del archivo")
	}
	path, err := filepath.Abs(raw)
	if err != nil {
		return "", err
	}
	info, err := os.Lstat(path)
	if err != nil {
		return "", err
	}
	if info.Mode()&os.ModeSymlink != 0 || info.IsDir() {
		return "", fmt.Errorf("ruta de importación no válida")
	}
	home, _ := os.UserHomeDir()
	allowed := false
	for _, base := range []string{e.snapshotDir(), home} {
		if base == "" {
			continue
		}
		rel, err := filepath.Rel(base, path)
		if err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			allowed = true
			break
		}
	}
	if !allowed {
		return "", fmt.Errorf("el archivo debe estar en snapshots o en tu home")
	}
	return path, nil
}

// autoSnapshotSite snapshots a site's database before a destructive action.
// It is a no-op when the policy is off or the site has no database file, and
// returns an error when a configured auto snapshot fails, so the destructive
// action is aborted rather than losing the data unprotected.
func (e *Engine) autoSnapshotSite(ctx context.Context, name string) error {
	c, err := e.Config.View()
	if err != nil || !c.Snapshots.Auto {
		return nil
	}
	t, err := e.dbTarget(name)
	if err != nil {
		// No installed engine or no readable .env: nothing can be captured,
		// so do not block the destructive action.
		return nil
	}
	if t.Driver == "sqlite" {
		if _, err := os.Stat(t.SQLite); os.IsNotExist(err) {
			return nil
		}
	}
	if _, err := e.writeSnapshot(ctx, t, "before-delete", true); err != nil {
		return fmt.Errorf("snapshot automático de %s: %w", name, err)
	}
	return nil
}

// autoSnapshotService snapshots every site whose database is served by the
// service about to lose its data directory.
func (e *Engine) autoSnapshotService(ctx context.Context, service string) error {
	c, err := e.Config.View()
	if err != nil || !c.Snapshots.Auto {
		return nil
	}
	for name, site := range c.Sites {
		driver := site.Database
		if env, _, readErr := readEnvFile(filepath.Join(site.Path, ".env")); readErr == nil {
			if fromEnv := strings.TrimSpace(env["DB_CONNECTION"]); fromEnv != "" {
				driver = fromEnv
			}
		}
		if e.databaseServiceID(driver) != service {
			continue
		}
		if err := e.autoSnapshotSite(ctx, name); err != nil {
			return err
		}
	}
	return nil
}

// DBControl handles snapshot, restore, import and export. Destructive ops
// take an automatic snapshot first so a bad import can always be undone.
func (e *Engine) DBControl(ctx context.Context, req DBRequest) (DBResult, error) {
	var out DBResult
	e.dbMu.Lock()
	defer e.dbMu.Unlock()
	switch req.Op {
	case "", "list", "snapshots":
		snaps, err := e.listSnapshotInfo()
		if err != nil {
			return out, err
		}
		out.Snapshots = snaps
		out.Output = fmt.Sprintf("%d snapshots", len(snaps))
	case "snapshot", "snapshot:auto":
		t, err := e.dbTarget(req.Site)
		if err != nil {
			return out, err
		}
		label := "manual"
		if req.Op == "snapshot:auto" {
			label = "auto"
		}
		s, err := e.writeSnapshot(ctx, t, label, req.Op == "snapshot:auto")
		if err != nil {
			return out, err
		}
		out.Output = "snapshot " + s.ID
	case "restore":
		if !req.Confirm {
			return out, fmt.Errorf("restaurar requiere confirmación (confirm: true)")
		}
		t, err := e.dbTarget(req.Site)
		if err != nil {
			return out, err
		}
		s, err := e.findSnapshot(req.Name)
		if err != nil {
			return out, err
		}
		if _, err := e.writeSnapshot(ctx, t, "before-restore", true); err != nil {
			return out, fmt.Errorf("snapshot previo a restaurar: %w", err)
		}
		if err := e.restoreDump(ctx, t, s.File); err != nil {
			return out, err
		}
		out.Output = "base restaurada desde " + s.ID
	case "import":
		if !req.Confirm {
			return out, fmt.Errorf("importar requiere confirmación (confirm: true)")
		}
		path, err := e.validateImportPath(req.File)
		if err != nil {
			return out, err
		}
		t, err := e.dbTarget(req.Site)
		if err != nil {
			return out, err
		}
		if _, err := e.writeSnapshot(ctx, t, "before-import", true); err != nil {
			return out, fmt.Errorf("snapshot previo a importar: %w", err)
		}
		if err := e.restoreDump(ctx, t, path); err != nil {
			return out, err
		}
		out.Output = "base importada desde " + filepath.Base(path)
	case "export":
		t, err := e.dbTarget(req.Site)
		if err != nil {
			return out, err
		}
		path := req.File
		if path == "" {
			path = filepath.Join(e.snapshotDir(), snapshotID(t.Site, "export")+".sql")
			if err := os.MkdirAll(e.snapshotDir(), 0o700); err != nil {
				return out, err
			}
		}
		path, err = filepath.Abs(path)
		if err != nil {
			return out, err
		}
		if err := e.writeDump(ctx, t, path); err != nil {
			return out, err
		}
		out.Output = "exportado a " + path
	default:
		return out, fmt.Errorf("operación db desconocida %q", req.Op)
	}
	return out, nil
}

func (e *Engine) writeDump(ctx context.Context, t dbTarget, dest string) error {
	f, err := os.OpenFile(dest, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()
	cmd, err := t.dumpCmd(ctx)
	if err != nil {
		return err
	}
	cmd.Stdout = f
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func (e *Engine) restoreDump(ctx context.Context, t dbTarget, source string) error {
	f, err := os.Open(source)
	if err != nil {
		return err
	}
	defer f.Close()
	var cmd *exec.Cmd
	switch {
	case t.Driver == "sqlite":
		dest, err := os.OpenFile(t.SQLite, os.O_WRONLY|os.O_TRUNC, 0o600)
		if err != nil {
			return err
		}
		defer dest.Close()
		_, err = io.Copy(dest, f)
		return err
	case t.Driver == "pgsql" || t.Service == "postgres":
		args := append([]string{"--clean", "--if-exists", "--no-owner"}, t.hostArgs()...)
		cmd = exec.CommandContext(ctx, t.client("pg_restore"), args...)
	default:
		args := t.hostArgs()
		cmd = exec.CommandContext(ctx, t.client("mariadb"), args...)
	}
	cmd.Env = append(cmd.Environ(), t.env()...)
	cmd.Stdin = f
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func (t dbTarget) dumpCmd(ctx context.Context) (*exec.Cmd, error) {
	switch {
	case t.Driver == "sqlite":
		return nil, fmt.Errorf("export de SQLite: usa snapshots (copia del archivo)")
	case t.Driver == "pgsql" || t.Service == "postgres":
		args := append([]string{"--no-owner", "--no-privileges"}, t.hostArgs()...)
		cmd := exec.CommandContext(ctx, t.client("pg_dump"), args...)
		cmd.Env = append(cmd.Environ(), t.env()...)
		return cmd, nil
	default:
		tool := "mysqldump"
		if _, err := os.Stat(t.client("mariadb-dump")); err == nil {
			tool = "mariadb-dump"
		}
		args := append([]string{"--single-transaction", "--routines", "--triggers"}, t.hostArgs()...)
		cmd := exec.CommandContext(ctx, t.client(tool), args...)
		cmd.Env = append(cmd.Environ(), t.env()...)
		return cmd, nil
	}
}
