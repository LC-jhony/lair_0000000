package daemon

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lair-sh/lair/internal/config"
	"github.com/lair-sh/lair/internal/project"
)

func TestProjectActionDirsOps(t *testing.T) {
	d, paths := testDispatcher(t)
	home := filepath.Dir(filepath.Dir(paths.Root)) // keep the fixture away from LAIR_HOME
	t.Setenv("HOME", home)

	var listed project.Result
	if err := d.Call(context.Background(), ActionProject, ProjectRequest{Op: "dirs"}, &listed); err != nil {
		t.Fatal(err)
	}
	if len(listed.Dirs) == 0 || listed.Dirs[0] != config.DefaultProjectsDir(home) {
		t.Fatalf("dirs = %v", listed.Dirs)
	}

	target := filepath.Join(t.TempDir(), "apps")
	if err := d.Call(context.Background(), ActionProject, ProjectRequest{Op: "dirs-add", Dir: target}, &listed); err != nil {
		t.Fatal(err)
	}
	if len(listed.Dirs) != 2 || listed.Dirs[1] != target {
		t.Fatalf("dirs after add = %v", listed.Dirs)
	}
	if err := d.Call(context.Background(), ActionProject, ProjectRequest{Op: "dirs-remove", Dir: target}, &listed); err != nil {
		t.Fatal(err)
	}
	if len(listed.Dirs) != 1 {
		t.Fatalf("dirs after remove = %v", listed.Dirs)
	}
	if _, err := os.Stat(target); err != nil {
		t.Fatalf("dirs-remove deleted the folder: %v", err)
	}
}

func TestProjectActionMigrateAndImportPayloads(t *testing.T) {
	d, _ := testDispatcher(t)
	home := t.TempDir()
	t.Setenv("HOME", home)

	from := filepath.Join(home, "lair")
	to := filepath.Join(home, "Lair")
	if err := os.MkdirAll(from, 0o755); err != nil {
		t.Fatal(err)
	}
	var res project.Result
	req := ProjectRequest{Op: "migrate-dirs"}
	req.From, req.To = from, to
	if err := d.Call(context.Background(), ActionProject, req, &res); err != nil {
		t.Fatal(err)
	}
	if res.Path != to || !strings.Contains(res.Output, to) {
		t.Fatalf("migrate result = %+v", res)
	}

	valet := filepath.Join(home, ".valet")
	if err := os.MkdirAll(valet, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(valet, "config.json"),
		[]byte(`{"domain":"test","paths":[]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := d.Call(context.Background(), ActionProject, ProjectRequest{Op: "import-valet"}, &res); err != nil {
		t.Fatal(err)
	}
	if res.Output == "" || len(res.Dirs) == 0 {
		t.Fatalf("import result = %+v", res)
	}
}
