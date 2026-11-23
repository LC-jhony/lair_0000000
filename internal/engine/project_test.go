package engine

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/lair-sh/lair/internal/app"
	"github.com/lair-sh/lair/internal/config"
	"github.com/lair-sh/lair/internal/project"
)

func newTestEngine(t *testing.T) *Engine {
	t.Helper()
	paths, err := app.Resolve()
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	paths.Root = root
	paths.Tools = filepath.Join(root, "tools")
	paths.Log = filepath.Join(root, "log")
	paths.Certs = filepath.Join(root, "certs")
	paths.Bin = filepath.Join(root, "bin")
	paths.Services = filepath.Join(root, "services")
	paths.ConfigFile = filepath.Join(root, "config.json")
	for _, dir := range []string{root, paths.Tools, paths.Log, paths.Certs, paths.Bin, paths.Services} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	e, err := New(paths, nil)
	if err != nil {
		t.Fatal(err)
	}
	return e
}

func fakeLaravel(t *testing.T, e *Engine) {
	t.Helper()
	binDir := filepath.Join(e.Paths.Tools, "laravel-global", "vendor", "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(binDir, "laravel")
	script := `#!/bin/sh
case "$1" in
  --version) echo "Laravel Installer 5.9.0"; exit 0;;
  new)
    shift
    if [ "$1" = "--help" ]; then
      echo "Usage: laravel new [options]"
      echo "  --react   Use React"
      echo "  --pest    Use Pest"
      exit 0
    fi
    name="$1"
    shift
    echo "fake laravel new $name $*"
    mkdir -p "$name/database"
    : > "$name/database/database.sqlite"
    printf 'APP_NAME=Lair\nAPP_URL=http://old\nDB_PASSWORD=old\n' > "$name/.env"
    printf 'APP_NAME=Lair\nAPP_URL=http://old\nDB_PASSWORD=\n' > "$name/.env.example"
    exit 0;;
esac
exit 0
`
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("LAIR_LARAVEL_BIN", bin)
}

func waitProject(t *testing.T, e *Engine, id string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		st, ok := e.Installer.JobStatus(id)
		if ok && st.Done {
			if st.Error != "" {
				t.Fatalf("job failed: %s", st.Error)
			}
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("job did not finish")
}

func TestProjectCreate(t *testing.T) {
	e := newTestEngine(t)
	fakeLaravel(t, e)
	parent := t.TempDir()
	result, err := e.ProjectControl(t.Context(), "info", "", project.CreateRequest{})
	if err != nil || !result.Info.LaravelInstalled {
		t.Fatalf("info = %+v, err = %v", result.Info, err)
	}
	if len(result.Info.InstallerFlags) == 0 {
		t.Fatal("installer flags were not detected")
	}

	result, err = e.ProjectControl(t.Context(), "create", "", project.CreateRequest{
		Name: "blog", Parent: parent, Stack: "blade", Testing: "pest",
		NodeManager: "npm", Database: "sqlite", Secure: false,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Site != "blog.test" || result.ID == "" {
		t.Fatalf("result = %+v", result)
	}
	waitProject(t, e, result.ID)

	target := filepath.Join(parent, "blog")
	env, err := os.ReadFile(filepath.Join(target, ".env"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(env), "APP_URL=http://blog.test") || !strings.Contains(string(env), "DB_CONNECTION=sqlite") {
		t.Fatalf("env = %s", env)
	}
	c, err := e.Config.View()
	if err != nil {
		t.Fatal(err)
	}
	site, ok := c.Sites["blog.test"]
	if !ok || site.Path != target {
		t.Fatalf("sites = %+v", c.Sites)
	}
	logData, err := os.ReadFile(filepath.Join(e.Paths.Log, result.ID+"-install.log"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(logData), "laravel new blog") || !strings.Contains(string(logData), "Proyecto listo") {
		t.Fatalf("log = %s", logData)
	}
}

func TestProjectCreateRemovesIncompleteFolder(t *testing.T) {
	e := newTestEngine(t)
	binDir := filepath.Join(e.Paths.Tools, "laravel-global", "vendor", "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(binDir, "laravel")
	if err := os.WriteFile(bin, []byte("#!/bin/sh\nif [ \"$1\" = \"new\" ]; then mkdir -p \"$2\"; exit 7; fi\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("LAIR_LARAVEL_BIN", bin)
	parent := t.TempDir()
	result, err := e.ProjectControl(t.Context(), "create", "", project.CreateRequest{
		Name: "empty", Parent: parent, Stack: "blade", Testing: "pest",
		NodeManager: "npm", Database: "sqlite",
	})
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if st, ok := e.Installer.JobStatus(result.ID); ok && st.Done {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if _, err := os.Stat(filepath.Join(parent, "empty")); !os.IsNotExist(err) {
		t.Fatalf("incomplete folder still exists: %v", err)
	}
}

func TestProjectDefaultParentIsHomeLair(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	e := newTestEngine(t)
	fakeLaravel(t, e)

	result, err := e.ProjectControl(t.Context(), "create", "", project.CreateRequest{
		Name: "default-parent", Stack: "blade", Testing: "pest",
		NodeManager: "npm", Database: "sqlite",
	})
	if err != nil {
		t.Fatal(err)
	}
	want := config.DefaultProjectsDir(home)
	if result.Path != filepath.Join(want, "default-parent") {
		t.Fatalf("path = %q, want under %q", result.Path, want)
	}
	waitProject(t, e, result.ID)
}

func TestProjectRejectsLairHomeParent(t *testing.T) {
	e := newTestEngine(t)
	_, err := e.ProjectControl(t.Context(), "create", "", project.CreateRequest{
		Name: "inside", Parent: e.Paths.Root, Stack: "blade", Testing: "pest",
		NodeManager: "npm", Database: "sqlite",
	})
	if err == nil || !strings.Contains(err.Error(), "LAIR_HOME") {
		t.Fatalf("err = %v", err)
	}
}

func TestSyncProjectDirsDiscoversLaravelProjects(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	e := newTestEngine(t)
	projects := config.DefaultProjectsDir(home)
	blog := filepath.Join(projects, "blog")
	if err := os.MkdirAll(blog, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(blog, "artisan"), []byte("#!/usr/bin/env php\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// A folder without a Laravel marker must be ignored.
	if err := os.Mkdir(filepath.Join(projects, "not-a-project"), 0o755); err != nil {
		t.Fatal(err)
	}

	if err := e.SyncProjectDirs(); err != nil {
		t.Fatal(err)
	}
	c, err := e.Config.View()
	if err != nil {
		t.Fatal(err)
	}
	site, ok := c.Sites["blog.test"]
	if !ok || site.Path != blog {
		t.Fatalf("sites = %+v", c.Sites)
	}
	if _, ok := c.Sites["not-a-project.test"]; ok {
		t.Fatal("non-project folder was registered")
	}
}

func TestProjectSetDir(t *testing.T) {
	e := newTestEngine(t)
	dir := t.TempDir()
	result, err := e.ProjectControl(t.Context(), "set-dir", dir, project.CreateRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if result.Path != dir {
		t.Fatalf("path = %q, want %q", result.Path, dir)
	}
	info := e.ProjectInfo()
	if info.ProjectsDir != dir {
		t.Fatalf("projects dir = %q, want %q", info.ProjectsDir, dir)
	}
}
