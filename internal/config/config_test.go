package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNewAndLegacyDefaults(t *testing.T) {
	c := New()
	if c.Mail.Port != DefaultMailPort {
		t.Fatalf("mail port = %d", c.Mail.Port)
	}
	if !c.Dumps.Configured || !c.Dumps.Queries || !c.Dumps.Logs || !c.Dumps.Views ||
		!c.Dumps.Jobs || !c.Dumps.Requests || c.Dumps.ShowInPage {
		t.Fatalf("dumps defaults = %+v", c.Dumps)
	}
	if c.Debugger.Mode != "debug" || c.Debugger.Detect || c.Debugger.Breakpoints {
		t.Fatalf("debugger defaults = %+v", c.Debugger)
	}

	path := filepath.Join(t.TempDir(), "config.json")
	// A config.json written before mail/dumps/debugger existed.
	legacy := `{"version":1,"installed":{},"defaults":{},"ports":{},"sites":{},` +
		`"web":{"http_port":8080,"https_port":8443,"https":true}}`
	if err := os.WriteFile(path, []byte(legacy), 0o600); err != nil {
		t.Fatal(err)
	}
	loaded, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Mail.Port != 2525 || !loaded.Dumps.Queries || loaded.Debugger.Mode != "debug" {
		t.Fatalf("legacy defaults = %+v %+v %+v", loaded.Mail, loaded.Dumps, loaded.Debugger)
	}
}

func TestDefaultProjectsDirAndProjectDirs(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	if got := DefaultProjectsDir(home); got != filepath.Join(home, "Lair") {
		t.Fatalf("DefaultProjectsDir = %q", got)
	}
	if got := DefaultProjectsDir(""); got != filepath.Join(home, "Lair") {
		t.Fatalf("DefaultProjectsDir(\"\") = %q", got)
	}

	c := New()
	if len(c.ProjectDirs) != 1 || c.ProjectDirs[0] != filepath.Join(home, "Lair") {
		t.Fatalf("ProjectDirs = %v", c.ProjectDirs)
	}
	if c.ProjectsDir != c.ProjectDirs[0] {
		t.Fatalf("ProjectsDir = %q, ProjectDirs = %v", c.ProjectsDir, c.ProjectDirs)
	}

	// A new config is written with both keys so old clients stay compatible.
	path := filepath.Join(t.TempDir(), "config.json")
	s := NewStore(path)
	if err := s.Update(func(c *Config) error { return nil }); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"project_dirs"`) || !strings.Contains(string(data), `"projects_dir"`) {
		t.Fatalf("config does not carry both project-dir keys:\n%s", data)
	}
}

func TestLoadMigratesLegacyProjectDirs(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	path := filepath.Join(t.TempDir(), "config.json")
	legacy := filepath.Join(home, "lair")
	body := `{"version":1,"projects_dir":"` + legacy + `","web":{"http_port":80,"https_port":443}}`
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	c, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(home, "Lair")
	if len(c.ProjectDirs) != 1 || c.ProjectDirs[0] != want {
		t.Fatalf("ProjectDirs = %v, want %v", c.ProjectDirs, want)
	}
	if c.ProjectsDir != want {
		t.Fatalf("ProjectsDir = %q, want %q", c.ProjectsDir, want)
	}
	// The old folder must not be deleted by a config migration.
	if _, err := os.Stat(legacy); err != nil && !os.IsNotExist(err) {
		t.Fatalf("migration touched the old folder: %v", err)
	}

	// An explicit custom folder is preserved as the primary entry.
	if err := NewStore(path).Update(func(c *Config) error {
		c.ProjectsDir = "/srv/apps"
		c.ProjectDirs = nil
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	c, err = Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if c.ProjectDirs[0] != "/srv/apps" || c.ProjectsDir != "/srv/apps" {
		t.Fatalf("custom dir lost: %v / %q", c.ProjectDirs, c.ProjectsDir)
	}
}

func TestDefaultWebPrivilegedPorts(t *testing.T) {
	w := DefaultWeb()
	if w.HTTPPort != 80 || w.HTTPSPort != 443 || !w.HTTPS ||
		!w.RedirectHTTP || w.Bind != "127.0.0.1" || w.Domain != "test" {
		t.Fatalf("DefaultWeb = %+v", w)
	}
	// Normalize still accepts user-chosen ports.
	w.HTTPPort, w.HTTPSPort = 8080, 8443
	got, err := w.Normalize()
	if err != nil {
		t.Fatal(err)
	}
	if got.HTTPPort != 8080 || got.HTTPSPort != 8443 {
		t.Fatalf("Normalize changed custom ports: %+v", got)
	}
}

func TestSiteEnabledDefaultsAndPersistence(t *testing.T) {
	// A config written before the field existed defaults every site to enabled.
	legacy := filepath.Join(t.TempDir(), "legacy.json")
	body := `{"version":1,"sites":{"old.test":{"path":"/srv/old"}}}`
	if err := os.WriteFile(legacy, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	old, err := Load(legacy)
	if err != nil {
		t.Fatal(err)
	}
	if !old.Sites["old.test"].Enabled {
		t.Fatal("legacy config site was not enabled")
	}

	// An explicit false must survive a save/reload round trip; omitempty on a
	// plain bool would silently drop it and Load would re-enable the site.
	path := filepath.Join(t.TempDir(), "config.json")
	store := NewStore(path)
	if err := store.Update(func(c *Config) error {
		s := c.Sites["blog.test"]
		s.Enabled = false
		c.Sites["blog.test"] = s
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"enabled": false`) {
		t.Fatalf("disabled site was not persisted:\n%s", data)
	}
	c, err := store.View()
	if err != nil {
		t.Fatal(err)
	}
	if c.Sites["blog.test"].Enabled {
		t.Fatal("explicit false was re-enabled on load")
	}

	// A raw Site marshals the flag in both directions.
	raw, err := json.Marshal(Site{Path: "/srv/x", Enabled: false})
	if err != nil {
		t.Fatal(err)
	}
	var parsed Site
	if err := json.Unmarshal(raw, &parsed); err != nil {
		t.Fatal(err)
	}
	if parsed.Enabled {
		t.Fatalf("explicit false did not round-trip: %s", raw)
	}
}

func TestStorePersistsNewSettingsAtomically(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	s := NewStore(path)
	if err := s.Update(func(c *Config) error {
		c.Mail.Port = 3000
		c.Dumps.Capturing = true
		c.Dumps.ShowInPage = true
		c.Debugger.Detect = true
		c.Debugger.Mode = "develop"
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("mode = %o", info.Mode().Perm())
	}
	c, err := s.View()
	if err != nil {
		t.Fatal(err)
	}
	if c.Mail.Port != 3000 || !c.Dumps.Capturing || !c.Dumps.ShowInPage ||
		!c.Debugger.Detect || c.Debugger.Mode != "develop" {
		t.Fatalf("persisted = %+v %+v %+v", c.Mail, c.Dumps, c.Debugger)
	}
	entries, err := os.ReadDir(filepath.Dir(path))
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if len(e.Name()) > 8 && e.Name()[:8] == ".config-" {
			t.Fatalf("temp file left behind: %s", e.Name())
		}
	}
}
