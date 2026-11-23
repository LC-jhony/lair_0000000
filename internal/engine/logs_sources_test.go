package engine

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/lair-sh/lair/internal/config"
)

func TestListLogSources(t *testing.T) {
	e, srv := testEnv(t)
	defer srv.Close()

	siteRoot := t.TempDir()
	logsDir := filepath.Join(siteRoot, "storage", "logs")
	if err := os.MkdirAll(logsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"laravel.log", "telemetry.log"} {
		if err := os.WriteFile(filepath.Join(logsDir, name), []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := e.Config.Update(func(c *config.Config) error {
		c.Sites["acme.test"] = config.Site{Path: siteRoot}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"lair.log", "nginx-error.log", "nginx-access.log", "acme.test-php.log"} {
		if err := os.WriteFile(filepath.Join(e.Paths.Log, name), []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	sources, err := e.ListLogSources()
	if err != nil {
		t.Fatal(err)
	}
	byID := map[string]LogSource{}
	for _, s := range sources {
		byID[s.ID] = s
	}
	site, ok := byID["acme.test"]
	if !ok || site.Kind != "site" || site.Path != logsDir || len(site.Files) != 2 {
		t.Fatalf("site source = %+v", site)
	}
	for _, id := range []string{"daemon", "nginx", "php-fpm"} {
		s, ok := byID[id]
		if !ok || s.Kind != "service" || s.Path != e.Paths.Log || len(s.Files) == 0 {
			t.Fatalf("service %s = %+v", id, s)
		}
	}
	if !containsString(byID["php-fpm"].Files, "acme.test-php.log") {
		t.Fatalf("php-fpm files = %v", byID["php-fpm"].Files)
	}
}

func containsString(list []string, want string) bool {
	for _, v := range list {
		if v == want {
			return true
		}
	}
	return false
}
