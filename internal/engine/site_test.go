package engine

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// linkTestSite registers a real project directory. The generated per-site
// files are still written to disk.
func linkTestSite(t *testing.T, e *Engine, name string) string {
	t.Helper()
	dir := t.TempDir()
	if err := e.Link(name, dir); err != nil {
		t.Fatal(err)
	}
	return dir
}

func writeTestFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

func findEnvEntry(entries []SiteEnvEntry, key string) (SiteEnvEntry, bool) {
	for _, entry := range entries {
		if entry.Key == key {
			return entry, true
		}
	}
	return SiteEnvEntry{}, false
}

func TestSiteControlDetail(t *testing.T) {
	e, _ := testEnv(t)
	dir := linkTestSite(t, e, "blog.test")
	writeTestFile(t, filepath.Join(dir, "public", "index.php"), "<?php // front controller\n")
	writeTestFile(t, filepath.Join(dir, ".env"), strings.Join([]string{
		"APP_URL=https://blog.test:8443",
		"DB_CONNECTION=mysql",
		"DB_HOST=127.0.0.1",
		"DB_PORT=3307",
		"DB_DATABASE=blog",
		"DB_PASSWORD=super-secret",
	}, "\n")+"\n")
	writeTestFile(t, filepath.Join(dir, ".lairrc"), "php=8.3\n")
	writeTestFile(t, filepath.Join(dir, ".nvmrc"), "22.11.0\n")
	writeTestFile(t, filepath.Join(dir, "composer.json"), `{"require":{"laravel/framework":"^12.0"}}`)

	res, err := e.SiteControl(context.Background(), SiteRequest{
		Op: "domains", Name: "blog.test", Domains: []string{"tenant.blog.test"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Domains) != 1 || res.Domains[0] != "tenant.blog.test" {
		t.Fatalf("domains = %v", res.Domains)
	}

	res, err = e.SiteControl(context.Background(), SiteRequest{Op: "detail", Name: "blog.test"})
	if err != nil {
		t.Fatal(err)
	}
	d := res.Detail
	if d == nil {
		t.Fatal("detail is nil")
	}
	checks := map[string]bool{
		"name":        d.Name == "blog.test",
		"path":        d.Path == dir,
		"url":         d.URL == "https://blog.test:8443",
		"type":        d.Type == "laravel",
		"framework":   d.Framework == "laravel",
		"php":         d.PHP == "8.3",
		"php_source":  d.PHPSource == ".lairrc",
		"php_file":    d.PHPFile == ".lairrc",
		"node":        d.Node == "22.11.0",
		"node_source": d.NodeSource == ".nvmrc",
		"node_file":   d.NodeFile == ".nvmrc",
		"database":    d.Database == "mysql",
		"db_host":     d.DatabaseHost == "127.0.0.1",
		"db_port":     d.DatabasePort == 3307,
		"db_name":     d.DatabaseName == "blog",
		"root_state":  d.RootState == "ok",
		"root":        d.Root == filepath.Join(dir, "public"),
		"secure":      d.Secure,
		"domains":     len(d.Domains) == 1 && d.Domains[0] == "tenant.blog.test",
		"no secrets":  !strings.Contains(mustJSON(t, d), "super-secret"),
	}
	for name, ok := range checks {
		if !ok {
			t.Errorf("detail %s failed: %+v", name, d)
		}
	}

	// The status snapshot exposes the same tolerant fields in web.Site.
	sites := e.Web.Router.List()
	if len(sites) != 1 {
		t.Fatalf("sites = %d", len(sites))
	}
	s := sites[0]
	if s.RootState != "ok" || s.Type != "laravel" || len(s.Domains) != 1 || s.Domains[0] != "tenant.blog.test" {
		t.Fatalf("router site = %+v", s)
	}
	if s.Name != "blog.test" || s.Root != dir || s.PHPSocket == "" || !s.Secure {
		t.Fatalf("routing fields changed: %+v", s)
	}
}

func mustJSON(t *testing.T, v any) string {
	t.Helper()
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestSiteControlEnvMasksSecrets(t *testing.T) {
	e, _ := testEnv(t)
	dir := linkTestSite(t, e, "env.test")
	writeTestFile(t, filepath.Join(dir, ".env"), strings.Join([]string{
		"APP_URL=http://env.test:8080",
		"DB_PASSWORD=super-secret",
		"APP_KEY=base64:abcdef",
		"API_TOKEN=abcd1234",
		"MY_SECRET=unique-secret-999",
		"EMPTY_KEY=",
	}, "\n")+"\n")

	res, err := e.SiteControl(context.Background(), SiteRequest{Op: "env", Name: "env.test"})
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]SiteEnvEntry{}
	for _, entry := range res.Env {
		got[entry.Key] = entry
	}
	appURL, ok := findEnvEntry(res.Env, "APP_URL")
	if !ok || appURL.Value != "http://env.test:8080" || appURL.Secret || !appURL.Set {
		t.Fatalf("APP_URL = %+v", appURL)
	}
	for _, key := range []string{"DB_PASSWORD", "APP_KEY", "API_TOKEN", "MY_SECRET", "EMPTY_KEY"} {
		entry, ok := got[key]
		if !ok {
			t.Fatalf("missing secret entry %s", key)
		}
		if !entry.Secret || !entry.Set || entry.Value != "" {
			t.Fatalf("secret %s was not masked: %+v", key, entry)
		}
	}
	encoded := mustJSON(t, res)
	for _, secret := range []string{"super-secret", "base64:abcdef", "abcd1234", "unique-secret-999"} {
		if strings.Contains(encoded, secret) {
			t.Fatalf("env response leaked %q: %s", secret, encoded)
		}
	}
}

func TestSiteControlEnvSet(t *testing.T) {
	e, _ := testEnv(t)
	dir := linkTestSite(t, e, "set.test")
	envPath := filepath.Join(dir, ".env")
	writeTestFile(t, envPath, "APP_URL=http://old.test:8080\nDB_PASSWORD=keep-me\n")

	res, err := e.SiteControl(context.Background(), SiteRequest{
		Op: "env-set", Name: "set.test", Key: "APP_URL", Value: "https://new.test:8443",
	})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(res.Output, "new.test") {
		t.Fatalf("output leaked the value: %q", res.Output)
	}
	if entry, ok := findEnvEntry(res.Env, "DB_PASSWORD"); !ok || entry.Value != "" || !entry.Secret {
		t.Fatalf("env-set response leaked or unmasked the password: %+v", res.Env)
	}
	body, err := os.ReadFile(envPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), "APP_URL=https://new.test:8443") || !strings.Contains(string(body), "DB_PASSWORD=keep-me") {
		t.Fatalf("unexpected .env:\n%s", body)
	}

	if _, err := e.SiteControl(context.Background(), SiteRequest{
		Op: "env-set", Name: "set.test", Key: "BAD KEY", Value: "x",
	}); !errors.Is(err, ErrInvalidEnvKey) {
		t.Fatalf("invalid key error = %v", err)
	}
	if _, err := e.SiteControl(context.Background(), SiteRequest{
		Op: "env-set", Name: "set.test", Key: "APP_URL", Value: "ok\nINJECTED=1",
	}); !errors.Is(err, ErrInvalidEnvKey) {
		t.Fatalf("multiline value error = %v", err)
	}
}

func TestSiteControlDomainsRegeneratesVhost(t *testing.T) {
	e, _ := testEnv(t)
	dir := linkTestSite(t, e, "sites.test")
	_ = dir
	if _, err := e.SiteControl(context.Background(), SiteRequest{
		Op: "domains", Name: "sites.test", Domains: []string{"a.sites.test", "b.sites.test", "a.sites.test"},
	}); err != nil {
		t.Fatal(err)
	}
	config, err := e.Config.View()
	if err != nil {
		t.Fatal(err)
	}
	domains := config.Sites["sites.test"].Domains
	if len(domains) != 2 || domains[0] != "a.sites.test" || domains[1] != "b.sites.test" {
		t.Fatalf("persisted domains = %v", domains)
	}
	vhost := filepath.Join(e.Paths.Root, "web", "nginx", "sites", "sites.test.conf")
	body, err := os.ReadFile(vhost)
	if err != nil {
		t.Fatal(err)
	}
	want := "server_name sites.test www.sites.test a.sites.test b.sites.test;"
	if !strings.Contains(string(body), want) {
		t.Fatalf("vhost missing aliases (%s):\n%s", want, body)
	}

	// An omitted field is a read; an explicit empty list clears the aliases.
	read, err := e.SiteControl(context.Background(), SiteRequest{Op: "domains", Name: "sites.test"})
	if err != nil || len(read.Domains) != 2 {
		t.Fatalf("domains read = %+v, %v", read, err)
	}
	if _, err := e.SiteControl(context.Background(), SiteRequest{
		Op: "domains", Name: "sites.test", Domains: []string{},
	}); err != nil {
		t.Fatal(err)
	}
	body, _ = os.ReadFile(vhost)
	if strings.Contains(string(body), "a.sites.test") {
		t.Fatalf("aliases survived clearing:\n%s", body)
	}
	if _, err := e.SiteControl(context.Background(), SiteRequest{
		Op: "domains", Name: "sites.test", Domains: []string{"bad domain"},
	}); err == nil {
		t.Fatal("invalid domain accepted")
	}
}

func TestSiteControlSetEnabledRegeneratesVhost(t *testing.T) {
	e, _ := testEnv(t)
	dir := linkTestSite(t, e, "toggle.test")
	writeTestFile(t, filepath.Join(dir, "public", "index.php"), "<?php\n")
	vhost := filepath.Join(e.Paths.Root, "web", "nginx", "sites", "toggle.test.conf")
	if _, err := os.Stat(vhost); err != nil {
		t.Fatalf("linked site has no vhost: %v", err)
	}

	detail, err := e.SiteControl(context.Background(), SiteRequest{Op: "detail", Name: "toggle.test"})
	if err != nil || !detail.Detail.Enabled {
		t.Fatalf("new site should be enabled: %+v, %v", detail, err)
	}

	off := false
	res, err := e.SiteControl(context.Background(), SiteRequest{
		Op: "set-enabled", Name: "toggle.test", Enabled: &off,
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Detail == nil || res.Detail.Enabled {
		t.Fatalf("set-enabled did not refresh the detail: %+v", res)
	}
	if _, err := os.Stat(vhost); !os.IsNotExist(err) {
		t.Fatalf("disabled site's vhost survived: %v", err)
	}
	c, _ := e.Config.View()
	if c.Sites["toggle.test"].Enabled {
		t.Fatal("Enabled=false was not persisted")
	}
	for _, site := range e.Web.Router.List() {
		if site.Name == "toggle.test" && site.Enabled {
			t.Fatal("status snapshot still reports the site as enabled")
		}
	}

	// The vhost preview still works while the site is disabled.
	preview, err := e.SiteControl(context.Background(), SiteRequest{Op: "config", Name: "toggle.test"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(preview.Config, "server {") || !strings.Contains(preview.Config, "toggle.test") {
		t.Fatalf("disabled vhost preview = %q", preview.Config)
	}
	if _, err := os.Stat(vhost); !os.IsNotExist(err) {
		t.Fatal("the config preview wrote a vhost to disk")
	}

	on := true
	res, err = e.SiteControl(context.Background(), SiteRequest{
		Op: "set-enabled", Name: "toggle.test", Enabled: &on,
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Detail == nil || !res.Detail.Enabled {
		t.Fatalf("re-enable did not refresh the detail: %+v", res)
	}
	if _, err := os.Stat(vhost); err != nil {
		t.Fatalf("re-enabled site has no vhost: %v", err)
	}

	// A missing enabled field is not an implicit enable.
	if _, err := e.SiteControl(context.Background(), SiteRequest{Op: "set-enabled", Name: "toggle.test"}); err == nil {
		t.Fatal("set-enabled without the enabled field was accepted")
	}
	if _, err := e.SiteControl(context.Background(), SiteRequest{Op: "config", Name: "missing.test"}); !errors.Is(err, ErrSiteNotFound) {
		t.Fatalf("config for missing site = %v", err)
	}
}

func TestSiteControlOpenChecksDNSAndRefreshesAppURL(t *testing.T) {
	e, _ := testEnv(t)
	dir := linkTestSite(t, e, "localhost.localdomain")
	writeTestFile(t, filepath.Join(dir, ".env"), "APP_URL=http://old.example\n")

	res, err := e.SiteControl(context.Background(), SiteRequest{Op: "open", Name: "localhost.localdomain"})
	if err != nil {
		t.Fatal(err)
	}
	if res.URL != "http://localhost.localdomain" || res.Output != res.URL {
		t.Fatalf("open result = %+v", res)
	}
	env, err := os.ReadFile(filepath.Join(dir, ".env"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(env), "APP_URL=http://localhost.localdomain") {
		t.Fatalf("APP_URL not refreshed: %s", env)
	}
}

func TestSiteControlOpenReportsDNSRepairCommand(t *testing.T) {
	e, _ := testEnv(t)
	_ = linkTestSite(t, e, "missing.lair.invalid")
	_, err := e.SiteControl(context.Background(), SiteRequest{Op: "open", Name: "missing.lair.invalid"})
	if err == nil {
		t.Fatal("open accepted a host that does not resolve")
	}
	for _, want := range []string{"setup-privileged.sh", "getent hosts missing.lair.invalid"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error = %q, missing %q", err, want)
		}
	}
}

func TestSiteControlConfigServesStatusPageForEmptyRoot(t *testing.T) {
	e, _ := testEnv(t)
	_ = linkTestSite(t, e, "empty.test") // no front controller on purpose
	vhost := filepath.Join(e.Paths.Root, "web", "nginx", "sites", "empty.test.conf")
	before, err := os.ReadFile(vhost)
	if err != nil {
		t.Fatal(err)
	}

	res, err := e.SiteControl(context.Background(), SiteRequest{Op: "config", Name: "empty.test"})
	if err != nil {
		t.Fatal(err)
	}
	if res.Detail != nil || res.Output != "" || res.Env != nil || res.Domains != nil {
		t.Fatalf("config op carried unrelated fields: %+v", res)
	}
	for _, want := range []string{"server {", "not-configured.html", "try_files $uri =404;", "pages"} {
		if !strings.Contains(res.Config, want) {
			t.Fatalf("vhost preview missing %q:\n%s", want, res.Config)
		}
	}
	if strings.Contains(res.Config, "fastcgi_pass") {
		t.Fatalf("empty root preview still proxies to PHP-FPM:\n%s", res.Config)
	}
	after, _ := os.ReadFile(vhost)
	if string(before) != string(after) {
		t.Fatal("config op rewrote the vhost on disk")
	}

	detail, err := e.SiteControl(context.Background(), SiteRequest{Op: "detail", Name: "empty.test"})
	if err != nil || detail.Detail.RootState != "empty" {
		t.Fatalf("empty root detail = %+v, %v", detail, err)
	}
}

func TestSiteVHostsFillsEnabledAndRootState(t *testing.T) {
	e, _ := testEnv(t)
	okDir := linkTestSite(t, e, "ok.test")
	writeTestFile(t, filepath.Join(okDir, "public", "index.php"), "<?php\n")
	missingDir := linkTestSite(t, e, "missing.test")
	writeTestFile(t, filepath.Join(missingDir, "README.md"), "readme")
	emptyDir := linkTestSite(t, e, "empty.test")
	_ = emptyDir

	off := false
	if _, err := e.SiteControl(context.Background(), SiteRequest{
		Op: "set-enabled", Name: "missing.test", Enabled: &off,
	}); err != nil {
		t.Fatal(err)
	}

	sites, err := e.siteVHosts(e.WebSettings())
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]struct {
		enabled bool
		state   string
	}{}
	for _, site := range sites {
		got[site.Name] = struct {
			enabled bool
			state   string
		}{site.Enabled, site.RootState}
	}
	for name, want := range map[string]struct {
		enabled bool
		state   string
	}{
		"ok.test":      {true, "ok"},
		"missing.test": {false, "missing"},
		"empty.test":   {true, "empty"},
	} {
		if value, ok := got[name]; !ok || value != want {
			t.Fatalf("site vhost %s = %+v, want %+v (all: %+v)", name, value, want, got)
		}
	}
}

func TestSiteControlDelete(t *testing.T) {
	e, _ := testEnv(t)

	// delete_files=false keeps the project on disk.
	dir := linkTestSite(t, e, "keep.test")
	res, err := e.SiteControl(context.Background(), SiteRequest{Op: "delete", Name: "keep.test"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(res.Output, "keep.test") {
		t.Fatalf("delete output = %q", res.Output)
	}
	if _, err := os.Stat(dir); err != nil {
		t.Fatalf("project files were removed without delete_files: %v", err)
	}
	if c, _ := e.Config.View(); c.Sites != nil && len(c.Sites) != 0 {
		t.Fatalf("site was not unlinked: %+v", c.Sites)
	}

	// delete_files=true removes the registered project directory.
	dir = linkTestSite(t, e, "gone.test")
	writeTestFile(t, filepath.Join(dir, "public", "index.php"), "<?php\n")
	if _, err := e.SiteControl(context.Background(), SiteRequest{
		Op: "delete", Name: "gone.test", DeleteFiles: true,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatalf("project files were not removed: %v", err)
	}

	// A registered path inside LAIR_HOME is never deleted.
	inside := filepath.Join(e.Paths.Root, "projects", "inner.test")
	writeTestFile(t, filepath.Join(inside, "public", "index.php"), "<?php\n")
	if err := e.Link("inner.test", inside); err != nil {
		t.Fatal(err)
	}
	if _, err := e.SiteControl(context.Background(), SiteRequest{
		Op: "delete", Name: "inner.test", DeleteFiles: true,
	}); !errors.Is(err, ErrUnsafeSitePath) {
		t.Fatalf("delete inside LAIR_HOME error = %v", err)
	}
	if _, err := os.Stat(inside); err != nil {
		t.Fatalf("site inside LAIR_HOME was deleted: %v", err)
	}

	// Symlinked project roots are refused too.
	real := t.TempDir()
	link := filepath.Join(t.TempDir(), "linked.test")
	if err := os.Symlink(real, link); err != nil {
		t.Skipf("symlinks not supported: %v", err)
	}
	if err := e.Link("linked.test", link); err != nil {
		t.Fatal(err)
	}
	if _, err := e.SiteControl(context.Background(), SiteRequest{
		Op: "delete", Name: "linked.test", DeleteFiles: true,
	}); !errors.Is(err, ErrUnsafeSitePath) {
		t.Fatalf("symlink delete error = %v", err)
	}
	if _, err := os.Stat(real); err != nil {
		t.Fatalf("symlink target was deleted: %v", err)
	}
}

func TestSiteControlValidation(t *testing.T) {
	e, _ := testEnv(t)
	for _, req := range []SiteRequest{
		{Op: "detail", Name: "../../etc"},
		{Op: "detail", Name: "missing.test"},
		{Op: "bogus", Name: "any.test"},
	} {
		if _, err := e.SiteControl(context.Background(), req); err == nil {
			t.Fatalf("request %+v accepted", req)
		}
	}
	if _, err := e.SiteControl(context.Background(), SiteRequest{Op: "detail", Name: "missing.test"}); !errors.Is(err, ErrSiteNotFound) {
		t.Fatalf("missing site error = %v", err)
	}
}

func TestDetectSiteRoot(t *testing.T) {
	public := filepath.Join(t.TempDir(), "site")
	writeTestFile(t, filepath.Join(public, "public", "index.php"), "<?php\n")
	if got := detectSiteRoot(public); got.state != "ok" || got.root != filepath.Join(public, "public") {
		t.Fatalf("public index = %+v", got)
	}

	flat := t.TempDir()
	writeTestFile(t, filepath.Join(flat, "index.php"), "<?php\n")
	got := detectSiteRoot(flat)
	if got.state != "ok" || got.root != flat || got.notice == "" || len(got.suggestions) == 0 {
		t.Fatalf("root index = %+v", got)
	}

	empty := t.TempDir()
	if got := detectSiteRoot(empty); got.state != "empty" || len(got.suggestions) == 0 {
		t.Fatalf("empty = %+v", got)
	}

	files := t.TempDir()
	writeTestFile(t, filepath.Join(files, "README.md"), "hi")
	if got := detectSiteRoot(files); got.state != "missing" || got.problem == "" {
		t.Fatalf("missing = %+v", got)
	}

	if got := detectSiteRoot(filepath.Join(t.TempDir(), "does-not-exist")); got.state != "missing" {
		t.Fatalf("missing path = %+v", got)
	}
}

func TestSiteRootDetailsWithoutIndex(t *testing.T) {
	e, _ := testEnv(t)
	dir := linkTestSite(t, e, "broken.test")
	res, err := e.SiteControl(context.Background(), SiteRequest{Op: "detail", Name: "broken.test"})
	if err != nil {
		t.Fatal(err)
	}
	if res.Detail.RootState != "empty" || len(res.Detail.RootSuggestions) == 0 {
		t.Fatalf("empty site detail = %+v", res.Detail)
	}
	_ = dir
}
