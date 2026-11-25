package tui

import (
	"context"
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/huh/v2"

	"github.com/lair-sh/lair/internal/daemon"
	"github.com/lair-sh/lair/internal/engine"
	"github.com/lair-sh/lair/internal/project"
)

type formKind int

const (
	formNone formKind = iota
	formProject
	formInstallPHP
	formInstallNode
	formInstallService
	formDomain
	formCreateDB
)

type formValues struct {
	name, parent, version, id, domains, db string
	secure, createDB, auth, git            bool
}

func (m *Model) openProjectForm() {
	info := m.snap.Project
	parent := info.ProjectsDir
	if parent == "" && len(m.snap.ProjectDirs) > 0 {
		parent = m.snap.ProjectDirs[0]
	}
	if parent == "" {
		parent = project.DefaultParent()
	}
	m.formVals = formValues{parent: parent, db: info.DefaultDatabase}
	if m.formVals.db == "" {
		m.formVals.db = "sqlite"
	}
	databases := info.Databases
	if len(databases) == 0 {
		databases = project.Databases
	}
	stacks := project.Stacks
	m.form = huh.NewForm(huh.NewGroup(
		huh.NewInput().Key("name").Title("Project / site name").Value(&m.formVals.name).Validate(func(s string) error {
			if !project.ValidName(strings.TrimSpace(s)) {
				return fmt.Errorf("use lowercase letters, digits and dashes")
			}
			return nil
		}),
		huh.NewInput().Key("parent").Title("Parent folder").Value(&m.formVals.parent),
		huh.NewSelect[string]().Key("stack").Title("Stack").Options(huhOptions(stacks)...).Value(&m.formVals.version),
		huh.NewSelect[string]().Key("db").Title("Database").Options(huhOptions(databases)...).Value(&m.formVals.db),
		huh.NewConfirm().Key("secure").Title("Issue HTTPS certificate").Value(&m.formVals.secure),
		huh.NewConfirm().Key("create_db").Title("Create the database").Value(&m.formVals.createDB),
	))
	m.formKind = formProject
}

func (m *Model) openInstallForm(kind string) {
	versions := m.versions[kind]
	if len(versions) == 0 {
		versions = []string{"latest"}
	}
	switch kind {
	case "php":
		m.formKind = formInstallPHP
	case "node":
		m.formKind = formInstallNode
	default:
		m.formKind = formInstallService
		ids := []string{}
		for id := range m.snap.Status.Services {
			ids = append(ids, id)
		}
		m.form = huh.NewForm(huh.NewGroup(
			huh.NewSelect[string]().Key("id").Title("Service").Options(huhOptions(ids)...).Value(&m.formVals.id),
			huh.NewInput().Key("version").Title("Version").Value(&m.formVals.version),
		))
		return
	}
	m.form = huh.NewForm(huh.NewGroup(
		huh.NewSelect[string]().Key("version").Title("Version to install").Options(huhOptions(versions)...).Value(&m.formVals.version),
	))
}

func huhOptions(items []string) []huh.Option[string] {
	out := make([]huh.Option[string], 0, len(items))
	for _, item := range items {
		out = append(out, huh.NewOption(item, item))
	}
	return out
}

func (m *Model) openDomainForm() {
	if m.selectedSite == "" {
		m.selectedSite = firstOr(m.siteNames(), "")
	}
	m.formVals.domains = strings.Join(m.snap.Details[m.selectedSite].Domains, ", ")
	m.form = huh.NewForm(huh.NewGroup(
		huh.NewInput().Key("domains").Title("Domains (comma separated)").Value(&m.formVals.domains),
	))
	m.formKind = formDomain
}

func (m *Model) openCreateDBForm() {
	ids := []string{}
	for id := range m.snap.Status.Services {
		if isDBService(id) {
			ids = append(ids, id)
		}
	}
	if len(ids) == 0 {
		ids = []string{"mariadb", "sqlite3"}
	}
	m.formVals.id = ids[0]
	m.form = huh.NewForm(huh.NewGroup(
		huh.NewSelect[string]().Key("id").Title("Database engine").Options(huhOptions(ids)...).Value(&m.formVals.id),
		huh.NewInput().Key("name").Title("Database name").Value(&m.formVals.name).Validate(func(s string) error {
			if strings.TrimSpace(s) == "" {
				return fmt.Errorf("name is required")
			}
			return nil
		}),
	))
	m.formKind = formCreateDB
}

func isDBService(id string) bool {
	switch id {
	case "mariadb", "mysql", "postgres", "mongodb", "redis", "valkey", "sqlite3":
		return true
	}
	return false
}

func firstOr(items []string, fallback string) string {
	if len(items) > 0 {
		return items[0]
	}
	return fallback
}

// submitForm translates the completed form into one daemon action.
func (m *Model) submitForm() tea.Cmd {
	kind := m.formKind
	v := m.formVals
	m.form, m.formKind = nil, formNone
	switch kind {
	case formProject:
		req := daemon.ProjectRequest{Op: "create", CreateRequest: project.CreateRequest{
			Name: strings.TrimSpace(v.name), Parent: strings.TrimSpace(v.parent),
			Secure: v.secure, Stack: v.version, Database: v.db,
			CreateDB: v.createDB, Auth: v.auth, Git: v.git,
		}}
		return m.callCmd("create project", daemon.ActionProject, req)
	case formInstallPHP:
		return m.callCmd("install PHP "+v.version, daemon.ActionInstallPHP, daemon.InstallRequest{Version: v.version})
	case formInstallNode:
		return m.callCmd("install Node "+v.version, daemon.ActionInstallNode, daemon.InstallRequest{Version: v.version})
	case formInstallService:
		return m.callCmd("install "+v.id+" "+v.version, daemon.ActionInstallService, daemon.InstallRequest{ID: v.id, Version: v.version})
	case formDomain:
		var domains []string
		for _, d := range strings.Split(v.domains, ",") {
			if d = strings.TrimSpace(d); d != "" {
				domains = append(domains, d)
			}
		}
		return m.callCmd("update domains", daemon.ActionSite, daemon.SiteRequest{Op: "domains", Name: m.selectedSite, Domains: domains})
	case formCreateDB:
		return m.callCmd("create database "+v.name, daemon.ActionCreateDB, daemon.CreateDBRequest{ID: v.id, Name: strings.TrimSpace(v.name)})
	}
	return nil
}

func (m *Model) toggleHTTPS() tea.Cmd {
	if m.selectedSite == "" {
		m.selectedSite = firstOr(m.siteNames(), "")
	}
	if m.selectedSite == "" {
		m.notify("Select a site first", "", true)
		return nil
	}
	on := !m.snap.Details[m.selectedSite].Secure
	return m.callCmd("toggle HTTPS for "+m.selectedSite, daemon.ActionSecure, daemon.SecureRequest{Name: m.selectedSite, On: &on})
}

func (m *Model) loadConfig() tea.Cmd {
	if m.selectedSite == "" {
		m.selectedSite = firstOr(m.siteNames(), "")
	}
	site := m.selectedSite
	return m.actionCmd("load vhost", func(ctx context.Context, c daemon.Caller) (string, error) {
		var out daemon.SiteResult
		if err := c.Call(ctx, daemon.ActionSite, daemon.SiteRequest{Op: "config", Name: site}, &out); err != nil {
			return "", err
		}
		m.configText = out.Config
		return "vhost loaded", nil
	})
}

func (m *Model) loadLogs() tea.Cmd {
	name := m.currentLogSource()
	return m.actionCmd("load logs", func(ctx context.Context, c daemon.Caller) (string, error) {
		var out daemon.LogsResult
		if err := c.Call(ctx, daemon.ActionLogs, daemon.LogsRequest{Name: name}, &out); err != nil {
			return "", err
		}
		m.logText = out.Text
		return "log loaded: " + name, nil
	})
}

func (m *Model) currentLogSource() string {
	if len(m.snap.LogSources) == 0 {
		return "lair"
	}
	i := m.logSourceIdx % len(m.snap.LogSources)
	src := m.snap.LogSources[i]
	if len(src.Files) > 0 {
		return src.Files[0]
	}
	return src.ID
}

func (m *Model) restartService() tea.Cmd {
	id := m.selectedService
	if id == "" {
		id = firstOr(serviceIDs(m.snap), "")
	}
	if id == "" {
		return nil
	}
	return m.callCmd("restart "+id, daemon.ActionServiceRestart, daemon.ServiceRequest{ID: id})
}

func serviceIDs(s Snapshot) []string {
	out := make([]string, 0, len(s.Status.Services))
	for id := range s.Status.Services {
		out = append(out, id)
	}
	return out
}

func (m *Model) toggleSetting(action string) tea.Cmd {
	var name, cmd, payload = action, "", any(nil)
	switch action {
	case "web-bind":
		bind := "127.0.0.1"
		if m.snap.Status.Web.Bind == "" || m.snap.Status.Web.Bind == "127.0.0.1" {
			bind = "0.0.0.0"
		}
		cmd, payload = daemon.ActionWeb, engineWebRequest{Bind: bind}
	case "web-https":
		on := !m.snap.Status.Web.HTTPS
		cmd, payload = daemon.ActionWeb, engineWebRequest{HTTPS: &on}
	case "web-redirect":
		on := !m.snap.Status.Web.RedirectHTTP
		cmd, payload = daemon.ActionWeb, engineWebRequest{RedirectHTTP: &on}
	case "mail-toggle":
		op := "stop"
		if !m.snap.Mail.Running {
			op = "start"
		}
		cmd, payload = daemon.ActionMail, daemon.MailRequest{Op: op}
	case "dumps-toggle":
		op := "capture-off"
		if !m.snap.Dumps.Capturing {
			op = "capture-on"
		}
		cmd, payload = daemon.ActionDumps, daemon.DumpsRequest{Op: op}
	case "debugger-detect":
		op := "detect-off"
		if !m.snap.Debugger.Detect {
			op = "detect-on"
		}
		cmd, payload = daemon.ActionDebugger, daemon.DebuggerRequest{Op: op}
	case "debugger-breakpoints":
		op := "breakpoints-off"
		if !m.snap.Debugger.Breakpoints {
			op = "breakpoints-on"
		}
		cmd, payload = daemon.ActionDebugger, daemon.DebuggerRequest{Op: op}
	default:
		return nil
	}
	return m.callCmd(name, cmd, payload)
}

// engineWebRequest is a local alias so toggles stay declarative.
type engineWebRequest = engine.WebRequest

func (m *Model) loadEnv() tea.Cmd {
	site := m.selectedSite
	return m.actionCmd("load .env", func(ctx context.Context, c daemon.Caller) (string, error) {
		var out daemon.SiteResult
		if err := c.Call(ctx, daemon.ActionSite, daemon.SiteRequest{Op: "env", Name: site}, &out); err != nil {
			return "", err
		}
		m.envEntries = out.Env
		return "env loaded (masked)", nil
	})
}

func (m *Model) setDefault() tea.Cmd {
	switch m.page {
	case pageRuntimes:
		if len(m.snap.Status.PHP) > 0 {
			v := m.snap.Status.PHP[0].Minor
			for _, info := range m.snap.Status.PHP {
				if info.Minor == m.formVals.version || info.Version == m.formVals.version {
					v = info.Minor
				}
			}
			return m.callCmd("default PHP "+v, daemon.ActionPHPUse, daemon.PHPUseRequest{Minor: v})
		}
	case pageDatabases:
		id := m.selectedService
		if id == "" {
			id = m.snap.Status.Defaults["mariadb"]
		}
		if id != "" {
			return m.callCmd("default DB "+id, daemon.ActionDBDefault, daemon.DBDefaultRequest{ID: id})
		}
	case pageServices:
		id := m.selectedService
		version := firstOr(m.snap.Status.Services[id].Installed, "")
		if id != "" && version != "" {
			return m.callCmd("use "+id+" "+version, daemon.ActionServiceUse, daemon.ServiceUseRequest{ID: id, Version: version})
		}
	}
	return nil
}
