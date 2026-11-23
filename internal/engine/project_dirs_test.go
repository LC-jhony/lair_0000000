package engine

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lair-sh/lair/internal/config"
	"github.com/lair-sh/lair/internal/project"
)

func dirsFromResult(t *testing.T, r project.Result) []string {
	t.Helper()
	if len(r.Dirs) == 0 {
		t.Fatalf("no dirs in result: %+v", r)
	}
	return r.Dirs
}

func TestProjectDirsOps(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	e := newTestEngine(t)
	def := config.DefaultProjectsDir(home)

	res, err := e.ProjectControlWith(t.Context(), project.ControlRequest{Op: "dirs"})
	if err != nil {
		t.Fatal(err)
	}
	dirs := dirsFromResult(t, res)
	if len(dirs) != 1 || dirs[0] != def {
		t.Fatalf("default dirs = %v, want [%s]", dirs, def)
	}
	// The legacy alias must work as well as the documented dirs-list.
	alias, err := e.ProjectControlWith(t.Context(), project.ControlRequest{Op: "dirs-list"})
	if err != nil || len(alias.Dirs) != 1 || alias.Dirs[0] != def {
		t.Fatalf("dirs-list = %+v, err=%v", alias, err)
	}

	extra := filepath.Join(home, "Proyectos", "nueva")
	res, err = e.ProjectControlWith(t.Context(), project.ControlRequest{Op: "dirs-add", Dir: extra})
	if err != nil {
		t.Fatal(err)
	}
	dirs = dirsFromResult(t, res)
	if len(dirs) != 2 || dirs[1] != extra {
		t.Fatalf("dirs after add = %v", dirs)
	}
	if info, err := os.Stat(extra); err != nil || !info.IsDir() {
		t.Fatalf("dirs-add did not create %s: %v", extra, err)
	}
	// Adding the same folder twice is idempotent.
	res, err = e.ProjectControlWith(t.Context(), project.ControlRequest{Op: "dirs-add", Dir: extra})
	if err != nil || len(res.Dirs) != 2 {
		t.Fatalf("duplicate dir added: %v, err=%v", res.Dirs, err)
	}

	res, err = e.ProjectControlWith(t.Context(), project.ControlRequest{Op: "dirs-remove", Dir: extra})
	if err != nil {
		t.Fatal(err)
	}
	dirs = dirsFromResult(t, res)
	if len(dirs) != 1 || dirs[0] != def {
		t.Fatalf("dirs after remove = %v", dirs)
	}
	// Removing never deletes the folder itself.
	if info, err := os.Stat(extra); err != nil || !info.IsDir() {
		t.Fatalf("dirs-remove deleted the folder: %v", err)
	}
	if _, err := e.ProjectControlWith(t.Context(), project.ControlRequest{Op: "dirs-remove", Dir: def}); err == nil {
		t.Fatal("removing the last folder was accepted")
	}
	// The documented `path` payload key is accepted as an alias of `dir`.
	aliasRes, err := e.ProjectControlWith(t.Context(), project.ControlRequest{Op: "dirs-add", Path: extra})
	if err != nil || len(aliasRes.Dirs) != 2 {
		t.Fatalf("path alias was not applied: %v, err=%v", aliasRes.Dirs, err)
	}

	// The documented dirs-default alias sets the primary folder.
	primary := filepath.Join(home, "Proyectos", "principal")
	if err := os.MkdirAll(primary, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := e.ProjectControlWith(t.Context(), project.ControlRequest{Op: "dirs-default", Dir: primary}); err != nil {
		t.Fatal(err)
	}
	c, err := e.Config.View()
	if err != nil {
		t.Fatal(err)
	}
	if c.ProjectsDir != primary || c.ProjectDirs[0] != primary {
		t.Fatalf("dirs-default did not persist: %v / %s", c.ProjectDirs, c.ProjectsDir)
	}
}

func TestProjectDirsMigrate(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	e := newTestEngine(t)
	from := filepath.Join(home, "lair")
	to := filepath.Join(home, "Lair")
	blog := filepath.Join(from, "blog")
	if err := os.MkdirAll(blog, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(blog, "artisan"), []byte("#!/usr/bin/env php\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	envBody := "DB_CONNECTION=sqlite\nDB_DATABASE=" + filepath.Join(blog, "database", "database.sqlite") + "\n"
	if err := os.WriteFile(filepath.Join(blog, ".env"), []byte(envBody), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := e.Config.Update(func(c *config.Config) error {
		c.ProjectDirs = []string{from}
		c.ProjectsDir = from
		c.Sites["blog.test"] = config.Site{Path: blog, Enabled: true}
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	res, err := e.ProjectControlWith(t.Context(), project.ControlRequest{
		Op: "migrate-dirs", From: from, To: to,
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Path != to || !strings.Contains(res.Output, to) {
		t.Fatalf("migrate result = %+v", res)
	}
	if _, err := os.Stat(filepath.Join(to, "blog", "artisan")); err != nil {
		t.Fatalf("folder was not moved: %v", err)
	}
	if _, err := os.Stat(from); !os.IsNotExist(err) {
		t.Fatalf("old folder still exists: %v", err)
	}
	c, err := e.Config.View()
	if err != nil {
		t.Fatal(err)
	}
	if c.Sites["blog.test"].Path != filepath.Join(to, "blog") {
		t.Fatalf("site path not migrated: %s", c.Sites["blog.test"].Path)
	}
	if c.ProjectDirs[0] != to || c.ProjectsDir != to {
		t.Fatalf("project dirs not migrated: %v / %s", c.ProjectDirs, c.ProjectsDir)
	}
	envAfter, err := os.ReadFile(filepath.Join(to, "blog", ".env"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(envAfter), from) || !strings.Contains(string(envAfter), filepath.Join(to, "blog")) {
		t.Fatalf(".env not migrated: %s", envAfter)
	}
	// A migration into the (now existing) destination must be rejected.
	other := filepath.Join(home, "otro")
	if err := os.MkdirAll(other, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := e.ProjectControlWith(t.Context(), project.ControlRequest{
		Op: "migrate-dirs", From: other, To: to,
	}); err == nil {
		t.Fatal("existing destination accepted")
	}
	// LAIR_HOME is never a valid destination.
	if _, err := e.ProjectControlWith(t.Context(), project.ControlRequest{
		Op: "migrate-dirs", From: to, To: filepath.Join(e.Paths.Root, "inside"),
	}); err == nil || !strings.Contains(err.Error(), "LAIR_HOME") {
		t.Fatalf("LAIR_HOME destination accepted: %v", err)
	}
}

func TestProjectDirsMigrateDefaultsToLegacyFolder(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	e := newTestEngine(t)
	from := filepath.Join(home, config.LegacyProjectsDirName)
	if err := os.MkdirAll(filepath.Join(from, "demo"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(from, "demo", "artisan"), []byte("#!/usr/bin/env php\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	res, err := e.ProjectControlWith(t.Context(), project.ControlRequest{Op: "migrate-dirs"})
	if err != nil {
		t.Fatal(err)
	}
	if res.Path != config.DefaultProjectsDir(home) {
		t.Fatalf("migrated to %q", res.Path)
	}
	if _, err := os.Stat(filepath.Join(config.DefaultProjectsDir(home), "demo", "artisan")); err != nil {
		t.Fatalf("default migration did not move the folder: %v", err)
	}
}

func TestProjectDirsMigrateRollsBackOnConfigFailure(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	e := newTestEngine(t)
	from := filepath.Join(home, "old")
	to := filepath.Join(home, "new")
	if err := os.MkdirAll(from, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(from, "keep.txt"), []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}
	// A regular file where the config directory should be makes every
	// Config.Update fail after the folder was moved.
	blocker := filepath.Join(t.TempDir(), "blocker")
	if err := os.WriteFile(blocker, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	e.Config = config.NewStore(filepath.Join(blocker, "config.json"))

	if _, err := e.ProjectControlWith(t.Context(), project.ControlRequest{
		Op: "migrate-dirs", From: from, To: to,
	}); err == nil {
		t.Fatal("migration with a broken config store succeeded")
	}
	if _, err := os.Stat(filepath.Join(from, "keep.txt")); err != nil {
		t.Fatalf("rollback did not restore the folder: %v", err)
	}
	if _, err := os.Stat(to); !os.IsNotExist(err) {
		t.Fatalf("destination left behind after rollback: %v", err)
	}
}

func TestProjectImportValetReadOnly(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	e := newTestEngine(t)
	valet := filepath.Join(home, ".valet")
	if err := os.MkdirAll(filepath.Join(valet, "Sites"), 0o755); err != nil {
		t.Fatal(err)
	}
	cfgBody := `{"domain":"valet.test","paths":["` + filepath.Join(home, "Sites") + `"]}`
	cfgPath := filepath.Join(valet, "config.json")
	if err := os.WriteFile(cfgPath, []byte(cfgBody), 0o600); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(home, "Sites", "blog")
	if err := os.MkdirAll(target, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, filepath.Join(valet, "Sites", "blog")); err != nil {
		t.Fatal(err)
	}
	if err := e.Config.Update(func(c *config.Config) error {
		c.Sites["blog.valet.test"] = config.Site{Path: target, Enabled: true}
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	res, err := e.ProjectControlWith(t.Context(), project.ControlRequest{Op: "import-valet"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(res.Output, "blog.valet.test") || !strings.Contains(res.Output, "ya registrado") {
		t.Fatalf("import output = %q", res.Output)
	}
	found := false
	for _, dir := range res.Dirs {
		if dir == filepath.Join(home, "Sites") {
			found = true
		}
	}
	if !found {
		t.Fatalf("Valet paths were not imported: %v", res.Dirs)
	}
	c, err := e.Config.View()
	if err != nil {
		t.Fatal(err)
	}
	if c.Web.Domain != "valet.test" {
		t.Fatalf("domain not imported: %q", c.Web.Domain)
	}
	if len(c.Sites) != 1 {
		t.Fatalf("import modified the parked sites: %+v", c.Sites)
	}
	// ~/.valet is strictly read-only.
	after, err := os.ReadFile(cfgPath)
	if err != nil || string(after) != cfgBody {
		t.Fatalf("Valet config was modified: %v", err)
	}
	if _, err := os.Lstat(filepath.Join(valet, "Sites", "blog")); err != nil {
		t.Fatalf("Valet symlink was removed: %v", err)
	}
}
