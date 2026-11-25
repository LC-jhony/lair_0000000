package cli

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/lair-sh/lair/internal/app"
	"github.com/lair-sh/lair/internal/engine"
	"github.com/lair-sh/lair/internal/manifest"
)

func TestUsageAndHelp(t *testing.T) {
	var out, errOut bytes.Buffer
	if err := run(nil, &out, &errOut); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "lair install") || !strings.Contains(out.String(), "lair daemon --foreground") {
		t.Fatalf("usage = %q", out.String())
	}
	out.Reset()
	if err := run([]string{"--help"}, &out, &errOut); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "Usage:") {
		t.Fatalf("help = %q", out.String())
	}
}

func TestVersionCommandNeedsNoDaemon(t *testing.T) {
	var out, errOut bytes.Buffer
	if err := run([]string{"version"}, &out, &errOut); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(out.String(), "lair ") {
		t.Fatalf("version = %q", out.String())
	}
	out.Reset()
	if err := run([]string{"--version"}, &out, &errOut); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(out.String(), "lair ") {
		t.Fatalf("--version = %q", out.String())
	}
}

func TestNewCommandHelpAndUsage(t *testing.T) {
	var out, errOut bytes.Buffer
	if err := run([]string{"new", "--help"}, &out, &errOut); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "lair new <name>") || !strings.Contains(out.String(), "lair project new") {
		t.Fatalf("new help = %q", out.String())
	}
	out.Reset()
	if err := run([]string{"help"}, &out, &errOut); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "lair new <name>") {
		t.Fatalf("usage = %q", out.String())
	}
}

func TestProjectNameArgSkipsOptionValues(t *testing.T) {
	cases := []struct {
		args []string
		want string
	}{
		{[]string{"blog", "--database", "mariadb"}, "blog"},
		{[]string{"--database", "mariadb", "blog"}, "blog"},
		{[]string{"--parent", "/tmp/x", "--database=sqlite", "mi-app"}, "mi-app"},
		{[]string{"--secure", "--no-auth", "shop"}, "shop"},
		{[]string{"--db-name", "shop_dev", "shop"}, "shop"},
	}
	for _, tc := range cases {
		if got := projectNameArg(tc.args); got != tc.want {
			t.Errorf("projectNameArg(%v) = %q, want %q", tc.args, got, tc.want)
		}
	}
}

func TestHelpAfterNameDoesNotCreate(t *testing.T) {
	for _, args := range [][]string{
		{"new", "blog", "--help"},
		{"project", "new", "blog", "--help"},
		{"import-valet", "--help"},
		{"project", "dir", "--help"},
	} {
		var out, errOut bytes.Buffer
		if err := run(args, &out, &errOut); err != nil {
			t.Fatalf("run(%v): %v", args, err)
		}
		if !strings.Contains(out.String(), "usage:") && !strings.Contains(out.String(), "lair new <name>") {
			t.Errorf("run(%v) did not print help: %q", args, out.String())
		}
	}
}

func TestIsCatalogError(t *testing.T) {
	cases := []struct {
		err  error
		want bool
	}{
		{nil, false},
		{errors.New("network timeout"), false},
		{errors.New("daemon: catalog is empty; wait for a refresh"), true},
		{errors.New("no catalog loaded; start the daemon"), true},
	}
	for _, tc := range cases {
		if got := isCatalogError(tc.err); got != tc.want {
			t.Fatalf("isCatalogError(%v) = %v, want %v", tc.err, got, tc.want)
		}
	}
}

func containsVersion(list []string, want string) bool {
	for _, v := range list {
		if v == want {
			return true
		}
	}
	return false
}

func TestEmbeddedSignedCatalog(t *testing.T) {
	t.Setenv("LAIR_HOME", t.TempDir())
	paths, err := app.Resolve()
	if err != nil {
		t.Fatal(err)
	}
	catalog, err := buildCatalog(paths)
	if err != nil {
		t.Fatal(err)
	}
	mf := catalog.Get()
	if mf == nil {
		t.Fatal("embedded catalog was not accepted")
	}
	nodes := mf.Versions("node", app.Platform())
	if len(nodes) != 1 || nodes[0] != "v26.13.1" {
		t.Fatalf("node versions = %v", nodes)
	}
	if lts := mf.LTSOf("node", "v26.13.1", app.Platform()); lts != "Lithium" {
		t.Fatalf("LTS = %q", lts)
	}
	dbs := mf.Versions("mariadb", "linux-x64")
	if len(dbs) != 2 || !containsVersion(dbs, "11.8.9") || !containsVersion(dbs, "11.4.7") {
		t.Fatalf("mariadb versions = %v", dbs)
	}
	phps := mf.Versions("php", "linux-x64")
	if len(phps) != 5 || !containsVersion(phps, "8.5.8") || !containsVersion(phps, "8.1.34") {
		t.Fatalf("php versions = %v", phps)
	}
	phpArtifact, ok := mf.Get("php", "8.4.23", "linux-x64")
	if !ok || len(phpArtifact.Components) != 1 {
		t.Fatalf("php component missing: %+v", phpArtifact)
	}
	if err := manifest.VerifyArtifact(phpArtifact); err != nil {
		t.Fatalf("php artifact rejected: %v", err)
	}
	if dbs := mf.Versions("postgres", "linux-x64"); len(dbs) != 2 {
		t.Fatalf("postgres versions = %v", dbs)
	}
	eng, err := engine.New(paths, catalog)
	if err != nil {
		t.Fatal(err)
	}
	if got := eng.LatestLTS("node"); got != "v26.13.1" {
		t.Fatalf("LatestLTS = %q", got)
	}
}

func TestStatusOffline(t *testing.T) {
	t.Setenv("LAIR_HOME", t.TempDir())
	var out, errOut bytes.Buffer
	if err := run([]string{"status"}, &out, &errOut); err != nil {
		t.Fatalf("status: %v (%s)", err, errOut.String())
	}
	text := out.String()
	for _, want := range []string{"Lair home:", "not running (offline view)", "PHP:", "Services:", "postgres"} {
		if !strings.Contains(text, want) {
			t.Errorf("status missing %q:\n%s", want, text)
		}
	}
	// JSON mode must produce parseable output.
	out.Reset()
	if err := run([]string{"status", "--json"}, &out, &errOut); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(strings.TrimSpace(out.String()), "{") {
		t.Fatalf("json status = %q", out.String())
	}
}

func TestParseServiceArgs(t *testing.T) {
	a, err := parseServiceArgs([]string{"postgres", "16", "--port", "55432", "--delete-data"})
	if err != nil {
		t.Fatal(err)
	}
	if a.id != "postgres" || a.version != "16" || a.port != 55432 || !a.deleteData {
		t.Fatalf("parsed = %+v", a)
	}
	a, err = parseServiceArgs([]string{"redis"})
	if err != nil {
		t.Fatal(err)
	}
	if a.id != "redis" || a.version != "" || a.port != 0 || a.deleteData {
		t.Fatalf("parsed = %+v", a)
	}
	for _, args := range [][]string{
		{"redis", "--port"},
		{"redis", "--port", "abc"},
		{"redis", "--port", "70000"},
	} {
		if _, err := parseServiceArgs(args); err == nil {
			t.Errorf("invalid args accepted: %v", args)
		}
	}
}

func TestMustPathsDoesNotPanicAndFlagErrors(t *testing.T) {
	// An invalid environment must be reported, not crash the CLI.
	t.Setenv("HOME", "")
	t.Setenv("LAIR_HOME", "")
	t.Setenv("XDG_DATA_HOME", "")
	if paths, err := mustPaths(); err == nil {
		t.Fatalf("mustPaths = %+v, %v; want an error", paths, err)
	}
	// Restore a valid environment so the failures below come from flag parsing.
	t.Setenv("HOME", t.TempDir())
	for _, args := range [][]string{
		{"--http-port"},
		{"--http-port", "nope"},
		{"--dns-port", "0"},
		{"--domain"},
	} {
		if err := runDaemon(append([]string{"daemon", "--foreground"}, args...), &bytes.Buffer{}, &bytes.Buffer{}); err == nil {
			t.Errorf("invalid daemon args accepted: %v", args)
		}
	}
}

type deadlineCaller struct {
	deadlineSeen bool
}

func (d *deadlineCaller) Call(ctx context.Context, action string, payload, out any) error {
	if _, ok := ctx.Deadline(); ok {
		d.deadlineSeen = true
	}
	return ctx.Err()
}

func TestWaitJobsUseTimeoutAndContext(t *testing.T) {
	old := jobWaitTimeout
	jobWaitTimeout = 40 * time.Millisecond
	defer func() { jobWaitTimeout = old }()
	caller := &deadlineCaller{}
	c := &ctx{caller: caller, out: &bytes.Buffer{}, errOut: &bytes.Buffer{}}
	if err := c.waitJob("job-1"); err == nil {
		t.Fatal("waitJob ignored the timeout")
	}
	if !caller.deadlineSeen {
		t.Fatal("waitJob did not pass a context with deadline to the caller")
	}
	caller.deadlineSeen = false
	if err := c.waitProjectJob("job-2"); err == nil {
		t.Fatal("waitProjectJob ignored the timeout")
	}
	if !caller.deadlineSeen {
		t.Fatal("waitProjectJob did not pass a context with deadline to the caller")
	}
}

func TestCallContextForwardsCancellation(t *testing.T) {
	caller := &deadlineCaller{}
	c := &ctx{caller: caller}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := c.callContext(ctx, "status", nil, nil); err == nil {
		t.Fatal("cancelled context was ignored")
	}
}

func TestUnknownCommand(t *testing.T) {
	var out, errOut bytes.Buffer
	if err := run([]string{"frobnicate"}, &out, &errOut); err == nil {
		t.Fatal("unknown command accepted")
	}
}

func TestUninstallReversalScripts(t *testing.T) {
	root := t.TempDir()
	t.Setenv("LAIR_HOME", root)
	// Uninstall removes the PATH snippets from the user's profiles; keep that
	// inside the sandbox so the developer's real shell is never modified.
	t.Setenv("HOME", t.TempDir())
	var out, errOut bytes.Buffer
	if err := run([]string{"uninstall"}, &out, &errOut); err != nil {
		t.Fatalf("uninstall: %v (%s)", err, errOut.String())
	}

	data, err := os.ReadFile(filepath.Join(root, revertPrivileged))
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	if !strings.Contains(text, "--remove") || !strings.Contains(text, trustSystemScript) {
		t.Errorf("reversal script does not call the CA helper with --remove:\n%s", text)
	}
	// The old bug embedded the install branch: with $1 empty it re-installed
	// the CA. That branch must not appear in the reversal script.
	if strings.Contains(text, "install -m 0644") {
		t.Errorf("reversal script still contains the CA install branch:\n%s", text)
	}
	if !strings.Contains(out.String(), trustUserScript+" --remove") {
		t.Errorf("output does not explain the NSS removal:\n%s", out.String())
	}
	if _, err := os.Stat(filepath.Join(root, trustUserScript)); err != nil {
		t.Errorf("NSS user helper missing: %v", err)
	}
}

func TestEnsureShimsAfterInstall(t *testing.T) {
	root := t.TempDir()
	t.Setenv("LAIR_HOME", root)
	// Keep the generated profile snippets inside the sandbox: the fallback in
	// ensureShims uses os.UserHomeDir(), which reads HOME on Unix.
	home := t.TempDir()
	t.Setenv("HOME", home)
	paths, err := app.Resolve()
	if err != nil {
		t.Fatal(err)
	}
	c := &ctx{paths: paths, out: &bytes.Buffer{}, errOut: &bytes.Buffer{}}
	if _, onPath := shimStatus(paths.Bin); onPath {
		t.Fatal("test environment unexpectedly has the Lair bin dir on PATH")
	}
	c.ensureShims()
	missing, _ := shimStatus(paths.Bin)
	if len(missing) != 0 {
		t.Fatalf("shims still missing: %v", missing)
	}
	if !strings.Contains(c.out.(*bytes.Buffer).String(), "Abre una terminal nueva") {
		t.Fatalf("no hint about refreshing the shell:\n%s", c.out.(*bytes.Buffer).String())
	}
	// The profile snippets must target the sandboxed HOME, never the real one.
	body, err := os.ReadFile(filepath.Join(home, ".bashrc"))
	if err != nil || !strings.Contains(string(body), paths.Bin) {
		t.Fatalf("sandboxed .bashrc = %q, %v", body, err)
	}
}

func TestDoctorReportsMissingShims(t *testing.T) {
	root := t.TempDir()
	t.Setenv("LAIR_HOME", root)
	var out, errOut bytes.Buffer
	if err := run([]string{"doctor"}, &out, &errOut); err != nil {
		t.Fatalf("doctor: %v (%s)", err, errOut.String())
	}
	text := out.String()
	for _, want := range []string{"Lair doctor", "PATH:", "Shims:", "FALTAN"} {
		if !strings.Contains(text, want) {
			t.Errorf("doctor missing %q:\n%s", want, text)
		}
	}
	if !strings.Contains(text, "export PATH=") {
		t.Errorf("doctor did not print a PATH fix:\n%s", text)
	}
}

func TestUninstallDeleteDataKeepsReversalScripts(t *testing.T) {
	root := t.TempDir()
	home := t.TempDir()
	t.Setenv("LAIR_HOME", root)
	t.Setenv("HOME", home)
	var out, errOut bytes.Buffer
	if err := run([]string{"uninstall", "--delete-data"}, &out, &errOut); err != nil {
		t.Fatalf("uninstall: %v (%s)", err, errOut.String())
	}
	if _, err := os.Stat(root); !os.IsNotExist(err) {
		t.Fatalf("data root should be deleted (got %v)", err)
	}
	for _, name := range []string{revertPrivileged, trustSystemScript, trustUserScript} {
		if _, err := os.Stat(filepath.Join(home, name)); err != nil {
			t.Errorf("%s not preserved in home: %v", name, err)
		}
	}
}
