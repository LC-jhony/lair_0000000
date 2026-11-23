package engine

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestDumpsOptionsIngestListAndClear(t *testing.T) {
	e, srv := testEnv(t)
	defer srv.Close()
	ctx := context.Background()

	initial, err := e.DumpsControl(ctx, DumpsRequest{Op: "status"})
	if err != nil {
		t.Fatal(err)
	}
	if initial.Capturing || !initial.Options.Queries || initial.Options.ShowInPage {
		t.Fatalf("defaults = %+v", initial)
	}

	updated, err := e.DumpsControl(ctx, DumpsRequest{
		Op: "options", Types: []string{"queries", "jobs"}, ShowInPage: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !updated.Options.Queries || !updated.Options.Jobs || updated.Options.Logs ||
		updated.Options.Views || updated.Options.Requests || !updated.Options.ShowInPage {
		t.Fatalf("options = %+v", updated)
	}
	c, err := e.Config.View()
	if err != nil {
		t.Fatal(err)
	}
	if !c.Dumps.ShowInPage || !c.Dumps.Jobs || c.Dumps.Logs {
		t.Fatalf("persisted = %+v", c.Dumps)
	}

	// Ingest two events and verify the JSONL file on disk.
	for _, event := range []DumpsRequest{
		{Op: "ingest", Site: "acme.test", Type: "queries", Time: "2026-01-02T03:04:05Z", Message: "select * from users", Request: "GET /users"},
		{Op: "ingest", Site: "acme.test", Type: "jobs", Time: "2026-01-03T03:04:05Z", Message: "SyncJob dispatched"},
		{Op: "ingest", Site: "blog.test", Type: "logs", Message: "otro sitio"},
	} {
		if _, err := e.DumpsControl(ctx, event); err != nil {
			t.Fatal(err)
		}
	}
	data, err := os.ReadFile(filepath.Join(e.Paths.Root, "dumps", "acme.test.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 2 || !strings.Contains(lines[0], "select * from users") {
		t.Fatalf("jsonl = %s", data)
	}

	status, err := e.DumpsControl(ctx, DumpsRequest{Op: "status"})
	if err != nil {
		t.Fatal(err)
	}
	if status.Count != 3 {
		t.Fatalf("status count = %d", status.Count)
	}

	listed, err := e.DumpsControl(ctx, DumpsRequest{Op: "list", Site: "acme.test", Type: "queries"})
	if err != nil {
		t.Fatal(err)
	}
	if listed.Count != 1 || len(listed.Events) != 1 || listed.Events[0].Message != "select * from users" {
		t.Fatalf("list = %+v", listed)
	}
	filtered, err := e.DumpsControl(ctx, DumpsRequest{Op: "list", Message: "SYNCJOB"})
	if err != nil {
		t.Fatal(err)
	}
	if filtered.Count != 1 || filtered.Events[0].Site != "acme.test" {
		t.Fatalf("message filter = %+v", filtered)
	}
	if _, err := e.DumpsControl(ctx, DumpsRequest{Op: "ingest", Site: "../escape", Message: "x"}); err == nil {
		t.Fatal("traversal in site accepted")
	}
	if _, err := e.DumpsControl(ctx, DumpsRequest{Op: "capture-anything"}); err == nil {
		t.Fatal("unknown op accepted")
	}

	cleared, err := e.DumpsControl(ctx, DumpsRequest{Op: "clear"})
	if err != nil {
		t.Fatal(err)
	}
	if cleared.Count != 0 {
		t.Fatalf("clear count = %d", cleared.Count)
	}
	if _, err := os.Stat(filepath.Join(e.Paths.Root, "dumps", "acme.test.jsonl")); !os.IsNotExist(err) {
		t.Fatalf("jsonl survived clear: %v", err)
	}
}

func TestDumpsAgentAndCLIScanDir(t *testing.T) {
	e, srv := testEnv(t)
	defer srv.Close()
	ctx := context.Background()

	// Simulate an installed PHP line with both scan dirs.
	phpDir := filepath.Join(e.Paths.PHP, "8.4")
	for _, dir := range []string{phpDir, filepath.Join(phpDir, "conf.d-cli"), filepath.Join(phpDir, "conf.d-xdebug")} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(phpDir, "lair.json"), []byte(`{"version":"8.4.3","build":"N1","minor":"8.4"}`), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := e.DumpsControl(ctx, DumpsRequest{Op: "capture-on"}); err != nil {
		t.Fatal(err)
	}
	agent := filepath.Join(e.Paths.Tools, "dumps-agent.php")
	body, err := os.ReadFile(agent)
	if err != nil {
		t.Fatal(err)
	}
	for _, needle := range []string{"proc_open", "VarDumper", "dumps", "ingest", "--site", "--token"} {
		if !strings.Contains(string(body), needle) {
			t.Fatalf("agent missing %q", needle)
		}
	}

	tokenPath := filepath.Join(e.Paths.Run, "dumps.token")
	info, err := os.Stat(tokenPath)
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("token mode = %v, %v", info, err)
	}
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), exe) {
		t.Fatalf("agent does not embed the lair binary: %s", body)
	}
	if !strings.Contains(string(body), e.Paths.Root) {
		t.Fatalf("agent does not embed LAIR_HOME: %s", body)
	}

	ini := filepath.Join(phpDir, "conf.d-cli", "zz-lair-dumps.ini")
	iniBody, err := os.ReadFile(ini)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(iniBody), "auto_prepend_file=") || !strings.Contains(string(iniBody), agent) {
		t.Fatalf("cli ini = %s", iniBody)
	}
	if _, err := os.Stat(filepath.Join(phpDir, "conf.d-xdebug", "zz-lair-dumps.ini")); !os.IsNotExist(err) {
		t.Fatalf("dumps ini leaked into the FPM/xdebug scan dir: %v", err)
	}

	// The PHP agent must at least parse, if a PHP binary is available.
	if path, err := exec.LookPath("php"); err == nil {
		out, err := exec.Command(path, "-l", agent).CombinedOutput()
		if err != nil {
			t.Fatalf("php -l: %v: %s", err, out)
		}
	}

	if _, err := e.DumpsControl(ctx, DumpsRequest{Op: "capture-off"}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(ini); !os.IsNotExist(err) {
		t.Fatalf("cli ini survived capture-off: %v", err)
	}
	if _, err := os.Stat(agent); err != nil {
		t.Fatalf("agent removed by capture-off: %v", err)
	}
}
