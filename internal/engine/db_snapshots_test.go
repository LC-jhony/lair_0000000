package engine

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lair-sh/lair/internal/app"
	"github.com/lair-sh/lair/internal/config"
)

func snapshotTestEngine(t *testing.T) *Engine {
	t.Helper()
	root := t.TempDir()
	paths, err := app.Resolve()
	if err != nil {
		t.Fatal(err)
	}
	paths.Root = root
	paths.Bin = filepath.Join(root, "bin")
	paths.Tools = filepath.Join(root, "tools")
	paths.Services = filepath.Join(root, "services")
	paths.Certs = filepath.Join(root, "certs")
	paths.Snapshots = filepath.Join(root, "snapshots")
	paths.Run = filepath.Join(root, "run")
	paths.ConfigFile = filepath.Join(root, "config.json")
	if err := paths.Ensure(); err != nil {
		t.Fatal(err)
	}
	cfg := config.New()
	siteDir := filepath.Join(root, "site")
	if err := os.MkdirAll(filepath.Join(siteDir, "database"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(siteDir, ".env"), []byte("DB_CONNECTION=sqlite\nDB_DATABASE=database/database.sqlite\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg.Sites["acme.test"] = config.Site{Path: siteDir, Database: "sqlite"}
	store := config.NewStore(paths.ConfigFile)
	if err := store.Update(func(c *config.Config) error {
		*c = *cfg
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	return &Engine{Paths: paths, Config: store}
}

func TestDBSnapshotsSQLiteRoundTrip(t *testing.T) {
	e := snapshotTestEngine(t)
	ctx := context.Background()

	dbFile := filepath.Join(e.Paths.Root, "site", "database", "database.sqlite")
	if err := os.WriteFile(dbFile, []byte("SQLite format 3\npayload-original"), 0o600); err != nil {
		t.Fatal(err)
	}

	res, err := e.DBControl(ctx, DBRequest{Op: "snapshot", Site: "acme.test"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(res.Output, "snapshot ") {
		t.Fatalf("snapshot output = %q", res.Output)
	}
	list, err := e.DBControl(ctx, DBRequest{Op: "snapshots"})
	if err != nil {
		t.Fatal(err)
	}
	if len(list.Snapshots) != 1 || list.Snapshots[0].Bytes == 0 {
		t.Fatalf("snapshots = %+v", list.Snapshots)
	}
	if len(list.Snapshots[0].SHA256) != 64 {
		t.Fatalf("missing sha256: %+v", list.Snapshots[0])
	}

	// Destructive restore must take its own pre-snapshot and then apply the
	// selected one; replacing the file content proves the restore ran.
	if err := os.WriteFile(dbFile, []byte("SQLite format 3\npayload-changed"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := e.DBControl(ctx, DBRequest{Op: "restore", Site: "acme.test",
		Name: list.Snapshots[0].ID}); err == nil {
		t.Fatal("restore without confirm accepted")
	}
	if _, err := e.DBControl(ctx, DBRequest{Op: "restore", Site: "acme.test",
		Name: list.Snapshots[0].ID, Confirm: true}); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(dbFile)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "payload-original") {
		t.Fatalf("restore did not apply the snapshot: %q", data)
	}
	after, _ := e.DBControl(ctx, DBRequest{Op: "snapshots"})
	if len(after.Snapshots) != 2 {
		t.Fatalf("expected the pre-restore snapshot too, got %d", len(after.Snapshots))
	}
}

func TestDBImportRejectsUnconfirmedAndOutsidePaths(t *testing.T) {
	e := snapshotTestEngine(t)
	ctx := context.Background()
	outside := filepath.Join(t.TempDir(), "evil.sql")
	if err := os.WriteFile(outside, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := e.DBControl(ctx, DBRequest{Op: "import", Site: "acme.test", File: outside}); err == nil {
		t.Fatal("import without confirm accepted")
	}
	if _, err := e.DBControl(ctx, DBRequest{Op: "import", Site: "acme.test", File: outside, Confirm: true}); err == nil {
		t.Fatal("import from outside home accepted")
	}
	if _, err := e.DBControl(ctx, DBRequest{Op: "restore", Site: "acme.test", Name: "../escape"}); err == nil {
		t.Fatal("traversal in snapshot id accepted")
	}
}
