package project

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lair-sh/lair/internal/app"
	"github.com/lair-sh/lair/internal/config"
)

func tempPaths(t *testing.T) app.Paths {
	t.Helper()
	root := t.TempDir()
	paths, err := app.Resolve()
	if err != nil {
		t.Fatal(err)
	}
	paths.Root = root
	paths.Tools = filepath.Join(root, "tools")
	paths.Bin = filepath.Join(root, "bin")
	paths.Services = filepath.Join(root, "services")
	for _, dir := range []string{paths.Root, paths.Tools, paths.Bin, paths.Services} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	return paths
}

func TestNormalize(t *testing.T) {
	parent := t.TempDir()
	valid := CreateRequest{
		Name: "Mi-App", Parent: parent, Stack: "react", Testing: "pest",
		NodeManager: "pnpm", Database: "mariadb", DBName: "mi_app_dev",
		DBUser: "dev", DBPassword: "secret", CreateDB: true, StartService: true,
	}
	target, err := valid.Normalize()
	if err != nil {
		t.Fatal(err)
	}
	if valid.Name != "mi-app" {
		t.Fatalf("name = %q", valid.Name)
	}
	if target != filepath.Join(parent, "mi-app") {
		t.Fatalf("target = %q", target)
	}
	if valid.DBHost != "127.0.0.1" || valid.DBPort != 3306 {
		t.Fatalf("defaults = %q %d", valid.DBHost, valid.DBPort)
	}

	bad := []CreateRequest{
		{Name: "Mi App", Parent: parent, Stack: "blade", Testing: "pest", NodeManager: "npm", Database: "sqlite"},
		{Name: "app", Parent: filepath.Join(parent, "missing"), Stack: "blade", Testing: "pest", NodeManager: "npm", Database: "sqlite"},
		{Name: "app", Parent: parent, Stack: "unknown", Testing: "pest", NodeManager: "npm", Database: "sqlite"},
		{Name: "app", Parent: parent, Stack: "blade", Testing: "pest", NodeManager: "npm", Database: "mysql", DBName: "bad-name", DBUser: "dev", DBPassword: "x"},
		{Name: "app", Parent: parent, Stack: "blade", Testing: "pest", NodeManager: "npm", Database: "mysql", DBName: "ok", DBUser: "", DBPassword: "x"},
		{Name: "app", Parent: parent, Stack: "blade", Testing: "pest", NodeManager: "npm", Database: "mysql", DBName: "ok", DBUser: "dev", DBPassword: ""},
	}
	for i := range bad {
		if _, err := bad[i].Normalize(); err == nil {
			t.Errorf("case %d should fail", i)
		}
	}
}

func TestExistingTargetRejected(t *testing.T) {
	parent := t.TempDir()
	name := "taken"
	if err := os.Mkdir(filepath.Join(parent, name), 0o755); err != nil {
		t.Fatal(err)
	}
	req := CreateRequest{Name: name, Parent: parent, Stack: "blade", Testing: "pest", NodeManager: "npm", Database: "sqlite"}
	if _, err := req.Normalize(); err == nil {
		t.Fatal("existing directory must be rejected")
	}
}

func TestLaravelArgsAndEnv(t *testing.T) {
	req := CreateRequest{
		Name: "blog", Stack: "livewire", Auth: false, Testing: "phpunit", Boost: true,
		NodeManager: "none", Git: true, Database: "pgsql",
		DBHost: "db.local", DBPort: 5432, DBName: "blog_dev", DBUser: "dev", DBPassword: "s3cret",
	}
	args := LaravelArgs(req)
	joined := strings.Join(args, " ")
	for _, want := range []string{
		"new blog", "--livewire", "--no-authentication", "--phpunit", "--boost",
		"--no-node", "--git", "--database=pgsql", "--no-interaction", "--no-ansi",
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("missing %q in %s", want, joined)
		}
	}
	if strings.Contains(joined, "s3cret") || strings.Contains(joined, "dev") {
		t.Fatalf("credentials leaked into argv: %s", joined)
	}
	env := strings.Join(EnvValues(req, "/home/dev"), "\n")
	if !strings.Contains(env, "DB_PASSWORD=s3cret") || !strings.Contains(env, "DB_PORT=5432") {
		t.Fatalf("missing DB env: %s", env)
	}
	if got := Redact("password=s3cret", "s3cret"); got != "password=***" {
		t.Fatalf("redact = %q", got)
	}
}

func TestPatchEnvFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".env")
	body := "# existing comment\nAPP_NAME=Lair\nAPP_URL=http://old\n\nDB_PASSWORD=old\n"
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	values := map[string]string{
		"APP_URL": "https://blog.test:8443", "DB_CONNECTION": "pgsql",
		"DB_PASSWORD": "new-secret", "DB_DATABASE": "blog_dev",
	}
	if err := PatchEnvFile(path, values); err != nil {
		t.Fatal(err)
	}
	out, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(out)
	for _, want := range []string{
		"# existing comment", "APP_NAME=Lair", "APP_URL=https://blog.test:8443",
		"DB_CONNECTION=pgsql", "DB_PASSWORD=new-secret", "DB_DATABASE=blog_dev",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("missing %q in %s", want, text)
		}
	}
	if strings.Count(text, "APP_URL=") != 1 || strings.Count(text, "DB_PASSWORD=") != 1 {
		t.Fatalf("duplicated keys in %s", text)
	}
}

func TestEnvFileValuesNeverLeaksPasswordToExample(t *testing.T) {
	req := CreateRequest{Database: "mysql", DBHost: "127.0.0.1", DBPort: 3306, DBName: "app_dev", DBUser: "dev", DBPassword: "s3cret"}
	if got := EnvFileValues(req, "http://app.test", "/home/dev", true)["DB_PASSWORD"]; got != "" {
		t.Fatalf("example password = %q", got)
	}
	if got := EnvFileValues(req, "http://app.test", "/home/dev", false)["DB_PASSWORD"]; got != "s3cret" {
		t.Fatalf("env password = %q", got)
	}
}

type stubResolver struct{ inv Invocation }

func (r stubResolver) ResolveTool(string, string) (Invocation, error) { return r.inv, nil }

func TestEnsureInstallerReportsCommandOnFailure(t *testing.T) {
	paths := tempPaths(t)
	t.Setenv("LAIR_LARAVEL_BIN", "")
	fakeComposer := filepath.Join(t.TempDir(), "composer")
	if err := os.WriteFile(fakeComposer, []byte("#!/bin/sh\nexit 1\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	logs := []string{}
	_, err := EnsureInstaller(context.Background(), stubResolver{inv: Invocation{Path: fakeComposer}}, paths,
		func(line string) { logs = append(logs, line) })
	if err == nil {
		t.Fatal("expected composer failure")
	}
	if !strings.Contains(err.Error(), InstallCommand) {
		t.Fatalf("error = %q", err)
	}
	if !strings.Contains(strings.Join(logs, "\n"), "Comando: "+InstallCommand) {
		t.Fatalf("logs = %v", logs)
	}
}

func TestEnvValuesSQLiteUsesAbsolutePath(t *testing.T) {
	req := CreateRequest{Name: "blog", Database: "sqlite"}
	env := strings.Join(EnvValues(req, "/home/dev/lair"), "\n")
	if !strings.Contains(env, "DB_DATABASE=/home/dev/lair/blog/database/database.sqlite") {
		t.Fatalf("env = %s", env)
	}
}

func TestEnvFileValuesSQLiteUsesAbsolutePath(t *testing.T) {
	req := CreateRequest{Name: "blog", Database: "sqlite"}
	values := EnvFileValues(req, "http://blog.test:8080", "/home/dev/lair", false)
	want := filepath.Join("/home/dev/lair", "blog", "database", "database.sqlite")
	if values["DB_DATABASE"] != want {
		t.Fatalf("DB_DATABASE = %q, want %q", values["DB_DATABASE"], want)
	}
}

func TestAppURLAndHelpFlags(t *testing.T) {
	web := config.DefaultWeb()
	if web.HTTPPort != 80 || web.HTTPSPort != 443 || !web.HTTPS || !web.RedirectHTTP ||
		web.Bind != "127.0.0.1" || web.Domain != "test" {
		t.Fatalf("default web = %+v", web)
	}
	if got := AppURL("blog", "test", web, true); got != "https://blog.test" {
		t.Fatalf("secure url = %q", got)
	}
	web.HTTPS = false
	if got := AppURL("blog", "test", web, true); got != "http://blog.test" {
		t.Fatalf("fallback url = %q", got)
	}
	flags := helpFlags("Usage: laravel new\n  --react    React\n  --no-authentication\n  --database[=DB]")
	if len(flags) != 3 || flags[0] != "--react" || flags[2] != "--database[=DB]" {
		t.Fatalf("flags = %v", flags)
	}
}
