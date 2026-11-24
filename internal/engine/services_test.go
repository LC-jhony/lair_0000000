package engine

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/lair-sh/lair/internal/config"
)

func TestDatabaseServiceIDFallsBackToMariaDB(t *testing.T) {
	e := newTestEngine(t)
	if got := e.databaseServiceID("mariadb"); got != "mariadb" {
		t.Fatalf("mariadb = %q", got)
	}
	if got := e.databaseServiceID("mysql"); got != "" {
		t.Fatalf("mysql without any engine = %q, want empty", got)
	}
	if err := os.MkdirAll(filepath.Join(e.Paths.Services, "mariadb", "11.8.9-buildN1"), 0o755); err != nil {
		t.Fatal(err)
	}
	if got := e.databaseServiceID("mysql"); got != "mariadb" {
		t.Fatalf("mysql with MariaDB installed = %q, want mariadb", got)
	}
	if err := os.MkdirAll(filepath.Join(e.Paths.Services, "mysql", "8.4.13-buildN1"), 0o755); err != nil {
		t.Fatal(err)
	}
	if got := e.databaseServiceID("mysql"); got != "mysql" {
		t.Fatalf("mysql with MySQL installed = %q, want mysql", got)
	}
}

func TestChooseInstallPortAvoidsOtherService(t *testing.T) {
	e := newTestEngine(t)
	if err := e.Config.Update(func(c *config.Config) error {
		c.Ports["redis"] = 6379
		c.Ports["mariadb"] = 3306
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	// Valkey and Redis share the conventional 6379 port. An automatic choice
	// must skip a port already assigned to another Lair service.
	got := e.chooseInstallPort("valkey", 0)
	if got <= 6379 || got == 3306 {
		t.Fatalf("valkey port = %d, want a free port above 6379", got)
	}
	// An explicit port is always respected, even when it is in use.
	if got := e.chooseInstallPort("valkey", 6390); got != 6390 {
		t.Fatalf("explicit port = %d", got)
	}
	// A service being reinstalled keeps its own port.
	if err := e.Config.Update(func(c *config.Config) error {
		c.Ports["valkey"] = 6400
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if got := e.chooseInstallPort("valkey", 0); got != 6400 {
		t.Fatalf("reinstall port = %d, want 6400", got)
	}
}
