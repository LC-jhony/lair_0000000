package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	"charm.land/huh/v2"
	"github.com/lair-sh/lair/internal/app"
	"github.com/lair-sh/lair/internal/daemon"
	"github.com/lair-sh/lair/internal/engine"
	"github.com/lair-sh/lair/internal/installer"
	"github.com/lair-sh/lair/internal/manifest"
	"github.com/lair-sh/lair/internal/project"
	"github.com/lair-sh/lair/internal/web"
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

func TestTUIHelp(t *testing.T) {
	var out, errOut bytes.Buffer
	if err := run([]string{"tui", "--help"}, &out, &errOut); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "lair tui") || !strings.Contains(out.String(), "tab") {
		t.Fatalf("tui help = %q", out.String())
	}
	out.Reset()
	if err := run([]string{"dashboard", "-h"}, &out, &errOut); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "elegir e instalar una versión") {
		t.Fatalf("dashboard help = %q", out.String())
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

// ---------- TUI dashboard regressions ----------

type tuiFakeCall struct {
	action  string
	payload any
}

type tuiFakeCaller struct {
	calls   []tuiFakeCall
	handler func(action string, payload, out any) error
}

func (f *tuiFakeCaller) Call(_ context.Context, action string, payload, out any) error {
	f.calls = append(f.calls, tuiFakeCall{action: action, payload: payload})
	if f.handler != nil {
		return f.handler(action, payload, out)
	}
	return nil
}

func (f *tuiFakeCaller) count(action string) int {
	n := 0
	for _, call := range f.calls {
		if call.action == action {
			n++
		}
	}
	return n
}

func putFakeResult(out, value any) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, out)
}

// runTUICmd executes a command that may be a tea.Batch (m.start batches the
// spinner tick with the action) and returns the business message it produced.
func runTUICmd(cmd tea.Cmd) tea.Msg {
	if cmd == nil {
		return nil
	}
	msg := cmd()
	batch, ok := msg.(tea.BatchMsg)
	if !ok {
		return msg
	}
	for _, part := range batch {
		if part == nil {
			continue
		}
		out := part()
		switch out.(type) {
		case actionMsg, jobMsg:
			return out
		}
	}
	return msg
}

func testTUIModel(t *testing.T, caller daemon.Caller) *tuiModel {
	t.Helper()
	c := &ctx{caller: caller, out: &bytes.Buffer{}, errOut: &bytes.Buffer{}}
	return newTUIModel(c)
}

func TestTUIHelpIsSingleSource(t *testing.T) {
	if strings.TrimSpace(tuiHelp) == "" {
		t.Fatal("derived --help text is empty")
	}
	if !strings.Contains(tuiHelpMD, strings.TrimSpace(tuiHelp)) {
		t.Fatal("--help text does not come from the embedded Markdown")
	}
	var out, errOut bytes.Buffer
	if err := run([]string{"tui", "--help"}, &out, &errOut); err != nil {
		t.Fatal(err)
	}
	if out.String() != tuiHelp {
		t.Fatalf("--help output and tuiHelp differ:\n%q\n%q", out.String(), tuiHelp)
	}
	// Every documented key must appear in the derived plain-text help.
	normalized := strings.NewReplacer("→", "right", "←", "left", "↑", "up", "↓", "down").Replace(tuiHelp)
	for _, group := range tuiKeys.FullHelp() {
		for _, binding := range group {
			for _, keyName := range binding.Keys() {
				keyName = strings.TrimSpace(keyName)
				if keyName != "" && !strings.Contains(normalized, keyName) {
					t.Errorf("key %q (%s) missing from --help", keyName, binding.Help().Key)
				}
			}
		}
	}
	for _, want := range []string{"elegir e instalar una versión", "rueda", "contextual", "C "} {
		if !strings.Contains(tuiHelp, want) {
			t.Errorf("--help missing %q", want)
		}
	}
}

func TestUpdateFormOnlyReceivesKeyAndMouse(t *testing.T) {
	fake := &tuiFakeCaller{}
	m := testTUIModel(t, fake)
	if _, cmd := m.openForm("demo", m.inputField("Valor", "", &m.formText)); cmd == nil {
		t.Fatal("form did not start")
	}
	m.busy = "Instalando…"

	// A business message must still reach the main switch while the form is open.
	m.Update(actionMsg{label: "tarea", err: errors.New("fallo\x1b[31m")})
	if m.form == nil {
		t.Fatal("actionMsg closed the open form")
	}
	if m.busy != "" {
		t.Fatal("actionMsg did not clear busy")
	}
	if !strings.Contains(m.message, "fallo") || strings.Contains(m.message, "\x1b[31m") {
		t.Fatalf("daemon error was not sanitized: %q", m.message)
	}
	m.Update(statusMsg{st: engine.Status{Services: map[string]engine.ServiceView{}}})
	if !m.loaded {
		t.Fatal("statusMsg did not reach the main switch")
	}
	m.Update(tea.WindowSizeMsg{Width: 90, Height: 30})
	if m.width != 90 {
		t.Fatalf("WindowSizeMsg did not reach the main switch: width=%d", m.width)
	}

	// A key press belongs to the form, not to the dashboard key map.
	if _, cmd := m.Update(tea.KeyPressMsg{Code: 'r', Text: "r"}); cmd != nil {
		_ = cmd // the form may return its own command; no daemon call expected
	}
	if fake.count(daemon.ActionStatus) != 0 {
		t.Fatal("key press leaked to the dashboard key map")
	}
	if m.form == nil {
		t.Fatal("key press closed the open form")
	}
}

func TestStartActionDoneErrorCancellationAndJob(t *testing.T) {
	cases := []struct {
		name      string
		status    installer.Status
		actionErr error
		wantErr   string
		wantJob   string
		wantShims int
	}{
		{name: "error", status: installer.Status{Done: true, Error: "falló \x1b[31m"}, wantErr: "falló", wantShims: 0},
		{name: "canceled", status: installer.Status{Done: true, Canceled: true, Error: "context canceled"}, wantErr: "cancelada", wantShims: 0},
		{name: "job", status: installer.Status{ID: "job-1"}, wantJob: "job-1", wantShims: 0},
		{name: "ok", status: installer.Status{Done: true}, wantShims: 1},
		{name: "transport", actionErr: errors.New("connect to daemon: \x1b[2Kboom"), wantErr: "boom", wantShims: 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fake := &tuiFakeCaller{handler: func(action string, payload, out any) error {
				if action == daemon.ActionEnsureShims {
					return nil
				}
				if tc.actionErr != nil {
					return tc.actionErr
				}
				return putFakeResult(out, tc.status)
			}}
			m := testTUIModel(t, fake)
			msg := m.startAction("instalar", daemon.ActionInstallPHP, nil)()
			switch out := msg.(type) {
			case jobMsg:
				if tc.wantJob == "" || out.id != tc.wantJob {
					t.Fatalf("jobMsg = %+v, want %q", out, tc.wantJob)
				}
			case actionMsg:
				if tc.wantJob != "" {
					t.Fatalf("got actionMsg %+v, want job", out)
				}
				if tc.wantErr == "" {
					if out.err != nil {
						t.Fatalf("unexpected error: %v", out.err)
					}
				} else if out.err == nil || !strings.Contains(out.err.Error(), tc.wantErr) {
					t.Fatalf("error = %v, want %q", out.err, tc.wantErr)
				} else if strings.Contains(out.err.Error(), "\x1b") {
					t.Fatalf("error not sanitized: %q", out.err.Error())
				}
			default:
				t.Fatalf("unexpected msg %T", msg)
			}
			if got := fake.count(daemon.ActionEnsureShims); got != tc.wantShims {
				t.Errorf("ensure-shims calls = %d, want %d", got, tc.wantShims)
			}
		})
	}
}

func TestPollJobCountersCancellationAndTransientRetries(t *testing.T) {
	t.Run("done with error", func(t *testing.T) {
		fake := &tuiFakeCaller{handler: func(action string, payload, out any) error {
			if action == daemon.ActionJob {
				return putFakeResult(out, installer.Status{Done: true, Error: "falló \x1b[2Kinstalación"})
			}
			return nil
		}}
		m := testTUIModel(t, fake)
		msg := m.pollJob("instalar", "job", 3, 0)()
		out, ok := msg.(actionMsg)
		if !ok || out.err == nil || !strings.Contains(out.err.Error(), "falló instalación") {
			t.Fatalf("msg = %+v, want sanitized failure", msg)
		}
		if fake.count(daemon.ActionEnsureShims) != 0 {
			t.Fatal("ensure-shims called for a failed job")
		}
	})
	t.Run("canceled before error", func(t *testing.T) {
		fake := &tuiFakeCaller{handler: func(action string, payload, out any) error {
			return putFakeResult(out, installer.Status{Done: true, Canceled: true, Error: "context canceled"})
		}}
		m := testTUIModel(t, fake)
		msg := m.pollJob("instalar", "job", 0, 0)()
		out, ok := msg.(actionMsg)
		if !ok || out.err == nil || !strings.Contains(out.err.Error(), "cancelado") {
			t.Fatalf("msg = %+v, want canceled actionMsg", msg)
		}
	})
	t.Run("attempt limit", func(t *testing.T) {
		fake := &tuiFakeCaller{}
		m := testTUIModel(t, fake)
		msg := m.pollJob("instalar", "job", maxPollAttempts, 0)()
		out, ok := msg.(actionMsg)
		if !ok || out.err == nil || !strings.Contains(out.err.Error(), "espera agotado") {
			t.Fatalf("msg = %+v, want timeout", msg)
		}
		if len(fake.calls) != 0 {
			t.Fatal("daemon called after the attempt limit")
		}
	})
	t.Run("bounded transport retries", func(t *testing.T) {
		calls := 0
		fake := &tuiFakeCaller{handler: func(action string, payload, out any) error {
			calls++
			if action == daemon.ActionEnsureShims {
				return nil
			}
			if calls <= 2 {
				return errors.New("connect to daemon: connection refused")
			}
			return putFakeResult(out, installer.Status{Done: true})
		}}
		m := testTUIModel(t, fake)
		msg := m.pollJob("instalar", "job", 0, 0)()
		first, ok := msg.(pollMsg)
		if !ok || first.transport != 1 || first.attempt != 1 {
			t.Fatalf("first retry = %+v", msg)
		}
		msg = m.pollJob(first.label, first.id, first.attempt, first.transport)()
		second, ok := msg.(pollMsg)
		if !ok || second.transport != 2 {
			t.Fatalf("second retry = %+v", msg)
		}
		msg = m.pollJob(second.label, second.id, second.attempt, second.transport)()
		out, ok := msg.(actionMsg)
		if !ok || out.err != nil {
			t.Fatalf("final msg = %+v, want success", msg)
		}
	})
	t.Run("bad request is not retried", func(t *testing.T) {
		fake := &tuiFakeCaller{handler: func(action string, payload, out any) error {
			if action == daemon.ActionJob {
				return errors.New("daemon: unknown job")
			}
			return nil
		}}
		m := testTUIModel(t, fake)
		msg := m.pollJob("instalar", "job", 0, 0)()
		if out, ok := msg.(actionMsg); !ok || out.err == nil {
			t.Fatalf("msg = %+v, want immediate error", msg)
		}
		if got := fake.count(daemon.ActionJob); got != 1 {
			t.Fatalf("job calls = %d, want 1", got)
		}
	})
}

func TestApplyDialogProjectValidation(t *testing.T) {
	fake := &tuiFakeCaller{}
	m := testTUIModel(t, fake)
	m.formText, m.formText2 = "mi-app", t.TempDir()
	m.projectDB = "mysql"
	m.projectDBUser, m.projectDBPass = "", ""
	if msg, ok := runTUICmd(m.applyDialog("project")).(actionMsg); !ok || msg.err == nil ||
		!strings.Contains(msg.err.Error(), "usuario") {
		t.Fatalf("missing DB user was accepted: %+v", runTUICmd(m.applyDialog("project")))
	}
	if fake.count(daemon.ActionProject) != 0 {
		t.Fatal("project action launched despite invalid credentials")
	}
	m.projectDBUser, m.projectDBPass = "root", "secret"
	msg := runTUICmd(m.applyDialog("project"))
	out, ok := msg.(actionMsg)
	if !ok || out.err != nil {
		t.Fatalf("valid credentials rejected: %+v", msg)
	}
	if fake.count(daemon.ActionProject) != 1 {
		t.Fatalf("project calls = %d, want 1", fake.count(daemon.ActionProject))
	}
	request, ok := fake.calls[0].payload.(daemon.ProjectRequest)
	if !ok || request.DBPassword != "secret" {
		t.Fatalf("project payload = %+v", fake.calls[0].payload)
	}

	// SQLite must not require credentials.
	m.busy = ""
	m.projectDB, m.projectDBUser, m.projectDBPass = "sqlite", "", ""
	if out, ok := runTUICmd(m.applyDialog("project")).(actionMsg); !ok || out.err != nil {
		t.Fatalf("sqlite project rejected: %+v", out)
	}
}

func TestSpinnerOnlyRunsWhileBusy(t *testing.T) {
	m := testTUIModel(t, &tuiFakeCaller{})
	batch, ok := m.Init()().(tea.BatchMsg)
	if !ok {
		t.Fatal("Init did not return a batch")
	}
	for _, cmd := range batch {
		if _, ok := cmd().(spinner.TickMsg); ok {
			t.Fatal("Init started the spinner")
		}
	}
	if _, cmd := m.Update(spinner.TickMsg{}); cmd != nil {
		t.Fatal("idle model rescheduled the spinner")
	}
	m.busy = "Instalando…"
	if _, cmd := m.Update(spinner.TickMsg{}); cmd == nil {
		t.Fatal("busy model did not advance the spinner")
	}
	m.busy = ""
	if _, cmd := m.Update(spinner.TickMsg{}); cmd != nil {
		t.Fatal("spinner did not stop when busy was cleared")
	}
}

func TestReentrancyGuard(t *testing.T) {
	fake := &tuiFakeCaller{}
	m := testTUIModel(t, fake)
	m.busy = "Instalando…"
	if _, cmd := m.Update(tea.KeyPressMsg{Code: 'u', Text: "u"}); cmd != nil {
		t.Fatal("blocked action returned a command")
	}
	if fake.count(daemon.ActionUpdate) != 0 {
		t.Fatal("new action launched while busy")
	}
	if !strings.Contains(m.message, "espera") {
		t.Fatalf("no busy notice: %q", m.message)
	}
}

func TestLoadCatalogParallelAndProjectInfoFilter(t *testing.T) {
	fake := &tuiFakeCaller{handler: func(action string, payload, out any) error {
		switch action {
		case daemon.ActionVersions:
			pkg := payload.(daemon.VersionRequest).Package
			return putFakeResult(out, daemon.VersionResult{Versions: []string{pkg + "-1"}})
		case daemon.ActionProject:
			return putFakeResult(out, daemon.ProjectResult{Info: project.Info{
				NodeManagers: []string{"npm", "pnpm"},
				Databases:    []string{"sqlite", "mysql"},
				Hint:         "falta el instalador",
			}})
		}
		return nil
	}}
	m := testTUIModel(t, fake)
	batch, ok := m.loadCatalog()().(tea.BatchMsg)
	if !ok {
		t.Fatal("loadCatalog did not batch its package commands")
	}
	if len(batch) < 3 {
		t.Fatalf("batch has %d commands", len(batch))
	}
	for _, cmd := range batch {
		m.Update(cmd())
	}
	if m.catalogPending != 0 {
		t.Fatalf("catalogPending = %d", m.catalogPending)
	}
	if len(m.catalog) != len(batch) {
		t.Fatalf("catalog has %d/%d packages: %v", len(m.catalog), len(batch), m.catalog)
	}

	m.Update(m.loadProjectInfo()())
	if len(m.projectInfo.NodeManagers) != 2 {
		t.Fatalf("project info = %+v", m.projectInfo)
	}
	if got := filterOptions(project.NodeManagers, m.projectInfo.NodeManagers); len(got) != 2 || got[0] != "npm" || got[1] != "pnpm" {
		t.Fatalf("filtered node managers = %v", got)
	}
	if got := filterOptions(project.Databases, m.projectInfo.Databases); len(got) != 2 || got[0] != "sqlite" || got[1] != "mysql" {
		t.Fatalf("filtered databases = %v", got)
	}
	form := m.projectForm()
	_ = form.Init()
	if !strings.Contains(m.message, "falta el instalador") {
		t.Fatalf("wizard did not show the installer hint: %q", m.message)
	}
}

func TestViewportContentUpdatedInUpdateAndNarrowLayout(t *testing.T) {
	m := testTUIModel(t, &tuiFakeCaller{})
	m.loaded = true
	m.status = engine.Status{Services: map[string]engine.ServiceView{}}
	for i := 0; i < 12; i++ {
		m.status.Services[fmt.Sprintf("svc-%02d", i)] = engine.ServiceView{}
	}
	m.pane, m.selected = paneServices, 11
	m.setSize(60, 16)
	m.Update(tea.WindowSizeMsg{Width: 60, Height: 16})
	if m.viewport.YOffset() == 0 {
		t.Fatal("selected service was not scrolled into view")
	}
	before := m.viewport.GetContent()
	_ = m.View()
	if m.viewport.GetContent() != before {
		t.Fatal("View mutated the viewport content")
	}

	m.pane = paneOverview
	m.setSize(60, 16)
	m.Update(tea.WindowSizeMsg{Width: 60, Height: 16})
	lines := strings.Split(m.viewport.GetContent(), "\n")
	versionLine, envLine := -1, -1
	for i, line := range lines {
		if strings.Contains(line, "Versiones") {
			versionLine = i
		}
		if strings.Contains(line, "Entorno") {
			envLine = i
		}
	}
	if versionLine < 0 || envLine < 0 || versionLine == envLine {
		t.Fatalf("narrow overview cards are not stacked: %v", lines)
	}
	m.setSize(120, 30)
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	lines = strings.Split(m.viewport.GetContent(), "\n")
	versionLine, envLine = -1, -1
	for i, line := range lines {
		if strings.Contains(line, "Versiones") {
			versionLine = i
		}
		if strings.Contains(line, "Entorno") {
			envLine = i
		}
	}
	if versionLine < 0 || envLine != versionLine {
		t.Fatalf("wide overview cards are not side by side: %v", lines)
	}
}

func TestTUIPaneNavigationFollowsPaneOrder(t *testing.T) {
	m := testTUIModel(t, &tuiFakeCaller{})

	seen := map[paneID]bool{}
	for _, id := range paneOrder {
		if seen[id] {
			t.Fatalf("pane %d is duplicated in paneOrder", id)
		}
		seen[id] = true
	}
	for id := range paneInfos {
		if !seen[id] {
			t.Fatalf("pane %d has no sidebar entry", id)
		}
	}

	for i, id := range paneOrder {
		if next := paneAt(paneIndex(id) + 1); next != paneOrder[(i+1)%len(paneOrder)] {
			t.Fatalf("next after pane %d = %d, want %d", id, next, paneOrder[(i+1)%len(paneOrder)])
		}
		if prev := paneAt(paneIndex(id) - 1); prev != paneOrder[(i-1+len(paneOrder))%len(paneOrder)] {
			t.Fatalf("prev before pane %d = %d, want %d", id, prev, paneOrder[(i-1+len(paneOrder))%len(paneOrder)])
		}
	}

	// Sitios is followed by Correo, not by Sistema: the enum order is not the
	// navigation order.
	if got := paneAt(paneIndex(paneSites) + 1); got != paneMail {
		t.Fatalf("next after Sitios = %d, want Correo (%d)", got, paneMail)
	}

	m.pane = paneOverview
	_ = m.goToPaneNumber(5)
	if m.pane != paneSites {
		t.Fatalf("5 jumped to pane %d, want Sitios (%d)", m.pane, paneSites)
	}
	m.setSize(100, 30)
	if sidebar := m.renderSidebar(); !strings.Contains(sidebar, "5") {
		t.Fatalf("sidebar does not show pane numbers: %q", sidebar)
	}
}

// ---------- Tooling panes: Mail, Dumps and Debugger ----------

// runPaneCmd unwraps the spinner batch used by pane actions and returns the
// first business message (mailMsg / dumpsMsg / debuggerMsg / actionMsg).
func runPaneCmd(cmd tea.Cmd) tea.Msg {
	if cmd == nil {
		return nil
	}
	msg := cmd()
	batch, ok := msg.(tea.BatchMsg)
	if !ok {
		return msg
	}
	for _, part := range batch {
		if part == nil {
			continue
		}
		out := part()
		switch out.(type) {
		case mailMsg, dumpsMsg, debuggerMsg, actionMsg, jobMsg:
			return out
		}
	}
	return msg
}

func keyPress(text string) tea.KeyPressMsg {
	return tea.KeyPressMsg{Code: rune(text[0]), Text: text}
}

func TestTUIToolingHelpSynced(t *testing.T) {
	// The pre-existing TestTUIHelpIsSingleSource verifies that every key from
	// FullHelp appears in the embedded help; this test additionally asserts
	// that the three new panes are documented in both help surfaces.
	for _, group := range [][]key.Binding{tuiKeys.ShortHelp(), flattenBindings(tuiKeys.FullHelp())} {
		found := map[string]bool{}
		for _, b := range group {
			found[b.Help().Key] = true
		}
		for _, want := range []string{"M", "D", "B"} {
			if !found[want] {
				t.Errorf("key %q missing from Short/FullHelp: %v", want, found)
			}
		}
	}
	for _, want := range []string{"M             correo", "D             dumps", "B             debugger", "Correo, dumps y debugger"} {
		if !strings.Contains(tuiHelpMD, want) {
			t.Errorf("tui_help.md missing %q", want)
		}
	}
}

func flattenBindings(groups [][]key.Binding) []key.Binding {
	var out []key.Binding
	for _, group := range groups {
		out = append(out, group...)
	}
	return out
}

func TestTUIToolingKeysNoCollision(t *testing.T) {
	k := tuiKeys
	// Uppercase letters match their own bindings and never the lowercase ones.
	for _, tc := range []struct {
		text  string
		mine  key.Binding
		other []key.Binding
	}{
		{"M", k.Mail, []key.Binding{k.Database, k.Defaults, k.Logs, k.CreateDB, k.Next}},
		{"D", k.Dumps, []key.Binding{k.Defaults, k.Database, k.Logs, k.CreateDB}},
		{"B", k.Debugger, []key.Binding{k.CreateDB, k.Database, k.Defaults, k.Logs}},
	} {
		msg := keyPress(tc.text)
		if !key.Matches(msg, tc.mine) {
			t.Errorf("%q did not match its own binding", tc.text)
		}
		for _, binding := range tc.other {
			if key.Matches(msg, binding) {
				t.Errorf("%q collided with binding %q", tc.text, binding.Help().Key)
			}
		}
	}
	// Lowercase m/d/v/b keep their original meaning.
	lowers := []struct {
		text string
		want key.Binding
	}{
		{"m", k.Database}, {"d", k.Defaults}, {"v", k.Logs}, {"b", k.CreateDB},
	}
	for _, tc := range lowers {
		msg := keyPress(tc.text)
		if !key.Matches(msg, tc.want) {
			t.Errorf("lowercase %q no longer matches %q", tc.text, tc.want.Help().Key)
		}
		for _, binding := range []key.Binding{k.Mail, k.Dumps, k.Debugger} {
			if key.Matches(msg, binding) {
				t.Errorf("lowercase %q triggered tooling binding %q", tc.text, binding.Help().Key)
			}
		}
	}
}

func TestTUIToolingKeyDispatch(t *testing.T) {
	fake := &tuiFakeCaller{}
	m := testTUIModel(t, fake)
	m.loaded = true

	// M opens the mail dialog only in the Correo pane.
	m.pane = paneMail
	m.Update(keyPress("M"))
	if m.dialog != "mail" || m.form == nil {
		t.Fatalf("M did not open the mail dialog: dialog=%q", m.dialog)
	}
	m.form, m.dialog = nil, ""

	// D and B behave the same in their own panes.
	m.pane = paneDumps
	m.Update(keyPress("D"))
	if m.dialog != "dumps" {
		t.Fatalf("D did not open the dumps dialog: %q", m.dialog)
	}
	m.form, m.dialog = nil, ""
	m.pane = paneDebugger
	m.Update(keyPress("B"))
	if m.dialog != "debugger" {
		t.Fatalf("B did not open the debugger dialog: %q", m.dialog)
	}
	m.form, m.dialog = nil, ""

	// The lowercase keys still reach their original dialogs.
	m.pane = paneDatabases
	m.catalog = map[string][]string{"mariadb": {"11.8"}}
	m.Update(keyPress("m"))
	if m.dialog != "installService" {
		t.Fatalf("m did not open the database dialog: %q", m.dialog)
	}
}

func TestTUIToolingPanesRender(t *testing.T) {
	m := testTUIModel(t, &tuiFakeCaller{})
	m.loaded = true
	m.setSize(120, 30)

	m.mail = mailResult{
		Running: true, Port: 1025, Count: 2, Env: "desarrollo",
		Messages: []mailMessage{{ID: "m1", From: "noreply@lair.test", To: "dev@lair.test", Subject: "Bienvenido", Date: "hoy", Body: "hola"}},
	}
	m.mailLoaded = true
	m.pane = paneMail
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	text := m.viewport.GetContent()
	for _, want := range []string{"Correo", "1025", "activo", "Bienvenido", "desarrollo"} {
		if !strings.Contains(text, want) {
			t.Errorf("mail pane missing %q:\n%s", want, text)
		}
	}

	// The detail view is shown above the pane content and closed with esc.
	m.detailTitle, m.detailText = "Mensaje · Bienvenido", "cuerpo del mensaje"
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	if text := m.viewport.GetContent(); !strings.Contains(text, "cuerpo del mensaje") {
		t.Errorf("mail detail not rendered:\n%s", text)
	}
	m.Update(keyPress("x")) // no-op key; detail still open
	m.Update(tea.KeyPressMsg{Code: 0, Text: "esc"})
	if m.detailText != "" {
		t.Fatal("esc did not close the mail detail")
	}

	m.dumps = dumpsResult{
		Capturing: true, Count: 3,
		Options: dumpsOptions{Queries: true, Requests: true, ShowInPage: true},
		Events:  []dumpEvent{{ID: "e1", Site: "acme.test", Type: "query", Message: "SELECT 1"}},
	}
	m.dumpsLoaded = true
	m.pane = paneDumps
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	text = m.viewport.GetContent()
	for _, want := range []string{"Dumps", "ACTIVA", "queries", "requests", "acme.test", "SELECT 1"} {
		if !strings.Contains(text, want) {
			t.Errorf("dumps pane missing %q:\n%s", want, text)
		}
	}

	m.debugger = debuggerResult{
		Detect: true, Breakpoints: true, Port: 9003, Mode: "listen",
		Versions: []debuggerVersion{{Version: "8.3", Available: true}, {Version: "8.4", Available: false}},
		Domains:  []string{"acme.test"},
	}
	m.debuggerLoaded = true
	m.pane = paneDebugger
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	text = m.viewport.GetContent()
	for _, want := range []string{"Debugger", "9003", "listen", "8.3", "acme.test"} {
		if !strings.Contains(text, want) {
			t.Errorf("debugger pane missing %q:\n%s", want, text)
		}
	}
}

func TestTUIToolingApplyDialog(t *testing.T) {
	cases := []struct {
		name    string
		dialog  string
		value   string
		action  string
		op      string
		id      string
		wantBtn bool
	}{
		{"mail start", "mail", mailStartSMTP, paneActionMail, "start", "", false},
		{"mail stop", "mail", mailStopSMTP, paneActionMail, "stop", "", false},
		{"mail clear", "mail", mailClearInbox, paneActionMail, "clear", "", false},
		{"mail refresh", "mail", mailRefresh, paneActionMail, "status", "", false},
		{"mail message", "mailMessage", "m1", paneActionMail, "message", "m1", false},
		{"dumps on", "dumps", dumpsCaptureOn, paneActionDumps, "capture-on", "", false},
		{"dumps off", "dumps", dumpsCaptureOff, paneActionDumps, "capture-off", "", false},
		{"dumps show page", "dumps", dumpsShowPage, paneActionDumps, "options", "", false},
		{"dumps clear", "dumps", dumpsClearEvents, paneActionDumps, "clear", "", false},
		{"dumps refresh", "dumps", dumpsRefresh, paneActionDumps, "status", "", false},
		{"debugger detect on", "debugger", debuggerDetectOn, paneActionDebugger, "detect-on", "", false},
		{"debugger detect off", "debugger", debuggerDetectOff, paneActionDebugger, "detect-off", "", false},
		{"debugger bp on", "debugger", debuggerBreakpointsOn, paneActionDebugger, "breakpoints-on", "", false},
		{"debugger bp off", "debugger", debuggerBreakpointsOff, paneActionDebugger, "breakpoints-off", "", false},
		{"debugger refresh", "debugger", debuggerRefresh, paneActionDebugger, "status", "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fake := &tuiFakeCaller{}
			m := testTUIModel(t, fake)
			m.formText = tc.value
			msg := runPaneCmd(m.applyDialog(tc.dialog))
			switch out := msg.(type) {
			case mailMsg:
				if out.err != nil || out.op != tc.op || out.state.Port != 0 {
					t.Fatalf("mailMsg = %+v", out)
				}
			case dumpsMsg:
				if out.err != nil || out.op != tc.op {
					t.Fatalf("dumpsMsg = %+v", out)
				}
			case debuggerMsg:
				if out.err != nil || out.op != tc.op {
					t.Fatalf("debuggerMsg = %+v", out)
				}
			default:
				t.Fatalf("unexpected message %T: %+v", msg, msg)
			}
			if fake.count(tc.action) != 1 {
				t.Fatalf("%s calls = %d, want 1", tc.action, fake.count(tc.action))
			}
			payload := fake.calls[0].payload
			switch tc.action {
			case paneActionMail:
				req, ok := payload.(mailRequest)
				if !ok || req.Op != tc.op || req.ID != tc.id {
					t.Fatalf("mail payload = %+v", payload)
				}
			case paneActionDumps:
				req, ok := payload.(dumpsRequest)
				if !ok || req.Op != tc.op {
					t.Fatalf("dumps payload = %+v", payload)
				}
				if tc.op == "options" && (req.ShowInPage == nil || !*req.ShowInPage) {
					t.Fatalf("show_in_page payload = %+v", payload)
				}
			case paneActionDebugger:
				req, ok := payload.(debuggerRequest)
				if !ok || req.Op != tc.op {
					t.Fatalf("debugger payload = %+v", payload)
				}
			}
		})
	}

	// "ver mensaje" chains into a message picker instead of calling the daemon.
	fake := &tuiFakeCaller{}
	m := testTUIModel(t, fake)
	m.mail.Messages = []mailMessage{{ID: "m1", Subject: "Hola"}}
	m.formText = mailViewMsg
	if cmd := m.applyDialog("mail"); cmd == nil {
		t.Fatal("ver mensaje did not open a picker")
	}
	if m.dialog != "mailMessage" || m.form == nil {
		t.Fatalf("message picker not opened: dialog=%q", m.dialog)
	}
	if id := m.mailMessageIDs["Hola"]; id != "m1" {
		t.Fatalf("message id map = %v", m.mailMessageIDs)
	}
	m.formText = "Hola"
	fetched := runPaneCmd(m.applyDialog("mailMessage"))
	if out, ok := fetched.(mailMsg); !ok || out.op != "message" {
		t.Fatalf("message fetch = %+v", fetched)
	}
}

func TestTUIToolingAsyncLoadAndMessages(t *testing.T) {
	fake := &tuiFakeCaller{handler: func(action string, payload, out any) error {
		switch action {
		case paneActionMail:
			switch payload.(mailRequest).Op {
			case "status":
				return putFakeResult(out, mailResult{Running: true, Port: 2525, Count: 4, Env: "qa"})
			case "list":
				return putFakeResult(out, mailResult{Messages: []mailMessage{{ID: "x", Subject: "Aviso"}}})
			}
		case paneActionDumps:
			switch payload.(dumpsRequest).Op {
			case "status":
				return putFakeResult(out, dumpsResult{Capturing: true, Count: 7, Options: dumpsOptions{Views: true}})
			case "list":
				return putFakeResult(out, dumpsResult{Events: []dumpEvent{{ID: "e", Type: "view"}}})
			}
		case paneActionDebugger:
			return putFakeResult(out, debuggerResult{Detect: true, Port: 9000, Mode: "listen"})
		}
		return nil
	}}
	m := testTUIModel(t, fake)
	// The loader returns commands whose messages never block Update.
	apply := func(cmd tea.Cmd) {
		msg := cmd()
		if batch, ok := msg.(tea.BatchMsg); ok {
			for _, part := range batch {
				if part != nil {
					m.Update(part())
				}
			}
			return
		}
		m.Update(msg)
	}
	apply(m.loadMail())
	apply(m.loadDumps())
	apply(m.loadDebugger())
	if !m.mailLoaded || m.mail.Port != 2525 || len(m.mail.Messages) != 1 {
		t.Fatalf("mail state = %+v", m.mail)
	}
	if !m.dumpsLoaded || !m.dumps.Capturing || len(m.dumps.Events) != 1 {
		t.Fatalf("dumps state = %+v", m.dumps)
	}
	if !m.debuggerLoaded || m.debugger.Port != 9000 {
		t.Fatalf("debugger state = %+v", m.debugger)
	}

	// A failed action surfaces the error without corrupting loaded state.
	m.Update(mailMsg{err: errors.New("boom\x1b[31m")})
	if m.busy != "" || !strings.Contains(m.message, "boom") || strings.Contains(m.message, "\x1b[31m") {
		t.Fatalf("mail error not handled: %q", m.message)
	}
	if m.mail.Port != 2525 {
		t.Fatal("a failed mail action changed the loaded state")
	}
}

func TestTUILogsListSources(t *testing.T) {
	fake := &tuiFakeCaller{}
	m := testTUIModel(t, fake)
	m.Update(logNamesMsg{
		items:   []string{"lair.log"},
		sources: []logSource{{Kind: "sitio", Name: "acme.log", ID: "acme", Files: []string{"error.log"}}},
	})
	if m.dialog != "logs" || m.form == nil {
		t.Fatal("logs dialog was not opened")
	}
	if got := m.logNameKeys["sitio · acme.log (error.log)"]; got != "acme.log" {
		t.Fatalf("source mapping = %v", m.logNameKeys)
	}
	m.formText = "sitio · acme.log (error.log)"
	_ = runTUICmd(m.applyDialog("logs"))
	if fake.count(daemon.ActionLogs) != 1 {
		t.Fatalf("logs calls = %d, want 1", fake.count(daemon.ActionLogs))
	}
	if payload, ok := fake.calls[0].payload.(daemon.LogsRequest); !ok || payload.Name != "acme.log" {
		t.Fatalf("logs payload = %+v", fake.calls[0].payload)
	}
}

// ---------- Site detail regressions ----------

func siteTestModel(t *testing.T, fake *tuiFakeCaller) *tuiModel {
	t.Helper()
	m := testTUIModel(t, fake)
	m.loaded = true
	m.setSize(120, 30)
	m.status.Sites = []web.Site{
		{Name: "acme.test", Root: "/srv/acme.test"},
		{Name: "blog.test", Root: "/srv/blog.test"},
	}
	m.pane = paneSites
	return m
}

func TestTUISiteDetailKeyDispatch(t *testing.T) {
	fake := &tuiFakeCaller{handler: func(action string, payload, out any) error {
		if action != paneActionSite {
			t.Errorf("unexpected action %q", action)
			return nil
		}
		req, ok := payload.(siteRequest)
		if !ok || req.Name != "acme.test" {
			t.Errorf("site payload = %+v", payload)
		}
		return nil
	}}
	m := siteTestModel(t, fake)

	// E is contextual: it does nothing outside Sitios.
	m.pane = paneServices
	m.Update(keyPress("E"))
	if fake.count(paneActionSite) != 0 || m.siteDetailName != "" {
		t.Fatalf("E outside Sitios started a load: calls=%d name=%q", fake.count(paneActionSite), m.siteDetailName)
	}

	m.pane = paneSites
	_, cmd := m.Update(keyPress("E"))
	if m.siteDetailName != "acme.test" {
		t.Fatalf("E did not select the highlighted site: %q", m.siteDetailName)
	}
	// Commands are lazy: run the batch so the three daemon calls happen.
	if batch, ok := cmd().(tea.BatchMsg); ok {
		for _, part := range batch {
			if part != nil {
				m.Update(part())
			}
		}
	}
	if fake.count(paneActionSite) != 3 {
		t.Fatalf("site calls = %d, want detail/env/domains", fake.count(paneActionSite))
	}
	ops := map[string]bool{}
	for _, call := range fake.calls {
		ops[call.payload.(siteRequest).Op] = true
	}
	for _, op := range []string{"detail", "env", "domains"} {
		if !ops[op] {
			t.Errorf("op %q not loaded: %v", op, ops)
		}
	}

	// Down moves the highlight, and E no longer collides with `e` (link).
	m.Update(keyPress("j"))
	if m.siteSelected != 1 {
		t.Fatalf("site selection = %d, want 1", m.siteSelected)
	}
	m.form, m.dialog = nil, ""
	m.Update(keyPress("e"))
	if m.dialog != "link" || m.form == nil {
		t.Fatalf("lowercase e no longer opens link: dialog=%q", m.dialog)
	}
}

func TestTUISiteDetailRenderAndMasking(t *testing.T) {
	m := siteTestModel(t, &tuiFakeCaller{})
	if _, cmd := m.openSiteDetail(); cmd == nil {
		t.Fatal("openSiteDetail returned no command")
	}
	m.Update(siteDetailMsg{op: "detail", name: "acme.test", state: siteResult{Detail: &siteDetail{
		Name: "acme.test", Path: "/srv/acme.test", URL: "http://acme.test:8080",
		PHP: "8.4", Node: "20.11", Database: "mariadb", Secure: true,
		Root: "/srv/acme.test/public", RootState: "ok",
		Domains: []string{"acme.test", "www.acme.test"},
		Problem: "fpm caído", Notice: "certificado caduca pronto",
	}}})
	m.Update(siteDetailMsg{op: "env", name: "acme.test", state: siteResult{Env: []siteEnvVar{
		{Key: "APP_KEY", Value: "base64:super-secreto", Secret: true, Set: true},
		{Key: "DB_PASSWORD", Value: "secreto", Secret: false, Set: false},
		{Key: "APP_ENV", Value: "local", Set: true},
	}}})
	m.Update(siteDetailMsg{op: "domains", name: "acme.test", state: siteResult{Domains: []string{"acme.test", "www.acme.test"}}})

	m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	text := m.viewport.GetContent()
	for _, want := range []string{
		"Overview", "http://acme.test:8080", "8.4", "20.11", "mariadb",
		"activo", "/srv/acme.test/public", "ok", "www.acme.test",
		"fpm caído", "certificado caduca pronto", "Environment",
		"APP_KEY", "••••••••", "DB_PASSWORD", "(sin definir)", "APP_ENV", "local",
		"Logs", "logs-list / logs",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("site detail missing %q:\n%s", want, text)
		}
	}
	for _, leak := range []string{"base64:super-secreto", "secreto"} {
		if strings.Contains(text, leak) {
			t.Errorf("site detail leaked %q:\n%s", leak, text)
		}
	}

	// esc returns to the site list and clears the detail state.
	m.Update(keyPress("x")) // no-op key: the detail stays open
	if m.detailText == "" {
		t.Fatal("a no-op key closed the site detail")
	}
	m.Update(tea.KeyPressMsg{Code: 0, Text: "esc"})
	if m.detailText != "" || m.siteDetailName != "" {
		t.Fatalf("esc did not close the site detail: %q / %q", m.detailText, m.siteDetailName)
	}
}

func TestTUISiteLogsFromDetail(t *testing.T) {
	fake := &tuiFakeCaller{handler: func(action string, payload, out any) error {
		switch action {
		case daemon.ActionLogsList:
			return putFakeResult(out, logsListResult{Items: []string{"lair.log", "acme.test-error.log"}})
		case daemon.ActionLogs:
			if name := payload.(daemon.LogsRequest).Name; name != "acme.test-error.log" {
				t.Errorf("logs name = %q", name)
			}
			return putFakeResult(out, daemon.LogsResult{Text: "error de acme"})
		}
		return nil
	}}
	m := siteTestModel(t, fake)
	m.openSiteDetail()
	m.Update(siteDetailMsg{op: "detail", name: "acme.test", state: siteResult{Detail: &siteDetail{URL: "http://acme.test"}}})
	m.Update(siteDetailMsg{op: "env", name: "acme.test", state: siteResult{}})
	m.Update(siteDetailMsg{op: "domains", name: "acme.test", state: siteResult{}})
	if m.detailText == "" {
		t.Fatal("detail was not rendered")
	}

	_, cmd := m.Update(keyPress("v"))
	if m.siteLogFilter != "acme.test" || m.detailText != "" {
		t.Fatalf("v did not scope logs to the site: filter=%q detail=%q", m.siteLogFilter, m.detailText)
	}
	if cmd == nil {
		t.Fatal("v did not call logs-list")
	}
	m.Update(cmd())
	if m.form == nil || m.dialog != "logs" {
		t.Fatalf("logs picker not opened: dialog=%q", m.dialog)
	}
	if m.formText != "acme.test-error.log" {
		t.Fatalf("logs picker was not filtered: %q", m.formText)
	}
	// Complete the picker through updateForm, as a real selection does.
	m.form.State = huh.StateCompleted
	_, cmd = m.updateForm(nil)
	m.Update(runTUICmd(cmd))
	if fake.count(daemon.ActionLogs) != 1 {
		t.Fatalf("logs calls = %d, want 1", fake.count(daemon.ActionLogs))
	}
	if !strings.Contains(m.logText, "error de acme") {
		t.Fatalf("log viewer empty: %q", m.logText)
	}
	m.Update(tea.KeyPressMsg{Code: 0, Text: "esc"})
	if m.logText != "" || m.siteLogFilter != "" {
		t.Fatalf("esc did not close the log viewer: %q / %q", m.logText, m.siteLogFilter)
	}
}

func TestTUISiteDeleteDialog(t *testing.T) {
	fake := &tuiFakeCaller{}
	m := siteTestModel(t, fake)
	m.siteSelected = 1

	// X is contextual: it does nothing outside Sitios.
	m.pane = paneSystem
	m.Update(keyPress("X"))
	if m.dialog != "" || m.form != nil {
		t.Fatalf("X outside Sitios opened a dialog: %q", m.dialog)
	}
	m.pane = paneSites

	// X opens a confirmation dialog for the highlighted site.
	m.Update(keyPress("X"))
	if m.dialog != "deleteSite" || m.form == nil || m.formText != "blog.test" {
		t.Fatalf("X did not open the delete dialog: dialog=%q value=%q", m.dialog, m.formText)
	}
	if m.siteDeleteFiles {
		t.Fatal("delete_files must default to false")
	}

	// Confirming (with the folder option) unlinks and sends delete_files.
	m.confirmValue = true
	m.siteDeleteFiles = true
	m.form.State = huh.StateCompleted
	_, cmd := m.updateForm(nil)
	if cmd == nil {
		t.Fatal("confirmation produced no command")
	}
	if msg, ok := runTUICmd(cmd).(actionMsg); ok && msg.err != nil {
		t.Fatalf("delete action failed: %v", msg.err)
	}
	if fake.count(daemon.ActionUnlink) != 1 {
		t.Fatalf("unlink calls = %d, want 1", fake.count(daemon.ActionUnlink))
	}
	payload, ok := fake.calls[0].payload.(unlinkRequest)
	if !ok || payload.Name != "blog.test" || !payload.DeleteFiles {
		t.Fatalf("unlink payload = %+v", fake.calls[0].payload)
	}

	// Cancelling must never call the daemon.
	fake.calls = nil
	m.busy = "" // simulate the previous unlink having finished
	m.Update(keyPress("X"))
	m.confirmValue = false
	m.form.State = huh.StateCompleted
	if _, cmd := m.updateForm(nil); cmd != nil {
		t.Fatal("cancelled delete still returned a command")
	}
	if fake.count(daemon.ActionUnlink) != 0 {
		t.Fatal("cancelled delete called unlink")
	}
	if !strings.Contains(m.message, "cancelado") {
		t.Fatalf("cancel message = %q", m.message)
	}

	// Confirming without the folder option sends delete_files=false.
	m.busy = ""
	m.Update(keyPress("X"))
	m.confirmValue = true
	m.siteDeleteFiles = false
	m.form.State = huh.StateCompleted
	_, cmd = m.updateForm(nil)
	_ = runTUICmd(cmd)
	payload, ok = fake.calls[0].payload.(unlinkRequest)
	if !ok || payload.DeleteFiles {
		t.Fatalf("unlink payload without folder deletion = %+v", fake.calls[0].payload)
	}
}

func TestTUISiteHelpAndKeysSynced(t *testing.T) {
	short := map[string]bool{}
	for _, b := range tuiKeys.ShortHelp() {
		short[b.Help().Key] = true
	}
	full := map[string]bool{}
	for _, b := range flattenBindings(tuiKeys.FullHelp()) {
		full[b.Help().Key] = true
	}
	for _, want := range []string{"E", "X"} {
		if !short[want] || !full[want] {
			t.Errorf("key %q missing from Short/FullHelp: short=%v full=%v", want, short, full)
		}
	}
	for _, want := range []string{
		"E             detalle del sitio seleccionado",
		"X             borrar el sitio",
		"detalle del sitio seleccionado (overview, entorno y logs)",
	} {
		if !strings.Contains(tuiHelpMD, want) {
			t.Errorf("tui_help.md missing %q", want)
		}
	}
	for _, want := range []string{"E             detalle del sitio", "X             borrar el sitio"} {
		if !strings.Contains(tuiHelp, strings.TrimSpace(want)) {
			t.Errorf("derived help missing %q", want)
		}
	}

	// E and X are distinct from every contextual key they sit next to.
	for _, tc := range []struct {
		text  string
		mine  key.Binding
		other []key.Binding
	}{
		{"E", tuiKeys.SiteDetail, []key.Binding{
			tuiKeys.Link, tuiKeys.Unlink, tuiKeys.Secure, tuiKeys.Project,
			tuiKeys.Pin, tuiKeys.Isolate, tuiKeys.Logs,
			tuiKeys.Mail, tuiKeys.Dumps, tuiKeys.Debugger,
		}},
		{"X", tuiKeys.DeleteSite, []key.Binding{tuiKeys.CancelJob, tuiKeys.Unlink}},
	} {
		msg := keyPress(tc.text)
		if !key.Matches(msg, tc.mine) {
			t.Errorf("%q did not match its own binding", tc.text)
		}
		for _, binding := range tc.other {
			if key.Matches(msg, binding) {
				t.Errorf("%q collided with binding %q", tc.text, binding.Help().Key)
			}
		}
	}
}

// ---------- Site enabled / vhost regressions ----------

// runSiteCmd unwraps the spinner batch used by the site actions and returns the
// typed site message it produced.
func runSiteCmd(cmd tea.Cmd) tea.Msg {
	if cmd == nil {
		return nil
	}
	msg := cmd()
	if batch, ok := msg.(tea.BatchMsg); ok {
		for _, part := range batch {
			if part == nil {
				continue
			}
			out := part()
			switch out.(type) {
			case siteConfigMsg, siteEnabledMsg, siteDetailMsg:
				return out
			}
		}
	}
	return msg
}

func TestTUISiteVhostKeyAndConfigLoad(t *testing.T) {
	configText := "server {\n    server_name acme.test;\n    root /srv/acme.test/public;\n}"
	fake := &tuiFakeCaller{handler: func(action string, payload, out any) error {
		if action != paneActionSite {
			t.Errorf("unexpected action %q", action)
			return nil
		}
		req, ok := payload.(siteRequest)
		if !ok || req.Op != "config" || req.Name != "acme.test" {
			t.Errorf("config payload = %+v", payload)
		}
		return putFakeResult(out, siteWireResult{Config: configText})
	}}
	m := siteTestModel(t, fake)

	// V is contextual: outside Sitios it must not open a viewer or call the daemon.
	m.pane = paneSystem
	m.Update(keyPress("V"))
	if m.siteDetailName != "" || m.siteConfigView || m.detailText != "" || fake.count(paneActionSite) != 0 {
		t.Fatalf("V outside Sitios opened a vhost: name=%q view=%v detail=%q calls=%d",
			m.siteDetailName, m.siteConfigView, m.detailText, fake.count(paneActionSite))
	}

	m.pane = paneSites
	_, cmd := m.Update(keyPress("V"))
	if m.siteDetailName != "acme.test" || !m.siteConfigView {
		t.Fatalf("V did not select the highlighted site: name=%q view=%v", m.siteDetailName, m.siteConfigView)
	}
	if cmd == nil {
		t.Fatal("V did not start a config load")
	}
	msg, ok := runSiteCmd(cmd).(siteConfigMsg)
	if !ok {
		t.Fatalf("expected siteConfigMsg, got %T", runSiteCmd(cmd))
	}
	if msg.name != "acme.test" || msg.err != nil || !strings.Contains(msg.config, "server_name acme.test;") {
		t.Fatalf("siteConfigMsg = %+v", msg)
	}
	if fake.count(paneActionSite) != 1 {
		t.Fatalf("site calls = %d, want one config", fake.count(paneActionSite))
	}

	m.Update(msg)
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	text := m.viewport.GetContent()
	if !strings.Contains(text, "server_name acme.test;") || !strings.Contains(m.detailTitle, "Vhost") {
		t.Fatalf("vhost viewer did not render config:\n%s", text)
	}

	// A disabled site has no vhost; an empty config must not look like a blank bug.
	configText = ""
	fake.calls = nil
	m.Update(keyPress("esc"))
	_, cmd = m.Update(keyPress("V"))
	cfg, ok := runSiteCmd(cmd).(siteConfigMsg)
	if !ok || cfg.config != "" {
		t.Fatalf("empty config msg = %+v", cfg)
	}
	m.Update(cfg)
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	if text := m.viewport.GetContent(); !strings.Contains(text, "desactivado") {
		t.Fatalf("disabled site did not explain the missing vhost:\n%s", text)
	}
	if !m.siteConfigView {
		t.Fatal("vhost view flag was lost")
	}
	// esc returns to Sitios and clears the context so a stale reply is ignored.
	m.Update(tea.KeyPressMsg{Code: 0, Text: "esc"})
	if m.siteConfigView || m.siteDetailName != "" || m.detailText != "" {
		t.Fatalf("esc did not close the vhost viewer: %+v", m)
	}
}

func TestTUISiteToggleEnabled(t *testing.T) {
	wantEnabled := false
	fake := &tuiFakeCaller{handler: func(action string, payload, out any) error {
		if action != paneActionSite {
			t.Errorf("unexpected action %q", action)
			return nil
		}
		req, ok := payload.(siteEnabledRequest)
		if !ok || req.Op != "set-enabled" || req.Name != "blog.test" || req.Enabled != wantEnabled {
			t.Errorf("set-enabled payload = %+v, want enabled=%v", payload, wantEnabled)
		}
		return putFakeResult(out, siteWireResult{Output: "ok"})
	}}
	m := siteTestModel(t, fake)
	m.siteSelected = 1
	m.siteDetailName = "blog.test"
	m.siteEnabled = true // currently active -> t must disable
	m.detailText, m.detailTitle = "detalle del sitio", "Sitio · blog.test"

	// t is contextual: it needs the Sitios pane AND an open site detail.
	m.pane = paneServices
	m.Update(keyPress("t"))
	if m.dialog != "" || m.form != nil {
		t.Fatalf("t outside Sitios opened a dialog: %q", m.dialog)
	}
	m.pane = paneSites
	m.siteConfigView = true // a vhost viewer is not the detail view
	m.Update(keyPress("t"))
	if m.dialog != "" {
		t.Fatalf("t on the vhost view opened a dialog: %q", m.dialog)
	}
	m.siteConfigView = false
	m.Update(keyPress("t"))
	if m.dialog != "siteToggle" || m.form == nil {
		t.Fatalf("t did not open the toggle confirmation: %q", m.dialog)
	}
	if m.pendingID != "blog.test" || m.pendingStart {
		t.Fatalf("pending toggle = %q/%v", m.pendingID, m.pendingStart)
	}

	// Confirming sends `site set-enabled {name, enabled}` and applies the reply.
	m.confirmValue = true
	m.form.State = huh.StateCompleted
	_, cmd := m.updateForm(nil)
	if cmd == nil {
		t.Fatal("confirmation produced no command")
	}
	out, ok := runSiteCmd(cmd).(siteEnabledMsg)
	if !ok || out.err != nil || out.enabled || out.name != "blog.test" {
		t.Fatalf("set-enabled reply = %+v", out)
	}
	if fake.count(paneActionSite) != 1 {
		t.Fatalf("site calls = %d, want one set-enabled", fake.count(paneActionSite))
	}
	m.Update(out)
	if m.siteEnabled || m.busy != "" {
		t.Fatalf("enabled flag not updated: %v (busy=%q)", m.siteEnabled, m.busy)
	}

	// Cancelling never calls the daemon and leaves the state untouched.
	fake.calls = nil
	m.siteEnabled = true
	m.busy = ""
	m.Update(keyPress("t"))
	m.confirmValue = false
	m.form.State = huh.StateCompleted
	if _, cmd := m.updateForm(nil); cmd != nil {
		t.Fatal("cancelled toggle still produced a command")
	}
	if fake.count(paneActionSite) != 0 || !m.siteEnabled {
		t.Fatalf("cancel changed state or called the daemon: %v/%+v", m.siteEnabled, fake.calls)
	}
	if !strings.Contains(m.message, "cancelado") {
		t.Fatalf("cancel message = %q", m.message)
	}

	// Enabling a disabled site sends enabled=true.
	wantEnabled = true
	m.siteEnabled = false
	m.busy = ""
	m.Update(keyPress("t"))
	if !m.pendingStart {
		t.Fatal("t on a disabled site must send enabled=true")
	}
	m.confirmValue = true
	m.form.State = huh.StateCompleted
	_, cmd = m.updateForm(nil)
	if out, ok := runSiteCmd(cmd).(siteEnabledMsg); !ok || !out.enabled {
		t.Fatalf("enable reply = %+v", out)
	}
}

func TestTUISiteDetailShowsEnabledAndRootWarning(t *testing.T) {
	m := siteTestModel(t, &tuiFakeCaller{})
	m.siteDetailName = "acme.test"
	m.siteDetailData = siteResult{Detail: &siteDetail{
		URL: "/", Root: "/srv/acme.test/public", RootState: "missing",
		RootSuggestions: []string{"crear public/", "añadir index.php"},
	}}
	m.detailTitle = "Sitio · acme.test"
	m.detailText = renderSiteDetail(m.siteDetailData, true, nil)
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	text := m.viewport.GetContent()
	for _, want := range []string{"ACTIVADO", "aviso", "missing", "crear public/", "V ver el vhost", "t alternar enabled"} {
		if !strings.Contains(text, want) {
			t.Errorf("site detail missing %q:\n%s", want, text)
		}
	}

	// A healthy root must not show the warning, and disabled sites are labelled.
	m.siteDetailData.Detail.RootState = "ok"
	m.detailText = renderSiteDetail(m.siteDetailData, false, nil)
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	text = m.viewport.GetContent()
	if strings.Contains(text, "aviso: root no válido") {
		t.Errorf("healthy root still warned:\n%s", text)
	}
	if !strings.Contains(text, "DESACTIVADO") {
		t.Errorf("disabled site not labelled:\n%s", text)
	}
}

func TestTUISiteEnabledAndVhostKeysNoCollision(t *testing.T) {
	k := tuiKeys
	// Uppercase V is its own binding and never triggers v, E or X.
	vhost := keyPress("V")
	if !key.Matches(vhost, k.SiteVhost) {
		t.Fatal("V did not match its own binding")
	}
	for _, other := range []key.Binding{k.Logs, k.SiteDetail, k.DeleteSite, k.Mail, k.Dumps, k.Debugger} {
		if key.Matches(vhost, other) {
			t.Errorf("V collided with %q", other.Help().Key)
		}
	}
	if logs := keyPress("v"); !key.Matches(logs, k.Logs) || key.Matches(logs, k.SiteVhost) {
		t.Error("lowercase v no longer means logs")
	}
	// t is free and distinct from the contextual keys around it.
	toggle := keyPress("t")
	if !key.Matches(toggle, k.SiteToggle) {
		t.Fatal("t did not match its own binding")
	}
	for _, other := range []key.Binding{k.Quit, k.Refresh, k.SiteVhost, k.SiteDetail, k.DeleteSite, k.Logs} {
		if key.Matches(toggle, other) {
			t.Errorf("t collided with %q", other.Help().Key)
		}
	}
}

func TestTUISiteEnabledAndVhostHelpSynced(t *testing.T) {
	short, full := map[string]bool{}, map[string]bool{}
	for _, b := range tuiKeys.ShortHelp() {
		short[b.Help().Key] = true
	}
	for _, b := range flattenBindings(tuiKeys.FullHelp()) {
		full[b.Help().Key] = true
	}
	for _, want := range []string{"V", "t"} {
		if !short[want] || !full[want] {
			t.Errorf("key %q missing from Short/FullHelp: short=%v full=%v", want, short, full)
		}
	}
	for _, want := range []string{
		"V             ver el vhost generado del sitio (contextual: Sitios)",
		"t             alternar enabled del sitio en el detalle (contextual: Sitios)",
		"| `V` |", "| `t` |", "root_state", "no tiene vhost",
	} {
		if !strings.Contains(tuiHelpMD, want) {
			t.Errorf("tui_help.md missing %q", want)
		}
		if strings.Contains(want, "  ") && !strings.Contains(tuiHelp, strings.TrimSpace(want)) {
			t.Errorf("derived --help missing %q", want)
		}
	}
}

// ---------- Project directories (F) ----------

// runAllTUICmds executes every command of a batch (used for actionMsg, which
// batches the status refresh with a possible project-dirs reload).
func runAllTUICmds(t *testing.T, cmd tea.Cmd) {
	t.Helper()
	if cmd == nil {
		return
	}
	msg := cmd()
	if batch, ok := msg.(tea.BatchMsg); ok {
		for _, part := range batch {
			if part != nil {
				_ = part()
			}
		}
	}
}

// renderTUIForm initialises a freshly opened huh form before rendering it;
// without Init the form view is blank in tests (Bubble Tea runs it in the app).
func renderTUIForm(m *tuiModel) string {
	if m.form == nil {
		return ""
	}
	if cmd := m.form.Init(); cmd != nil {
		if updated, _ := m.form.Update(cmd()); updated != nil {
			if f, ok := updated.(*huh.Form); ok {
				m.form = f
			}
		}
	}
	return stripControl(m.form.View())
}

func TestTUIProjectDirsKeyNoCollision(t *testing.T) {
	k := tuiKeys
	msg := keyPress("F")
	if !key.Matches(msg, k.ProjectDirs) {
		t.Fatal("F did not match its own binding")
	}
	for _, other := range []key.Binding{
		k.SiteDetail, k.DeleteSite, k.SiteVhost, k.SiteToggle,
		k.Project, k.Mail, k.Dumps, k.Debugger,
		k.Install, k.CreateDB, k.CancelJob, k.Logs,
	} {
		if key.Matches(msg, other) {
			t.Errorf("F collided with %q", other.Help().Key)
		}
	}
	if lower := keyPress("f"); key.Matches(lower, k.ProjectDirs) {
		t.Error("lowercase f unexpectedly triggered the project-dirs binding")
	}
	// The site keys keep their exact meaning.
	for _, tc := range []struct {
		text string
		want key.Binding
	}{
		{"E", k.SiteDetail}, {"X", k.DeleteSite}, {"V", k.SiteVhost}, {"t", k.SiteToggle},
	} {
		site := keyPress(tc.text)
		if !key.Matches(site, tc.want) || key.Matches(site, k.ProjectDirs) {
			t.Errorf("site key %q no longer maps to %q", tc.text, tc.want.Help().Key)
		}
	}
	full := map[string]bool{}
	for _, b := range flattenBindings(k.FullHelp()) {
		full[b.Help().Key] = true
	}
	if !full["F"] {
		t.Errorf("F missing from FullHelp: %v", full)
	}
}

func TestTUIProjectDirsHelpSynced(t *testing.T) {
	block := helpFromMarkdown(tuiHelpMD)
	for _, want := range []string{
		"F             gestionar carpetas de proyectos",
		"x             cancelar un trabajo en curso (contextual: Sistema)",
	} {
		if !strings.Contains(block, want) {
			t.Errorf("help:start/end block missing %q:\n%s", want, block)
		}
	}
	for _, want := range []string{
		"## Carpetas de proyectos", "| `F` | Resumen / Runtimes / Sistema |",
		"`project dirs`", "`project dirs-add`", "`project dirs-remove`",
		"`project import-valet`", "`project migrate-dirs`", "`~/Lair`",
	} {
		if !strings.Contains(tuiHelpMD, want) {
			t.Errorf("tui_help.md missing %q", want)
		}
	}
	// The Sitios section must be byte-for-byte what the site tests expect.
	for _, want := range []string{
		"| `E` | Ver el detalle del sitio seleccionado (overview, entorno y logs) |",
		"| `t` | Alternar `enabled` del sitio desde la vista de detalle (`set-enabled`) |",
		"| `X` | Borrar el sitio seleccionado; confirma si se borra también su carpeta |",
	} {
		if !strings.Contains(tuiHelpMD, want) {
			t.Errorf("Sitios help was modified: missing %q", want)
		}
	}
	if !strings.Contains(tuiHelp, "F             gestionar carpetas de proyectos") {
		t.Errorf("derived --help missing F:\n%s", tuiHelp)
	}
}

func TestTUIProjectDirsLoadAddRemove(t *testing.T) {
	var ops []string
	fake := &tuiFakeCaller{handler: func(action string, payload, out any) error {
		if action != daemon.ActionProject {
			return nil
		}
		req := payload.(daemon.ProjectRequest)
		ops = append(ops, req.Op+":"+req.Dir)
		if req.Op == "dirs" {
			return putFakeResult(out, projectDirsResult{Dirs: []string{"/home/u/Lair", "/srv/apps"}})
		}
		return nil
	}}
	m := testTUIModel(t, fake)
	m.loaded = true

	// F is only active on the general panes.
	m.pane = paneSites
	m.Update(keyPress("F"))
	if m.dialog != "" || m.form != nil || len(ops) != 0 {
		t.Fatalf("F opened a dialog on Sitios: dialog=%q ops=%v", m.dialog, ops)
	}
	m.pane = paneOverview
	if _, cmd := m.Update(keyPress("F")); cmd == nil {
		t.Fatal("F did not start a command")
	}
	if m.dialog != "projectDirs" || m.form == nil {
		t.Fatalf("F did not open the dirs dialog: %q", m.dialog)
	}
	if m.formText != dirsRefresh {
		t.Fatalf("default choice = %q, want %q", m.formText, dirsRefresh)
	}
	m.Update(m.loadProjectDirs()())
	if len(m.projectDirs) != 2 || m.projectDirs[0] != "/home/u/Lair" {
		t.Fatalf("dirs not loaded: %v (err=%v)", m.projectDirs, m.projectDirsErr)
	}
	if !hasString(ops, "dirs:") {
		t.Fatalf("project dirs op not sent: %v", ops)
	}
	// The list is visible even if the reply arrives after the form was opened.
	if view := renderTUIForm(m); !strings.Contains(view, "/srv/apps") {
		t.Fatalf("dirs dialog does not list the folders:\n%s", view)
	}

	// Choosing "add" opens the follow-up input dialog.
	m.formText, m.form.State = dirsAdd, huh.StateCompleted
	if _, cmd := m.updateForm(nil); cmd == nil || m.dialog != "projectDirsAdd" {
		t.Fatalf("add choice did not open the input dialog: %q", m.dialog)
	}
}

func TestTUIProjectDirsOpsPayloads(t *testing.T) {
	var ops []string
	var migrateReq daemon.ProjectRequest
	fake := &tuiFakeCaller{handler: func(action string, payload, out any) error {
		if action != daemon.ActionProject {
			return nil
		}
		req := payload.(daemon.ProjectRequest)
		if req.Op == "migrate-dirs" {
			migrateReq = req
		}
		ops = append(ops, req.Op+":"+req.Dir)
		if req.Op == "dirs" {
			return putFakeResult(out, projectDirsResult{Dirs: []string{"/home/u/Lair"}})
		}
		return nil
	}}
	m := testTUIModel(t, fake)

	// Walking through the dialog: add, remove, import and migrate.
	for _, tc := range []struct {
		choice, label, op, dir string
	}{
		{dirsAdd, "projectDirsAdd", "dirs-add", "/tmp/nueva"},
		{dirsRemove, "projectDirsRemove", "dirs-remove", "/tmp/vieja"},
		{dirsImportValet, "", "import-valet", ""},
		{dirsMigrate, "", "migrate-dirs", ""},
	} {
		m.formText, m.formText2 = tc.choice, tc.dir
		cmd := m.applyDialog("projectDirs")
		if tc.label != "" {
			if msg, ok := runTUICmd(cmd).(actionMsg); ok && msg.err != nil {
				t.Fatalf("%s failed: %v", tc.choice, msg.err)
			}
			if m.dialog != tc.label {
				t.Fatalf("%s did not open %q: %q", tc.choice, tc.label, m.dialog)
			}
			m.formText2 = tc.dir
			cmd = m.applyDialog(tc.label)
		}
		if msg, ok := runTUICmd(cmd).(actionMsg); ok && msg.err != nil {
			t.Fatalf("%s failed: %v", tc.choice, msg.err)
		}
		want := tc.op + ":" + tc.dir
		if !hasString(ops, want) {
			t.Fatalf("op %q not sent: %v", want, ops)
		}
		m.busy = ""
	}

	// The migration is explicit about the legacy and the new folder.
	if migrateReq.From != "~/lair" || migrateReq.To != "~/Lair" {
		t.Fatalf("migrate-dirs payload = %+v, want from=~/lair to=~/Lair", migrateReq)
	}

	// The action reply reloads the list so the dialog shows fresh data.
	before := len(ops)
	_, cmd := m.Update(actionMsg{label: "carpetas"})
	runAllTUICmds(t, cmd)
	if len(ops) <= before || !hasString(ops, "dirs:") {
		t.Fatalf("dirs not reloaded after action: %v", ops)
	}

	// Invalid (empty) paths never reach the daemon.
	before = len(ops)
	if msg, ok := runTUICmd(m.applyDialog("projectDirsAdd")).(actionMsg); !ok || msg.err == nil {
		t.Fatalf("empty add path was accepted: %+v", msg)
	}
	if len(ops) != before {
		t.Fatalf("empty add path called the daemon: %v", ops)
	}
}

func TestTUIProjectParentDefaultsToLair(t *testing.T) {
	if got := defaultProjectParent(); filepath.Base(got) != "Lair" {
		t.Fatalf("default project parent = %q, want ~/Lair", got)
	}
	m := testTUIModel(t, &tuiFakeCaller{})
	parent := strings.TrimSpace(m.projectInfo.ProjectsDir)
	if parent == "" {
		parent = defaultProjectParent()
	}
	if filepath.Base(parent) != "Lair" {
		t.Fatalf("wizard parent = %q, want ~/Lair", parent)
	}
}
