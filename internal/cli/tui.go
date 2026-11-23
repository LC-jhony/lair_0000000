package cli

import (
	_ "embed"
	"errors"
	"fmt"
	"io"
	"math"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"charm.land/bubbles/v2/help"
	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/glamour/v2"
	"charm.land/huh/v2"
	"charm.land/lipgloss/v2"
	charmlog "charm.land/log/v2"
	"github.com/charmbracelet/harmonica"
	"github.com/lair-sh/lair/internal/daemon"
	"github.com/lair-sh/lair/internal/engine"
	"github.com/lair-sh/lair/internal/installer"
	"github.com/lair-sh/lair/internal/php"
	"github.com/lair-sh/lair/internal/project"
	"github.com/lair-sh/lair/internal/services"
)

// tuiHelpMD is the single source of truth for the in-TUI help (rendered with
// Glamour), the CLI documentation (viewable with Glow) and the plain-text help
// printed by `lair tui --help`.
//
//go:embed tui_help.md
var tuiHelpMD string

const (
	helpStartMarker = "<!-- help:start -->"
	helpEndMarker   = "<!-- help:end -->"
)

// tuiHelp is not a second copy of the documentation: it is extracted from the
// embedded Markdown block delimited by the markers above. This keeps `--help`
// and the in-app help from drifting apart.
var tuiHelp = helpFromMarkdown(tuiHelpMD)

func helpFromMarkdown(md string) string {
	start := strings.Index(md, helpStartMarker)
	end := strings.Index(md, helpEndMarker)
	if start < 0 || end <= start+len(helpStartMarker) {
		return md
	}
	text := md[start+len(helpStartMarker) : end]
	return strings.Trim(text, "\n") + "\n"
}

// runTUI starts the interactive Charm dashboard. When stdin is not a terminal
// (CI, pipes, tests) it prints a one-shot textual dashboard instead of leaving
// a stuck full-screen program behind.
func runTUI(args []string, out, errOut io.Writer) error {
	for _, a := range args {
		if isHelp(a) {
			fmt.Fprint(out, tuiHelp)
			return nil
		}
	}
	c, err := newCtx(out, errOut)
	if err != nil {
		return err
	}
	// The React/Ink dashboard built with termcn is the primary interface: it
	// mirrors the desktop app pane by pane and uses the same daemon actions.
	// It allocates a PTY through script(1) when stdin is not a terminal, so it
	// also works from IDE shells and command runners.
	if err := runReactTUI(out, errOut); err == nil {
		return nil
	} else {
		fmt.Fprintf(errOut, "lair: dashboard termcn no disponible (%v); usando la TUI Go.\n", err)
	}

	if !isTerminal(os.Stdin) {
		return printDashboard(c, out)
	}

	m := newTUIModel(c)
	defer m.closeLog()
	p := tea.NewProgram(m, tea.WithInput(os.Stdin), tea.WithOutput(out))
	if _, err := p.Run(); err != nil {
		// Some hosts advertise a TTY but cannot provide a cancel reader (CI,
		// nested terminals). Never leave the caller stuck or without data.
		return printDashboard(c, out)
	}
	return nil
}

// printDashboard is the non-interactive fallback: the same data, plain text.
func printDashboard(c *ctx, out io.Writer) error {
	var st engine.Status
	if err := c.call(daemon.ActionStatus, nil, &st); err != nil {
		return err
	}
	fmt.Fprintf(out, "Lair · %s\n", st.Home)
	fmt.Fprintf(out, "PHP:     %s (default: %s)\n", joinMinors(st.PHP), st.Defaults["php"])
	fmt.Fprintf(out, "Node:    %s (default: %s)\n", strings.Join(st.Node, ", "), st.Defaults["node"])
	defaultDB := st.Defaults["database"]
	if defaultDB == "" {
		defaultDB = engine.DefaultDatabaseID
	}
	fmt.Fprintf(out, "BD:      %s\n", defaultDB)
	ids := make([]string, 0, len(st.Services))
	for id := range st.Services {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		v := st.Services[id]
		state := "stopped"
		if v.Running {
			state = "running"
		}
		fmt.Fprintf(out, "  %-12s %-10s port=%-6d %s\n", id, orDash(v.Active), v.Port, state)
	}
	fmt.Fprintf(out, "Sitios:  %d\n", len(st.Sites))
	if len(st.Jobs) > 0 {
		fmt.Fprintf(out, "Trabajos:\n")
		for _, j := range st.Jobs {
			fmt.Fprintf(out, "  %s: %s\n", j.ID, j.Phase)
		}
	}
	return nil
}

func joinMinors(list []php.Info) string {
	parts := make([]string, 0, len(list))
	for _, p := range list {
		parts = append(parts, p.Minor)
	}
	if len(parts) == 0 {
		return "(ninguna)"
	}
	return strings.Join(parts, ", ")
}

// ---------- Styling ----------

var (
	styleLogo     = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("205"))
	styleTitle    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("205"))
	styleSubtle   = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
	styleOK       = lipgloss.NewStyle().Foreground(lipgloss.Color("42"))
	styleWarn     = lipgloss.NewStyle().Foreground(lipgloss.Color("214"))
	styleErr      = lipgloss.NewStyle().Foreground(lipgloss.Color("203"))
	stylePaneName = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("230")).Background(lipgloss.Color("63")).Padding(0, 1)
	styleNav      = lipgloss.NewStyle().Foreground(lipgloss.Color("252")).Padding(0, 1)
	styleNavOn    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("230")).Background(lipgloss.Color("63")).Padding(0, 1)
	styleSidebar  = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("63")).Padding(1, 1)
	styleMain     = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("238")).Padding(1, 1)
	styleCard     = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("238")).Padding(0, 1)
	styleCardOn   = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("205")).Padding(0, 1)
	styleToggleOn = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("234")).Background(lipgloss.Color("42")).Padding(0, 1)
	styleToggleOf = lipgloss.NewStyle().Foreground(lipgloss.Color("245")).Background(lipgloss.Color("238")).Padding(0, 1)
	styleDialog   = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("205")).Background(lipgloss.Color("235")).Padding(1, 2)
	styleBarOn    = lipgloss.NewStyle().Foreground(lipgloss.Color("42"))
	styleBarOff   = lipgloss.NewStyle().Foreground(lipgloss.Color("238"))
)

// ---------- Panes ----------

type paneID int

const (
	paneOverview paneID = iota
	paneRuntimes
	paneDatabases
	paneServices
	paneSites
	paneSystem
	paneMail
	paneDumps
	paneDebugger
)

// paneIndex and paneAt are the only source of truth for navigation order.
// paneID is just an identifier: its numeric value must never be used for
// next/previous or sidebar highlighting (the enum and paneOrder differ).
func paneIndex(id paneID) int {
	for i, candidate := range paneOrder {
		if candidate == id {
			return i
		}
	}
	return -1
}

func paneAt(index int) paneID {
	if len(paneOrder) == 0 {
		return paneOverview
	}
	wrapped := ((index % len(paneOrder)) + len(paneOrder)) % len(paneOrder)
	return paneOrder[wrapped]
}

func paneNumber(id paneID) int {
	if i := paneIndex(id); i >= 0 {
		return i + 1
	}
	return 0
}

type paneInfo struct {
	icon     string
	title    string
	subtitle string
}

var (
	paneOrder = []paneID{
		paneOverview, paneRuntimes, paneDatabases, paneServices, paneSites,
		paneMail, paneDumps, paneDebugger, paneSystem,
	}
	paneInfos = map[paneID]paneInfo{
		paneOverview:  {"◈", "Resumen", "estado general del entorno"},
		paneRuntimes:  {"⬡", "Runtimes", "versiones instaladas de PHP y Node"},
		paneDatabases: {"◫", "Bases de datos", "motores disponibles y valores por defecto"},
		paneServices:  {"▤", "Servicios", "activa o desactiva cada servicio"},
		paneSites:     {"◎", "Sitios", "proyectos aparcados en el entorno"},
		paneMail:      {"✉", "Correo", "servidor SMTP de pruebas y bandeja"},
		paneDumps:     {"≡", "Dumps", "capturas de dump del entorno"},
		paneDebugger:  {"⚗", "Debugger", "depuración PHP y dominios"},
		paneSystem:    {"⚙", "Sistema", "catálogo y enlaces"},
	}
)

// ---------- Messages ----------

type statusMsg struct {
	st  engine.Status
	err error
}

// catalogMsg refreshes the version catalog so installs request a concrete
// version, exactly like the desktop app. catalogEntryMsg is produced by the
// per-package commands batched by loadCatalog.
type catalogMsg struct {
	catalog map[string][]string
	err     error
}

type catalogEntryMsg struct {
	pkg      string
	versions []string
	err      error
}

// projectInfoMsg carries the capabilities reported by `project info`.
type projectInfoMsg struct {
	info project.Info
	err  error
}

// projectDirsMsg carries the reply of `project dirs` into the event loop. The
// list is optional so an older daemon that does not report it simply leaves
// the dialog empty instead of breaking the dashboard.
type projectDirsMsg struct {
	dirs []string
	err  error
}

type actionMsg struct {
	label string
	err   error
}

// jobMsg starts polling an asynchronous installer job; pollMsg schedules the
// next check via tea.Tick so the event loop is never blocked by time.Sleep.
type jobMsg struct {
	label string
	id    string
}

// pollMsg reschedules a job check. attempt bounds the total number of checks
// and transport counts consecutive transient transport failures, so a flaky
// socket cannot spin forever.
type pollMsg struct {
	label     string
	id        string
	attempt   int
	transport int
	progress  *installer.Status
}

type serviceMsg struct {
	id    string
	start bool
	verb  string
	err   error
}

type frameMsg struct{}

const (
	pollInterval = 150 * time.Millisecond
	frameEvery   = 50 * time.Millisecond

	// maxPollAttempts * pollInterval is roughly a minute of waiting for a job.
	maxPollAttempts = 400
	maxPollRetries  = 5
)

// ---------- Key map ----------

type tuiKeyMap struct {
	Next, Prev, Up, Down      key.Binding
	Toggle, Refresh           key.Binding
	Node, PHP, Database       key.Binding
	Install                   key.Binding
	Catalog, Services         key.Binding
	Defaults, Logs, CancelJob key.Binding
	Park, Link, Unlink        key.Binding
	SiteDetail, DeleteSite    key.Binding
	SiteVhost, SiteToggle     key.Binding
	Secure, Pin, Isolate      key.Binding
	Project, ProjectDirs      key.Binding
	Mail, Dumps, Debugger     key.Binding
	UseVersion, Uninstall     key.Binding
	CreateDB                  key.Binding
	Help, Quit                key.Binding
}

func (k tuiKeyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Next, k.Refresh, k.SiteDetail, k.SiteVhost, k.DeleteSite, k.SiteToggle, k.Mail, k.Dumps, k.Debugger, k.Help, k.Quit}
}

func (k tuiKeyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{k.Next, k.Prev, k.Up, k.Down, k.Toggle},
		{k.Refresh, k.Help, k.Node, k.PHP, k.Database, k.Install, k.Catalog, k.Services},
		{k.Defaults, k.Logs, k.CancelJob, k.Park, k.Link, k.Unlink, k.SiteDetail, k.SiteVhost, k.SiteToggle, k.DeleteSite},
		{k.Mail, k.Dumps, k.Debugger},
		{k.Secure, k.Project, k.ProjectDirs, k.Pin, k.Isolate, k.UseVersion, k.Uninstall, k.CreateDB, k.Quit},
	}
}

var tuiKeys = tuiKeyMap{
	Next:        key.NewBinding(key.WithKeys("tab", "right", "l"), key.WithHelp("tab/→", "sección")),
	Prev:        key.NewBinding(key.WithKeys("left", "h"), key.WithHelp("←", "anterior")),
	Up:          key.NewBinding(key.WithKeys("up", "k"), key.WithHelp("↑", "arriba")),
	Down:        key.NewBinding(key.WithKeys("down", "j"), key.WithHelp("↓", "abajo")),
	Toggle:      key.NewBinding(key.WithKeys("enter", " "), key.WithHelp("enter", "activar/desactivar")),
	Refresh:     key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "refrescar")),
	Node:        key.NewBinding(key.WithKeys("n"), key.WithHelp("n", "Node")),
	PHP:         key.NewBinding(key.WithKeys("p"), key.WithHelp("p", "PHP")),
	Database:    key.NewBinding(key.WithKeys("m"), key.WithHelp("m", "base de datos")),
	Install:     key.NewBinding(key.WithKeys("I"), key.WithHelp("I", "instalar servicio")),
	Catalog:     key.NewBinding(key.WithKeys("u"), key.WithHelp("u", "catálogo")),
	Services:    key.NewBinding(key.WithKeys("s"), key.WithHelp("s", "reiniciar servicios")),
	Defaults:    key.NewBinding(key.WithKeys("d"), key.WithHelp("d", "predeterminada")),
	Logs:        key.NewBinding(key.WithKeys("v"), key.WithHelp("v", "ver logs")),
	CancelJob:   key.NewBinding(key.WithKeys("x"), key.WithHelp("x", "cancelar trabajo")),
	Park:        key.NewBinding(key.WithKeys("a"), key.WithHelp("a", "aparcar ruta")),
	Link:        key.NewBinding(key.WithKeys("e"), key.WithHelp("e", "enlazar sitio")),
	Unlink:      key.NewBinding(key.WithKeys("o"), key.WithHelp("o", "desvincular")),
	SiteDetail:  key.NewBinding(key.WithKeys("E"), key.WithHelp("E", "detalle del sitio")),
	SiteVhost:   key.NewBinding(key.WithKeys("V"), key.WithHelp("V", "ver vhost del sitio")),
	SiteToggle:  key.NewBinding(key.WithKeys("t"), key.WithHelp("t", "alternar enabled del sitio")),
	DeleteSite:  key.NewBinding(key.WithKeys("X"), key.WithHelp("X", "borrar sitio")),
	Secure:      key.NewBinding(key.WithKeys("c"), key.WithHelp("c", "certificado")),
	Project:     key.NewBinding(key.WithKeys("C"), key.WithHelp("C", "nuevo proyecto")),
	ProjectDirs: key.NewBinding(key.WithKeys("F"), key.WithHelp("F", "gestionar carpetas de proyectos")),
	Pin:         key.NewBinding(key.WithKeys("g"), key.WithHelp("g", "fijar versión")),
	Isolate:     key.NewBinding(key.WithKeys("i"), key.WithHelp("i", "aislar sitio")),
	Mail:        key.NewBinding(key.WithKeys("M"), key.WithHelp("M", "correo / SMTP")),
	Dumps:       key.NewBinding(key.WithKeys("D"), key.WithHelp("D", "dumps")),
	Debugger:    key.NewBinding(key.WithKeys("B"), key.WithHelp("B", "debugger")),
	UseVersion:  key.NewBinding(key.WithKeys("y"), key.WithHelp("y", "usar versión")),
	Uninstall:   key.NewBinding(key.WithKeys("z"), key.WithHelp("z", "desinstalar")),
	CreateDB:    key.NewBinding(key.WithKeys("b"), key.WithHelp("b", "crear BD")),
	Help:        key.NewBinding(key.WithKeys("?"), key.WithHelp("?", "ayuda")),
	Quit:        key.NewBinding(key.WithKeys("q", "ctrl+c", "esc"), key.WithHelp("q", "salir")),
}

// ---------- Model ----------

type tuiModel struct {
	c        *ctx
	keys     tuiKeyMap
	help     help.Model
	viewport viewport.Model
	spinner  spinner.Model
	logger   *charmlog.Logger
	logFile  *os.File
	spring   harmonica.Spring

	barPos, barVel, barTarget float64

	pane              paneID
	selected          int
	siteSelected      int
	status            engine.Status
	statusErr         error
	catalog           map[string][]string
	catalogPending    int
	catalogErr        error
	projectInfo       project.Info
	projectDirs       []string // last `project dirs` reply
	projectDirsLoaded bool
	projectDirsErr    error
	dirsDirty         bool // a dirs-changing op finished: reload the list
	loaded            bool
	busy              string
	message           string
	jobProgress       *installer.Status

	form            *huh.Form
	confirmValue    bool
	pendingID       string
	pendingStart    bool
	dialog          string
	formText        string
	formText2       string
	formTool        string
	siteDeleteFiles bool
	projectDB       string
	projectStack    string
	projectTest     string
	projectNode     string
	projectDBName   string
	projectDBUser   string
	projectDBPass   string
	projectSecure   bool
	projectCreateDB bool
	projectStart    bool
	logText         string
	logTitle        string
	logNameKeys     map[string]string // logs-list label -> log name
	siteLogFilter   string            // when set, logs are filtered to one site

	// Site detail is assembled from the three `site` ops. Each response is
	// merged into siteDetailData; pending counts the replies still in flight.
	siteDetailName    string
	siteDetailData    siteResult
	siteDetailErr     error
	siteDetailPending int
	siteEnabled       bool // last known `detail.enabled` of the open site
	siteConfigView    bool // the scrollable viewer is showing a generated vhost

	// Tooling panes. Each has its own async state so a slow daemon never
	// blocks the event loop or the other panes.
	mail           mailResult
	mailLoaded     bool
	mailErr        error
	mailMessageIDs map[string]string // message label -> id
	dumps          dumpsResult
	dumpsLoaded    bool
	dumpsErr       error
	debugger       debuggerResult
	debuggerLoaded bool
	debuggerErr    error
	detailTitle    string
	detailText     string

	showHelp      bool
	helpMarkdown  string
	helpMarkdownW int

	width, height   int
	sidebarW, mainW int
	bodyH           int
}

func newTUIModel(c *ctx) *tuiModel {
	s := spinner.New(spinner.WithSpinner(spinner.Dot), spinner.WithStyle(styleOK))
	vp := viewport.New(viewport.WithWidth(70), viewport.WithHeight(12))
	vp.SoftWrap = true
	vp.MouseWheelEnabled = true

	logger := charmlog.NewWithOptions(io.Discard, charmlog.Options{Level: charmlog.InfoLevel})
	var logFile *os.File
	if c.paths.Log != "" {
		_ = os.MkdirAll(c.paths.Log, 0o755)
		if f, err := os.OpenFile(filepath.Join(c.paths.Log, "tui.log"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644); err == nil {
			logFile = f
			logger = charmlog.NewWithOptions(f, charmlog.Options{
				Level:           charmlog.InfoLevel,
				Prefix:          "lair-tui",
				ReportTimestamp: true,
			})
		}
	}

	m := &tuiModel{
		c:           c,
		keys:        tuiKeys,
		help:        help.New(),
		viewport:    vp,
		spinner:     s,
		logger:      logger,
		logFile:     logFile,
		spring:      harmonica.NewSpring(harmonica.FPS(20), 8, 0.4),
		catalog:     map[string][]string{},
		message:     "Listo",
		siteEnabled: true,
	}
	m.status = engine.Status{Services: map[string]engine.ServiceView{}}
	m.setSize(100, 30)
	m.logger.Info("dashboard started")
	return m
}

// closeLog flushes and closes the dashboard log file. It is safe to call more
// than once (the TUI also closes it on tea.Quit).
func (m *tuiModel) closeLog() {
	if m.logFile != nil {
		_ = m.logFile.Close()
		m.logFile = nil
	}
}

// setSize recomputes the sidebar/main/footer layout and child component sizes.
func (m *tuiModel) setSize(w, h int) {
	if w < 60 {
		w = 60
	}
	if h < 16 {
		h = 16
	}
	m.width, m.height = w, h

	sidebarW := w / 4
	if sidebarW < 22 {
		sidebarW = 22
	}
	if sidebarW > 30 {
		sidebarW = 30
	}
	m.sidebarW = sidebarW
	m.mainW = w - sidebarW - 1
	m.bodyH = h - 3

	m.help.SetWidth(w)
	vpW := m.mainW - 6
	if vpW < 20 {
		vpW = 20
	}
	vpH := m.bodyH - 6
	if vpH < 3 {
		vpH = 3
	}
	m.viewport.SetWidth(vpW)
	m.viewport.SetHeight(vpH)
	// Glamour rendering is comparatively expensive; only redo it when the
	// available width actually changes.
	if m.helpMarkdown == "" || m.helpMarkdownW != vpW {
		m.helpMarkdown = m.renderHelpMarkdown(vpW)
		m.helpMarkdownW = vpW
	}
}

func (m *tuiModel) renderHelpMarkdown(width int) string {
	tr, err := glamour.NewTermRenderer(
		glamour.WithStandardStyle("dark"),
		glamour.WithWordWrap(width-2),
	)
	if err != nil {
		return tuiHelpMD
	}
	defer tr.Close()
	out, err := tr.Render(tuiHelpMD)
	if err != nil {
		return tuiHelpMD
	}
	return out
}

func (m *tuiModel) Init() tea.Cmd {
	// No spinner tick here: it is only started when an action actually begins
	// (see start/applyToggle) and stopped as soon as busy is empty again.
	return tea.Batch(m.refresh(), m.loadCatalog(), m.loadProjectInfo(), m.loadProjectDirs())
}

// loadCatalog fans out one command per package so a slow mirror cannot delay
// all the other choices. Each closure captures its own caller and package name.
func (m *tuiModel) loadCatalog() tea.Cmd {
	packages := append([]string{"php", "node"}, services.IDs()...)
	if m.catalog == nil {
		m.catalog = map[string][]string{}
	}
	m.catalogErr = nil
	m.catalogPending = len(packages)
	cmds := make([]tea.Cmd, 0, len(packages))
	for _, name := range packages {
		c := m.c
		pkg := name
		cmds = append(cmds, func() tea.Msg {
			var result daemon.VersionResult
			err := c.call(daemon.ActionVersions, daemon.VersionRequest{Package: pkg}, &result)
			return catalogEntryMsg{pkg: pkg, versions: result.Versions, err: cleanErr(err)}
		})
	}
	return tea.Batch(cmds...)
}

func (m *tuiModel) loadProjectInfo() tea.Cmd {
	c := m.c
	return func() tea.Msg {
		var result daemon.ProjectResult
		err := c.call(daemon.ActionProject, daemon.ProjectRequest{Op: "info"}, &result)
		return projectInfoMsg{info: result.Info, err: cleanErr(err)}
	}
}

func (m *tuiModel) catalogVersions(pkg string) []string {
	return m.catalog[pkg]
}

func (m *tuiModel) latestPHP() string {
	if versions := m.catalogVersions("php"); len(versions) > 0 {
		return versions[0]
	}
	return ""
}

func (m *tuiModel) latestNode() string {
	if versions := m.catalogVersions("node"); len(versions) > 0 {
		return versions[0]
	}
	return ""
}

func (m *tuiModel) refresh() tea.Cmd {
	c := m.c
	return func() tea.Msg {
		var st engine.Status
		err := c.call(daemon.ActionStatus, nil, &st)
		return statusMsg{st: st, err: cleanErr(err)}
	}
}

// paneLoadCmd loads the state of the pane currently on screen. It is called on
// every pane change and on refresh; all work happens in tea.Cmd closures so
// Update stays non-blocking.
func (m *tuiModel) paneLoadCmd() tea.Cmd {
	switch m.pane {
	case paneMail:
		return m.loadMail()
	case paneDumps:
		return m.loadDumps()
	case paneDebugger:
		return m.loadDebugger()
	}
	return nil
}

// refreshAll refreshes the dashboard and all three tooling panes at once.
func (m *tuiModel) refreshAll() tea.Cmd {
	return tea.Batch(m.refresh(), m.loadMail(), m.loadDumps(), m.loadDebugger())
}

// ansiEscapeRE matches CSI, OSC and two-character escape sequences that could
// be injected through a daemon message. stripControl keeps only printable text
// (plus line breaks and tabs) before anything reaches Glamour/Lip Gloss.
var ansiEscapeRE = regexp.MustCompile(`\x1b(?:\[[0-?]*[ -/]*[@-~]|\][^\x07\x1b]*(?:\x07|\x1b\\)|[()#][0-9A-Za-z]?|[@>8MD])`)

func stripControl(s string) string {
	s = ansiEscapeRE.ReplaceAllString(s, "")
	return strings.Map(func(r rune) rune {
		switch r {
		case '\n', '\t':
			return r
		}
		if r < 0x20 || r == 0x7f {
			return -1
		}
		return r
	}, s)
}

// cleanErr sanitizes an error coming from the daemon before it is displayed.
func cleanErr(err error) error {
	if err == nil {
		return nil
	}
	text := stripControl(err.Error())
	if text == "" {
		text = "error desconocido"
	}
	return errors.New(text)
}

// isTransientPollErr distinguishes a temporarily unreachable daemon from an
// action that the daemon actually rejected. Transient failures are retried a
// bounded number of times instead of failing the whole job.
func isTransientPollErr(err error) bool {
	if err == nil {
		return false
	}
	var netErr net.Error
	if errors.As(err, &netErr) {
		return true
	}
	text := strings.ToLower(err.Error())
	for _, token := range []string{
		"timeout", "timed out", "deadline exceeded", "connection reset",
		"connection refused", "broken pipe", "unexpected eof", "eof",
		"temporarily unavailable", "connect to daemon", "no daemon socket configured",
	} {
		if strings.Contains(text, token) {
			return true
		}
	}
	return false
}

// isCatalogError reports whether the daemon rejected an action because its
// signed catalog was missing or empty. An old daemon process can survive an
// upgrade, so the TUI asks it to refresh and retries before surfacing the
// error to the user.
func isCatalogError(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "catalog is empty") || strings.Contains(msg, "no catalog loaded")
}

// startAction launches an installer action. If the daemon returns a job ID,
// it hands off to pollJob; otherwise it reports completion immediately. A job
// that is already done with an error or cancellation must never refresh shims.
func (m *tuiModel) startAction(label, action string, payload any) tea.Cmd {
	c := m.c
	return func() tea.Msg {
		var raw installer.Status
		err := c.call(action, payload, &raw)
		if isCatalogError(err) {
			// The daemon owns the catalog: ask it to refresh, then retry once.
			if updateErr := c.call(daemon.ActionUpdate, nil, &struct{}{}); updateErr == nil {
				raw = installer.Status{}
				err = c.call(action, payload, &raw)
			}
		}
		if err != nil {
			return actionMsg{label: label, err: cleanErr(err)}
		}
		if raw.Done {
			// Cancellation often comes with a context error; report the
			// cancellation first, as the daemon does in `project info`.
			if raw.Canceled {
				return actionMsg{label: label, err: errors.New(stripControl(label) + ": operación cancelada")}
			}
			if raw.Error != "" {
				return actionMsg{label: label, err: errors.New(stripControl(raw.Error))}
			}
		}
		if raw.ID != "" && !raw.Done {
			return jobMsg{label: label, id: stripControl(raw.ID)}
		}
		m.ensureShimsSilently()
		return actionMsg{label: label}
	}
}

// pollJob checks a job after one tick and re-schedules itself with another
// tea.Tick while it is still running, keeping Update non-blocking. Both the
// number of checks and the number of consecutive transport retries are bounded.
func (m *tuiModel) pollJob(label, id string, attempt, transport int) tea.Cmd {
	c := m.c
	return tea.Tick(pollInterval, func(time.Time) tea.Msg {
		if attempt >= maxPollAttempts {
			return actionMsg{label: label, err: fmt.Errorf("tiempo de espera agotado esperando el trabajo %s", stripControl(id))}
		}
		var st installer.Status
		if err := c.call(daemon.ActionJob, daemon.JobRequest{ID: id}, &st); err != nil {
			if isTransientPollErr(err) && transport < maxPollRetries {
				return pollMsg{label: label, id: id, attempt: attempt + 1, transport: transport + 1}
			}
			return actionMsg{label: label, err: cleanErr(err)}
		}
		if st.Done {
			if st.Canceled {
				return actionMsg{label: label, err: errors.New(stripControl(label) + ": trabajo cancelado")}
			}
			if st.Error != "" {
				return actionMsg{label: label, err: errors.New(stripControl(st.Error))}
			}
			m.ensureShimsSilently()
			return actionMsg{label: label}
		}
		progress := st
		return pollMsg{label: label, id: id, attempt: attempt + 1, transport: transport, progress: &progress}
	})
}

func (m *tuiModel) nextFrame() tea.Cmd {
	return tea.Tick(frameEvery, func(time.Time) tea.Msg { return frameMsg{} })
}

// ---------- Update ----------

func (m *tuiModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	model, cmd := m.update(msg)
	// Keep View() pure: refresh the viewport content here, after every state
	// change (pane, status, log, help or form), never while rendering.
	m.syncViewport()
	return model, cmd
}

func (m *tuiModel) update(msg tea.Msg) (tea.Model, tea.Cmd) {
	// An open form owns key and mouse events only. Business messages must keep
	// reaching the main switch, otherwise a status/action/job reply arriving
	// while a dialog is open would be silently swallowed.
	if m.form != nil {
		switch msg.(type) {
		case tea.KeyPressMsg, tea.MouseMsg:
			return m.updateForm(msg)
		}
	}
	return m.updateMain(msg)
}

func (m *tuiModel) updateForm(msg tea.Msg) (tea.Model, tea.Cmd) {
	// Huh only maps ctrl+c to abort by default; keep Esc consistent with
	// the rest of the dashboard.
	if k, ok := msg.(tea.KeyPressMsg); ok && k.String() == "esc" {
		m.form, m.pendingID, m.dialog = nil, "", ""
		m.message = styleSubtle.Render("cambio cancelado")
		return m, nil
	}
	updated, cmd := m.form.Update(msg)
	if f, ok := updated.(*huh.Form); ok {
		m.form = f
	}
	switch m.form.State {
	case huh.StateCompleted:
		dialog := m.dialog
		accepted := m.confirmValue
		id, start := m.pendingID, m.pendingStart
		m.form, m.pendingID, m.dialog = nil, "", ""
		if dialog == "toggle" && !accepted {
			m.message = styleSubtle.Render("cambio cancelado")
			return m, nil
		}
		if dialog == "deleteSite" && !accepted {
			m.message = styleSubtle.Render("borrado cancelado")
			return m, nil
		}
		if dialog == "siteToggle" && !accepted {
			m.message = styleSubtle.Render("cambio cancelado")
			return m, nil
		}
		if dialog == "toggle" {
			return m.applyToggle(id, start)
		}
		if dialog == "siteToggle" {
			return m.applySiteToggle(id, start)
		}
		return m, m.applyDialog(dialog)
	case huh.StateAborted:
		m.form, m.pendingID, m.dialog = nil, "", ""
		m.message = styleSubtle.Render("cambio cancelado")
		return m, nil
	}
	return m, cmd
}

// isActionKey reports keys that would start new work. Navigation, help and
// scrolling stay available while an action is in flight.
func (m *tuiModel) isActionKey(msg tea.KeyPressMsg) bool {
	k := m.keys
	for _, binding := range []key.Binding{
		k.Toggle, k.Node, k.PHP, k.Database, k.Install, k.Catalog, k.Services,
		k.Defaults, k.CancelJob, k.Park, k.Link, k.Unlink, k.SiteDetail, k.SiteVhost,
		k.SiteToggle, k.DeleteSite, k.Secure, k.Project, k.ProjectDirs, k.Pin, k.Isolate,
		k.Mail, k.Dumps, k.Debugger,
		k.UseVersion, k.Uninstall, k.CreateDB,
	} {
		if key.Matches(msg, binding) {
			return true
		}
	}
	return false
}

func (m *tuiModel) busyNotice() (tea.Model, tea.Cmd) {
	m.message = styleSubtle.Render("espera a que termine la acción en curso")
	return m, nil
}

// goToPane changes the active pane through paneOrder, resets the scroll and
// reloads only when the pane actually changed.
func (m *tuiModel) goToPane(next paneID) tea.Cmd {
	if next == m.pane {
		return nil
	}
	m.pane = next
	m.viewport.GotoTop()
	m.message = ""
	m.logger.Debug("pane changed", "pane", paneNumber(next))
	return m.paneLoadCmd()
}

// goToPaneNumber implements the 1-9 shortcuts shown in the sidebar.
func (m *tuiModel) goToPaneNumber(n int) tea.Cmd {
	if n < 1 || n > len(paneOrder) {
		return nil
	}
	return m.goToPane(paneAt(n - 1))
}

func (m *tuiModel) updateMain(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		if m.showHelp {
			switch {
			case key.Matches(msg, m.keys.Quit), key.Matches(msg, m.keys.Help):
				m.showHelp = false
				return m, nil
			}
			var cmd tea.Cmd
			m.viewport, cmd = m.viewport.Update(msg)
			return m, cmd
		}
		if (m.logText != "" || m.detailText != "") && (msg.String() == "esc" || msg.String() == "backspace") {
			m.logText, m.logTitle = "", ""
			m.detailText, m.detailTitle = "", ""
			m.siteDetailName, m.siteLogFilter = "", ""
			m.siteDetailData, m.siteDetailErr = siteResult{}, nil
			m.siteConfigView = false
			m.viewport.GotoTop()
			m.message = styleSubtle.Render("registro cerrado")
			return m, nil
		}
		// 1-9 jump directly to the pane shown in the sidebar (not while a
		// log/detail overlay is open).
		if k := msg.String(); m.logText == "" && m.detailText == "" && len(k) == 1 && k[0] >= '1' && k[0] <= '9' {
			return m, m.goToPaneNumber(int(k[0] - '0'))
		}

		// Reentrancy guard: never launch a second action while one is running.
		if m.busy != "" && m.isActionKey(msg) {
			return m.busyNotice()
		}

		switch {
		case key.Matches(msg, m.keys.Quit):
			m.logger.Info("dashboard closed")
			m.closeLog()
			return m, tea.Quit
		case key.Matches(msg, m.keys.Next):
			return m, m.goToPane(paneAt(paneIndex(m.pane) + 1))
		case key.Matches(msg, m.keys.Prev):
			return m, m.goToPane(paneAt(paneIndex(m.pane) - 1))
		case key.Matches(msg, m.keys.Help):
			m.showHelp = true
			m.viewport.GotoTop()
			return m, nil
		case key.Matches(msg, m.keys.Refresh):
			return m, m.refreshAll()
		case key.Matches(msg, m.keys.Up) && m.pane == paneServices:
			m.moveSelection(-1)
			return m, nil
		case key.Matches(msg, m.keys.Down) && m.pane == paneServices:
			m.moveSelection(1)
			return m, nil
		case key.Matches(msg, m.keys.Up) && m.pane == paneSites:
			m.moveSiteSelection(-1)
			return m, nil
		case key.Matches(msg, m.keys.Down) && m.pane == paneSites:
			m.moveSiteSelection(1)
			return m, nil
		case key.Matches(msg, m.keys.Toggle) && m.pane == paneServices:
			return m.askToggle()
		case key.Matches(msg, m.keys.Node):
			return m.askInstallNode()
		case key.Matches(msg, m.keys.PHP):
			return m.askInstallPHP()
		case key.Matches(msg, m.keys.Database):
			return m.askInstallDatabase()
		case key.Matches(msg, m.keys.Install):
			return m.askInstallService()
		case key.Matches(msg, m.keys.Catalog):
			return m.start("Actualizando catálogo…", "catálogo", daemon.ActionUpdate, nil)
		case key.Matches(msg, m.keys.Services):
			return m.start("Reiniciando servicios…", "servicios", daemon.ActionRestart, nil)
		case key.Matches(msg, m.keys.Defaults):
			return m.askDefault()
		case key.Matches(msg, m.keys.Logs):
			return m.askLogs()
		case key.Matches(msg, m.keys.CancelJob) && m.pane == paneSystem:
			return m.askCancelJob()
		case key.Matches(msg, m.keys.Park) && m.pane == paneSites:
			return m.askPark()
		case key.Matches(msg, m.keys.Link) && m.pane == paneSites:
			return m.askLink()
		case key.Matches(msg, m.keys.Unlink) && m.pane == paneSites:
			return m.askSiteAction("unlink")
		case key.Matches(msg, m.keys.SiteDetail) && m.pane == paneSites:
			return m.openSiteDetail()
		case key.Matches(msg, m.keys.SiteVhost) && m.pane == paneSites:
			return m.openSiteVhost()
		case key.Matches(msg, m.keys.SiteToggle) && m.pane == paneSites &&
			m.detailText != "" && m.siteDetailName != "" && !m.siteConfigView:
			return m.askSiteToggle()
		case key.Matches(msg, m.keys.DeleteSite) && m.pane == paneSites:
			return m.askDeleteSite()
		case key.Matches(msg, m.keys.Secure) && m.pane == paneSites:
			return m.askSiteAction("secure")
		case key.Matches(msg, m.keys.Project) && m.pane == paneSites:
			return m.askCreateProject()
		case key.Matches(msg, m.keys.ProjectDirs) && m.projectDirsPane():
			return m.askProjectDirs()
		case key.Matches(msg, m.keys.Pin) && m.pane == paneSites:
			return m.askPin()
		case key.Matches(msg, m.keys.Isolate) && m.pane == paneSites:
			return m.askIsolate()
		case key.Matches(msg, m.keys.Mail) && m.pane == paneMail:
			return m.askMail()
		case key.Matches(msg, m.keys.Dumps) && m.pane == paneDumps:
			return m.askDumps()
		case key.Matches(msg, m.keys.Debugger) && m.pane == paneDebugger:
			return m.askDebugger()
		case key.Matches(msg, m.keys.UseVersion):
			return m.askUseVersion()
		case key.Matches(msg, m.keys.Uninstall):
			return m.askUninstall()
		case key.Matches(msg, m.keys.CreateDB) && m.pane == paneDatabases:
			return m.askCreateDB()
		default:
			var cmd tea.Cmd
			m.viewport, cmd = m.viewport.Update(msg)
			return m, cmd
		}

	case tea.MouseMsg:
		var cmd tea.Cmd
		m.viewport, cmd = m.viewport.Update(msg)
		return m, cmd

	case spinner.TickMsg:
		// The tick chain stops as soon as there is no work in progress.
		if m.busy == "" {
			return m, nil
		}
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd

	case tea.WindowSizeMsg:
		m.setSize(msg.Width, msg.Height)

	case catalogMsg:
		if msg.catalog != nil {
			m.catalog = msg.catalog
		}
		if msg.err != nil {
			m.message = styleSubtle.Render("catálogo parcialmente disponible")
		}
		return m, nil

	case catalogEntryMsg:
		if msg.err != nil {
			if msg.pkg == "php" || msg.pkg == "node" {
				m.catalogErr = msg.err
			}
		} else if m.catalog != nil {
			m.catalog[msg.pkg] = msg.versions
		}
		if m.catalogPending > 0 {
			m.catalogPending--
		}
		if m.catalogPending == 0 && m.catalogErr != nil {
			m.message = styleSubtle.Render("catálogo parcialmente disponible")
		}
		return m, nil

	case projectInfoMsg:
		if msg.err != nil {
			m.logger.Warn("project info unavailable", "err", msg.err)
		} else {
			m.projectInfo = msg.info
		}
		// If capabilities arrived after the wizard was opened, rebuild it with
		// the filtered choices while preserving the values already typed.
		if m.dialog == "project" && m.form != nil {
			form := m.projectForm()
			m.form = form
			return m, form.Init()
		}
		return m, nil

	case projectDirsMsg:
		if msg.err != nil {
			m.projectDirsErr = msg.err
			m.message = styleErr.Render("carpetas de proyectos: " + stripControl(msg.err.Error()))
		} else {
			m.projectDirs, m.projectDirsErr, m.projectDirsLoaded = msg.dirs, nil, true
		}
		// The list may arrive after the dialog was opened; rebuild it in place
		// so the note shows the fresh dirs while keeping the chosen action.
		if m.dialog == "projectDirs" && m.form != nil {
			form := m.projectDirsForm()
			m.form = form
			return m, form.Init()
		}
		return m, nil

	case statusMsg:
		m.loaded = true
		m.statusErr = msg.err
		if msg.err == nil {
			m.status = msg.st
		}
		ids := m.serviceIDs()
		if m.selected >= len(ids) {
			m.selected = len(ids) - 1
		}
		if m.selected < 0 {
			m.selected = 0
		}
		if m.siteSelected >= len(m.status.Sites) {
			m.siteSelected = len(m.status.Sites) - 1
		}
		if m.siteSelected < 0 {
			m.siteSelected = 0
		}
		running, total := m.serviceCounts()
		if total > 0 {
			m.barTarget = float64(running) / float64(total)
		}
		return m, m.nextFrame()

	case jobMsg:
		return m, m.pollJob(msg.label, msg.id, 0, 0)
	case pollMsg:
		m.jobProgress = msg.progress
		return m, m.pollJob(msg.label, msg.id, msg.attempt, msg.transport)

	case logNamesMsg:
		if msg.err != nil {
			m.message = styleErr.Render("logs: " + stripControl(msg.err.Error()))
			return m, nil
		}
		items, sources := msg.items, msg.sources
		// `v` from a site detail asks for logs scoped to that site; fall back
		// to the full list when the daemon has no site-specific source.
		if filter := strings.ToLower(strings.TrimSpace(m.siteLogFilter)); filter != "" {
			m.siteLogFilter = ""
			matchedItems := make([]string, 0, len(items))
			for _, item := range items {
				if strings.Contains(strings.ToLower(item), filter) {
					matchedItems = append(matchedItems, item)
				}
			}
			matchedSources := make([]logSource, 0, len(sources))
			for _, source := range sources {
				haystack := strings.ToLower(source.Name + " " + source.ID + " " + source.Path + " " + strings.Join(source.Files, " "))
				if strings.Contains(haystack, filter) {
					matchedSources = append(matchedSources, source)
				}
			}
			if len(matchedItems)+len(matchedSources) > 0 {
				items, sources = matchedItems, matchedSources
			} else {
				m.message = styleSubtle.Render("sin registros específicos del sitio; se muestran todos")
			}
		}
		options := append([]string(nil), items...)
		m.logNameKeys = map[string]string{}
		for _, item := range items {
			m.logNameKeys[item] = item
		}
		// Newer daemons also report grouped sources. Show them with a richer
		// label while mapping the selection back to a `logs`-compatible name.
		for _, source := range sources {
			name := stripControl(source.Name)
			if name == "" {
				name = stripControl(source.ID)
			}
			if name == "" {
				name = stripControl(source.Path)
			}
			if name == "" {
				continue
			}
			label := stripControl(source.Kind)
			if label == "" {
				label = "fuente"
			}
			label += " · " + name
			if len(source.Files) > 0 {
				files := make([]string, 0, len(source.Files))
				for _, file := range source.Files {
					files = append(files, stripControl(file))
				}
				label += " (" + strings.Join(files, ", ") + ")"
			}
			options = append(options, label)
			m.logNameKeys[label] = name
		}
		if len(options) == 0 {
			m.message = styleSubtle.Render("no hay registros disponibles")
			return m, nil
		}
		m.formText = options[0]
		return m.openForm("logs", m.selectField("Registro", options, &m.formText))

	case logTextMsg:
		m.logTitle = "Registro · " + stripControl(msg.name)
		m.logText = strings.TrimSpace(stripControl(msg.text))
		if m.logText == "" {
			m.logText = "(vacío)"
		}
		m.viewport.GotoTop()
		m.message = styleSubtle.Render("esc / retroceso: volver")
		return m, nil

	case siteDetailMsg:
		return m.applySiteDetail(msg)

	case siteConfigMsg:
		return m.applySiteConfig(msg)

	case siteEnabledMsg:
		return m.applySiteEnabled(msg)

	case mailMsg:
		m.busy = ""
		if msg.err != nil {
			m.mailErr = msg.err
			m.message = styleErr.Render("correo: " + stripControl(msg.err.Error()))
			m.logger.Error("mail action failed", "op", msg.op, "err", msg.err)
			return m, nil
		}
		switch msg.op {
		case "status":
			m.mail.Running = msg.state.Running
			m.mail.Port = msg.state.Port
			m.mail.Count = msg.state.Count
			m.mail.Env = msg.state.Env
			m.mailLoaded, m.mailErr = true, nil
		case "list":
			m.mail.Messages = msg.state.Messages
			m.mailLoaded, m.mailErr = true, nil
		case "message":
			mm := msg.state.Message
			if mm == nil {
				m.message = styleSubtle.Render("mensaje no disponible")
				return m, nil
			}
			m.detailTitle = "Mensaje · " + orDash(stripControl(mm.Subject))
			m.detailText = renderMailMessage(mm)
			m.viewport.GotoTop()
			m.message = styleSubtle.Render("esc / retroceso: volver")
		default:
			m.message = styleOK.Render("correo: " + msg.op + " listo")
			return m, m.loadMail()
		}
		return m, nil

	case dumpsMsg:
		m.busy = ""
		if msg.err != nil {
			m.dumpsErr = msg.err
			m.message = styleErr.Render("dumps: " + stripControl(msg.err.Error()))
			m.logger.Error("dumps action failed", "op", msg.op, "err", msg.err)
			return m, nil
		}
		switch msg.op {
		case "status":
			m.dumps.Capturing = msg.state.Capturing
			m.dumps.Options = msg.state.Options
			m.dumps.Count = msg.state.Count
			m.dumpsLoaded, m.dumpsErr = true, nil
		case "list":
			m.dumps.Events = msg.state.Events
			m.dumpsLoaded, m.dumpsErr = true, nil
		default:
			m.message = styleOK.Render("dumps: " + msg.op + " listo")
			return m, m.loadDumps()
		}
		return m, nil

	case debuggerMsg:
		m.busy = ""
		if msg.err != nil {
			m.debuggerErr = msg.err
			m.message = styleErr.Render("debugger: " + stripControl(msg.err.Error()))
			m.logger.Error("debugger action failed", "op", msg.op, "err", msg.err)
			return m, nil
		}
		if msg.op == "status" {
			m.debugger = msg.state
			m.debuggerLoaded, m.debuggerErr = true, nil
			return m, nil
		}
		m.message = styleOK.Render("debugger: " + msg.op + " listo")
		return m, m.loadDebugger()

	case serviceMsg:
		m.busy = ""
		if msg.err != nil {
			m.message = styleErr.Render(stripControl(msg.id) + ": " + stripControl(msg.err.Error()))
			m.logger.Error("service toggle failed", "service", msg.id, "err", msg.err)
		} else {
			m.message = styleOK.Render(msg.id + " " + msg.verb)
			m.logger.Info("service toggle ok", "service", msg.id, "start", msg.start)
		}
		return m, m.refresh()

	case actionMsg:
		m.busy = ""
		m.jobProgress = nil
		if msg.err != nil {
			m.message = styleErr.Render(stripControl(msg.label) + ": " + stripControl(msg.err.Error()))
			m.logger.Error("action failed", "action", msg.label, "err", msg.err)
		} else {
			m.message = styleOK.Render(msg.label + ": listo")
			m.logger.Info("action ok", "action", msg.label)
		}
		cmds := []tea.Cmd{m.refresh()}
		if m.dirsDirty {
			m.dirsDirty = false
			cmds = append(cmds, m.loadProjectDirs())
		}
		return m, tea.Batch(cmds...)

	case frameMsg:
		m.barPos, m.barVel = m.spring.Update(m.barPos, m.barVel, m.barTarget)
		if math.Abs(m.barPos-m.barTarget) > 0.002 {
			return m, m.nextFrame()
		}
		m.barPos = m.barTarget
		return m, nil
	}

	var spinnerCmd, vpCmd tea.Cmd
	if m.busy != "" {
		m.spinner, spinnerCmd = m.spinner.Update(msg)
	}
	m.viewport, vpCmd = m.viewport.Update(msg)
	return m, tea.Batch(spinnerCmd, vpCmd)
}

// start sets the busy indicator and launches an action together with the
// spinner tick. The spinner only runs while busy is set.
func (m *tuiModel) start(busy, label, action string, payload any) (tea.Model, tea.Cmd) {
	if m.busy != "" {
		return m.busyNotice()
	}
	m.busy = busy
	m.logger.Info("action requested", "action", label)
	return m, tea.Batch(m.spinner.Tick, m.startAction(label, action, payload))
}

func (m *tuiModel) moveSelection(delta int) {
	ids := m.serviceIDs()
	if len(ids) == 0 {
		return
	}
	m.selected = (m.selected + delta + len(ids)) % len(ids)
}

func (m *tuiModel) askToggle() (tea.Model, tea.Cmd) {
	ids := m.serviceIDs()
	if len(ids) == 0 || m.selected >= len(ids) {
		return m, nil
	}
	id := ids[m.selected]
	running := m.status.Services[id].Running
	verb := "Iniciar"
	if running {
		verb = "Detener"
	}
	m.confirmValue = false
	m.pendingID = id
	m.pendingStart = !running
	m.dialog = "toggle"

	form := huh.NewForm(huh.NewGroup(
		huh.NewConfirm().
			Title(verb + " " + id + "?").
			Description("El daemon aplicará el cambio en el entorno local.").
			Affirmative("Sí, " + strings.ToLower(verb)).
			Negative("Cancelar").
			Value(&m.confirmValue),
	)).WithShowHelp(false).WithWidth(46).WithTheme(huh.ThemeFunc(huh.ThemeCharm))
	m.form = form
	return m, form.Init()
}

func (m *tuiModel) applyToggle(id string, start bool) (tea.Model, tea.Cmd) {
	if m.busy != "" {
		return m.busyNotice()
	}
	action := daemon.ActionServiceStop
	verb := "detenido"
	if start {
		action = daemon.ActionServiceRestart
		verb = "iniciado"
	}
	m.busy = "Aplicando cambio en " + id + "…"
	m.logger.Info("service toggle requested", "service", id, "start", start)
	c := m.c
	return m, tea.Batch(m.spinner.Tick, func() tea.Msg {
		err := c.call(action, daemon.ServiceRequest{ID: id}, nil)
		return serviceMsg{id: id, start: start, verb: verb, err: cleanErr(err)}
	})
}

// syncViewport is the single place that writes viewport content. It is called
// from Update so View() never mutates model state.
func (m *tuiModel) syncViewport() {
	if m.form != nil {
		// The dialog is rendered directly over the main pane; there is no
		// reason to rebuild the (hidden) scrollable content.
		return
	}
	content := m.renderPane()
	if m.showHelp {
		content = m.helpMarkdown
	}
	m.viewport.SetContent(content)
	if m.pane == paneServices && m.logText == "" && !m.showHelp {
		m.ensureSelectionVisible()
	}
}

// ensureSelectionVisible scrolls the service list so the selected card is not
// left outside the viewport after a navigation key.
func (m *tuiModel) ensureSelectionVisible() {
	ids := m.serviceIDs()
	if m.selected < 0 || m.selected >= len(ids) {
		return
	}
	line := 0
	for i := 0; i < m.selected; i++ {
		// Each card has a top and bottom border plus two info lines, and an
		// extra line for the last error when present. JoinVertical adds one
		// separating newline between cards.
		cardLines := 4
		if m.status.Services[ids[i]].LastError != "" {
			cardLines++
		}
		line += cardLines + 1
	}
	top, height := m.viewport.YOffset(), m.viewport.Height()
	switch {
	case line < top:
		m.viewport.SetYOffset(line)
	case line >= top+height:
		m.viewport.SetYOffset(line - height + 1)
	}
}

func (m *tuiModel) serviceIDs() []string {
	ids := make([]string, 0, len(m.status.Services))
	for id := range m.status.Services {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

func (m *tuiModel) serviceCounts() (running, total int) {
	for _, v := range m.status.Services {
		total++
		if v.Running {
			running++
		}
	}
	return running, total
}

// ---------- View ----------

func (m *tuiModel) View() tea.View {
	if m.width == 0 {
		return tea.NewView("Iniciando…")
	}
	body := lipgloss.JoinHorizontal(lipgloss.Top, m.renderSidebar(), " ", m.renderMain())
	v := tea.NewView(lipgloss.JoinVertical(lipgloss.Left, body, m.renderFooter()))
	v.AltScreen = true
	v.MouseMode = tea.MouseModeCellMotion
	return v
}

func (m *tuiModel) renderSidebar() string {
	lines := []string{
		styleLogo.Render("◆ LAIR"),
		styleSubtle.Render("entorno nativo PHP/Laravel"),
		"",
	}
	for i, id := range paneOrder {
		info := paneInfos[id]
		number := fmt.Sprintf("%d", i+1)
		if id == m.pane {
			lines = append(lines, styleNavOn.Render("▸ "+number+" "+info.icon+" "+info.title))
		} else {
			lines = append(lines, styleNav.Render("  "+number+" "+info.icon+" "+info.title))
		}
	}
	running, total := m.serviceCounts()
	lines = append(lines, "", styleSubtle.Render("─"+strings.Repeat("─", maxInt(m.sidebarW-6, 4))))
	lines = append(lines, styleSubtle.Render(fmt.Sprintf("  servicios  %d/%d", running, total)))
	lines = append(lines, styleSubtle.Render(fmt.Sprintf("  sitios     %d", len(m.status.Sites))))
	lines = append(lines, styleSubtle.Render(fmt.Sprintf("  trabajos   %d", len(m.status.Jobs))))
	return styleSidebar.Width(m.sidebarW - 4).Height(m.bodyH - 4).Render(strings.Join(lines, "\n"))
}

func (m *tuiModel) renderMain() string {
	info := paneInfos[m.pane]
	header := stylePaneName.Render(info.icon+" "+info.title) + "  " + styleSubtle.Render(info.subtitle)

	var body string
	if m.form != nil {
		dialog := styleDialog.Render(m.form.View())
		body = lipgloss.Place(m.viewport.Width(), m.viewport.Height(), lipgloss.Center, lipgloss.Center, dialog)
	} else {
		// Content is refreshed in Update (syncViewport), never here.
		body = m.viewport.View()
	}
	inner := lipgloss.JoinVertical(lipgloss.Left, header, "", body)
	return styleMain.Width(m.mainW - 4).Height(m.bodyH - 4).Render(inner)
}

func (m *tuiModel) renderPane() string {
	if !m.loaded {
		return styleSubtle.Render("Cargando estado…")
	}
	if m.statusErr != nil {
		return styleErr.Render("sin conexión: " + stripControl(m.statusErr.Error()))
	}
	if m.detailText != "" {
		return m.renderDetail()
	}
	if m.logText != "" {
		return styleCard.Width(m.viewport.Width() - 2).Render(strings.Join([]string{
			styleTitle.Render(m.logTitle),
			"",
			m.logText,
			"",
			styleSubtle.Render("esc / retroceso: volver"),
		}, "\n"))
	}
	switch m.pane {
	case paneRuntimes:
		return m.renderRuntimes()
	case paneDatabases:
		return m.renderDatabases()
	case paneServices:
		return m.renderServices()
	case paneSites:
		return m.renderSites()
	case paneMail:
		return m.renderMail()
	case paneDumps:
		return m.renderDumps()
	case paneDebugger:
		return m.renderDebugger()
	case paneSystem:
		return m.renderSystem()
	default:
		return m.renderOverview()
	}
}

// renderDetail shows the body of a selected mail message using the same
// scrollable card as the log viewer.
func (m *tuiModel) renderDetail() string {
	return styleCard.Width(m.viewport.Width() - 2).Render(strings.Join([]string{
		styleTitle.Render(m.detailTitle),
		"",
		m.detailText,
		"",
		styleSubtle.Render("esc / retroceso: volver"),
	}, "\n"))
}

// renderMail shows the SMTP test server status and the captured inbox.
func (m *tuiModel) renderMail() string {
	lines := []string{styleTitle.Render("Correo · SMTP de pruebas"), ""}
	if !m.mailLoaded {
		lines = append(lines, styleSubtle.Render("  (cargando…)"))
	} else if m.mailErr != nil {
		lines = append(lines, styleErr.Render("  sin conexión: "+stripControl(m.mailErr.Error())))
	} else {
		state := styleWarn.Render("detenido")
		if m.mail.Running {
			state = styleOK.Render("activo")
		}
		lines = append(lines,
			fmt.Sprintf("  estado    %s", state),
			fmt.Sprintf("  puerto    %d", m.mail.Port),
			fmt.Sprintf("  mensajes  %d", m.mail.Count),
			fmt.Sprintf("  entorno   %s", orDash(stripControl(m.mail.Env))),
			"",
			styleTitle.Render("Bandeja"),
			"",
		)
		if len(m.mail.Messages) == 0 {
			lines = append(lines, styleSubtle.Render("  (vacía)"))
		}
		for _, msg := range m.mail.Messages {
			subject := stripControl(msg.Subject)
			if subject == "" {
				subject = "(sin asunto)"
			}
			lines = append(lines, fmt.Sprintf("  %-8s %-28s %s",
				orDash(stripControl(msg.ID)), subject, stripControl(msg.From)))
		}
	}
	lines = append(lines, "", styleSubtle.Render("M ver mensaje / iniciar o detener SMTP / limpiar bandeja  ·  r refrescar"))
	return styleCard.Width(m.viewport.Width() - 2).Render(strings.Join(lines, "\n"))
}

// renderDumps shows dump capture status, enabled types and recent events.
func (m *tuiModel) renderDumps() string {
	lines := []string{styleTitle.Render("Dumps · capturas"), ""}
	if !m.dumpsLoaded {
		lines = append(lines, styleSubtle.Render("  (cargando…)"))
	} else if m.dumpsErr != nil {
		lines = append(lines, styleErr.Render("  sin conexión: "+stripControl(m.dumpsErr.Error())))
	} else {
		capture := styleToggleOf.Render("○ DESACTIVADA")
		if m.dumps.Capturing {
			capture = styleToggleOn.Render("● ACTIVA")
		}
		page := styleSubtle.Render("no")
		if m.dumps.Options.ShowInPage {
			page = styleOK.Render("sí")
		}
		lines = append(lines,
			fmt.Sprintf("  captura   %s", capture),
			fmt.Sprintf("  eventos   %d", m.dumps.Count),
			fmt.Sprintf("  en página %s", page),
			fmt.Sprintf("  tipos     %s", renderDumpTypes(m.dumps.Options)),
			"",
			styleTitle.Render("Eventos recientes"),
			"",
		)
		if len(m.dumps.Events) == 0 {
			lines = append(lines, styleSubtle.Render("  (ninguno)"))
		}
		for _, event := range m.dumps.Events {
			lines = append(lines, fmt.Sprintf("  %-7s %-14s %s",
				orDash(stripControl(event.Type)), orDash(stripControl(event.Site)), stripControl(event.Message)))
		}
	}
	lines = append(lines, "", styleSubtle.Render("D activar/desactivar captura, show_in_page o limpiar  ·  r refrescar"))
	return styleCard.Width(m.viewport.Width() - 2).Render(strings.Join(lines, "\n"))
}

func renderDumpTypes(opts dumpsOptions) string {
	all := []struct {
		name string
		on   bool
	}{
		{"queries", opts.Queries},
		{"logs", opts.Logs},
		{"views", opts.Views},
		{"jobs", opts.Jobs},
		{"requests", opts.Requests},
	}
	parts := make([]string, 0, len(all))
	for _, item := range all {
		if item.on {
			parts = append(parts, item.name)
		}
	}
	if len(parts) == 0 {
		return styleSubtle.Render("(ninguno)")
	}
	return strings.Join(parts, ", ")
}

// renderDebugger shows detection/breakpoint toggles, port/mode and the
// availability of each PHP version together with the configured domains.
func (m *tuiModel) renderDebugger() string {
	lines := []string{styleTitle.Render("Debugger · PHP"), ""}
	if !m.debuggerLoaded {
		lines = append(lines, styleSubtle.Render("  (cargando…)"))
	} else if m.debuggerErr != nil {
		lines = append(lines, styleErr.Render("  sin conexión: "+stripControl(m.debuggerErr.Error())))
	} else {
		d := m.debugger
		detect := styleToggleOf.Render("○ DESACTIVADA")
		if d.Detect {
			detect = styleToggleOn.Render("● ACTIVA")
		}
		breaks := styleToggleOf.Render("○ DESACTIVADOS")
		if d.Breakpoints {
			breaks = styleToggleOn.Render("● ACTIVOS")
		}
		lines = append(lines,
			fmt.Sprintf("  detección   %s", detect),
			fmt.Sprintf("  breakpoints %s", breaks),
			fmt.Sprintf("  puerto      %d", d.Port),
			fmt.Sprintf("  modo        %s", orDash(stripControl(d.Mode))),
			"",
			styleTitle.Render("Versiones de PHP"),
			"",
		)
		if len(d.Versions) == 0 {
			lines = append(lines, styleSubtle.Render("  (ninguna)"))
		}
		for _, v := range d.Versions {
			mark := styleSubtle.Render("·")
			if v.Available {
				mark = styleOK.Render("✓")
			}
			lines = append(lines, fmt.Sprintf("  %s %s", mark, stripControl(v.Version)))
		}
		lines = append(lines, "", styleTitle.Render("Dominios"), "")
		if len(d.Domains) == 0 {
			lines = append(lines, styleSubtle.Render("  (ninguno)"))
		} else {
			lines = append(lines, "  "+stripControl(strings.Join(d.Domains, ", ")))
		}
	}
	lines = append(lines, "", styleSubtle.Render("B activar/desactivar detección y breakpoints  ·  r refrescar"))
	return styleCard.Width(m.viewport.Width() - 2).Render(strings.Join(lines, "\n"))
}

// renderMailMessage formats one captured message for the detail viewport.
func renderMailMessage(mm *mailMessage) string {
	lines := []string{
		"De:      " + orDash(stripControl(mm.From)),
		"Para:    " + orDash(stripControl(mm.To)),
		"Asunto:  " + orDash(stripControl(mm.Subject)),
		"Fecha:   " + orDash(stripControl(mm.Date)),
		"ID:      " + orDash(stripControl(mm.ID)),
		"",
		stripControl(mm.Body),
	}
	if len(mm.Attachments) > 0 {
		files := make([]string, 0, len(mm.Attachments))
		for _, file := range mm.Attachments {
			name := stripControl(file.Name)
			if name == "" {
				name = "adjunto"
			}
			files = append(files, name)
		}
		lines = append(lines, "", "Adjuntos: "+strings.Join(files, ", "))
	}
	return strings.Join(lines, "\n")
}

func (m *tuiModel) renderOverview() string {
	s := m.status
	running, total := m.serviceCounts()
	versionLines := []string{
		styleTitle.Render("Versiones"),
		"",
		fmt.Sprintf("  PHP    %s  (default %s)", joinMinors(s.PHP), orDash(stripControl(s.Defaults["php"]))),
		fmt.Sprintf("  Node   %s  (default %s)", strings.Join(s.Node, ", "), orDash(stripControl(s.Defaults["node"]))),
		fmt.Sprintf("  BD     %s", orDash(stripControl(s.Defaults["database"]))),
	}
	envLines := []string{
		styleTitle.Render("Entorno"),
		"",
		fmt.Sprintf("  Home      %s", stripControl(s.Home)),
		fmt.Sprintf("  Sitios    %d", len(s.Sites)),
		fmt.Sprintf("  Servicios %d/%d activos", running, total),
		fmt.Sprintf("  Trabajos  %d", len(s.Jobs)),
	}
	vpWidth := m.viewport.Width()
	if vpWidth < 56 {
		// On a narrow terminal two side-by-side cards become unreadable:
		// stack them and let the viewport scroll instead.
		width := maxInt(vpWidth-2, 24)
		return lipgloss.JoinVertical(lipgloss.Left,
			styleCard.Width(width).Render(strings.Join(versionLines, "\n")),
			styleCard.Width(width).Render(strings.Join(envLines, "\n")),
		)
	}
	half := maxInt((vpWidth-2)/2, 24)
	return lipgloss.JoinHorizontal(lipgloss.Top,
		styleCard.Width(half).Render(strings.Join(versionLines, "\n")), " ",
		styleCard.Width(half).Render(strings.Join(envLines, "\n")),
	)
}

func (m *tuiModel) renderRuntimes() string {
	s := m.status
	nodeList := "(ninguna)"
	if len(s.Node) > 0 {
		nodeList = strings.Join(s.Node, ", ")
	}
	return styleCard.Width(m.viewport.Width() - 2).Render(strings.Join([]string{
		styleTitle.Render("Runtimes instalados"),
		"",
		"  PHP   " + joinMinors(s.PHP) + "   (default " + orDash(s.Defaults["php"]) + ")",
		"  Node  " + nodeList + "   (default " + orDash(s.Defaults["node"]) + ")",
		"",
		styleSubtle.Render("p elige versión de PHP  ·  n elige versión de Node  ·  d cambia la predeterminada"),
		styleSubtle.Render("F gestiona las carpetas de proyectos (lista, alta/baja, importar Valet, migrar ~/lair)"),
	}, "\n"))
}

func (m *tuiModel) renderDatabases() string {
	s := m.status
	lines := []string{styleTitle.Render("Bases de datos"), ""}
	for _, id := range []string{"mariadb", "mysql", "postgres"} {
		v, ok := s.Services[id]
		if !ok {
			continue
		}
		mark := "  "
		if s.Defaults["database"] == id {
			mark = styleOK.Render("* ")
		}
		state := styleSubtle.Render("detenida")
		if v.Running {
			state = styleOK.Render("activa")
		}
		lines = append(lines, fmt.Sprintf("%s%-10s puerto %-6d %s", mark, id, v.Port, state))
	}
	lines = append(lines, "", styleSubtle.Render("m elige motor y versión  ·  I instala cualquier servicio  ·  d cambia la predeterminada"))
	return styleCard.Width(m.viewport.Width() - 2).Render(strings.Join(lines, "\n"))
}

func (m *tuiModel) renderServices() string {
	ids := m.serviceIDs()
	if len(ids) == 0 {
		return styleSubtle.Render("No hay servicios registrados.")
	}
	cards := make([]string, 0, len(ids))
	width := maxInt(m.viewport.Width()-2, 24)
	for i, id := range ids {
		v := m.status.Services[id]
		st := styleCard
		if i == m.selected {
			st = styleCardOn
		}
		lines := []string{
			fmt.Sprintf("%s   %s", styleTitle.Render(id), m.renderToggle(v.Running)),
			styleSubtle.Render(fmt.Sprintf("versión %s · puerto %d · PID %d", orDash(v.Active), v.Port, v.PID)),
		}
		if v.LastError != "" {
			lines = append(lines, styleWarn.Render("último error: "+stripControl(v.LastError)))
		}
		cards = append(cards, st.Width(width).Render(strings.Join(lines, "\n")))
	}
	return lipgloss.JoinVertical(lipgloss.Left, cards...)
}

func (m *tuiModel) renderToggle(active bool) string {
	if active {
		return styleToggleOn.Render("● ACTIVO")
	}
	return styleToggleOf.Render("○ DETENIDO")
}

func (m *tuiModel) renderSites() string {
	s := m.status
	lines := []string{styleTitle.Render("Sitios aparcados"), ""}
	if len(s.Sites) == 0 {
		lines = append(lines, styleSubtle.Render("  (ninguno)"))
	}
	for i, site := range s.Sites {
		port := s.Web.HTTPPort
		if site.Secure && s.Web.HTTPS {
			port = s.Web.HTTPSPort
		}
		scheme := "http"
		if site.Secure && s.Web.HTTPS {
			scheme = "https"
		}
		mark := "  "
		if i == m.siteSelected {
			mark = styleOK.Render("▸ ")
		}
		lines = append(lines, mark+fmt.Sprintf("%s://%s:%d -> %s", scheme, stripControl(site.Name), port, stripControl(site.Root)))
	}
	lines = append(lines, "", styleSubtle.Render(
		"↑/↓ seleccionar  ·  E detalle  ·  V vhost  ·  t enabled (en el detalle)  ·  X borrar  ·  C nuevo proyecto  ·  a aparcar  ·  e enlazar  ·  o desvincular  ·  c certificado  ·  g fijar  ·  i aislar"))
	return styleCard.Width(m.viewport.Width() - 2).Render(strings.Join(lines, "\n"))
}

func (m *tuiModel) renderSystem() string {
	lines := []string{
		styleTitle.Render("Sistema"),
		"",
		"  " + styleSubtle.Render("Catálogo firmado con Ed25519; actualízalo con 'u'."),
		"  " + styleSubtle.Render("v ver logs  ·  x cancelar trabajo  ·  r refrescar"),
		"  " + styleSubtle.Render("M correo  ·  D dumps  ·  B debugger (en su sección)"),
		"  " + styleSubtle.Render("F gestiona las carpetas de proyectos (lista, alta/baja, importar Valet, migrar ~/lair)"),
		"  " + styleSubtle.Render("La app desktop ofrece las mismas acciones: lair desktop"),
	}
	if len(m.projectDirs) > 0 {
		lines = append(lines, "", styleTitle.Render("Carpetas de proyectos"), "")
		for _, dir := range m.projectDirs {
			lines = append(lines, "  "+stripControl(dir))
		}
	}
	if jobs := m.status.Jobs; len(jobs) > 0 {
		lines = append(lines, "", styleTitle.Render("Trabajos"), "")
		for _, job := range jobs {
			lines = append(lines, fmt.Sprintf("  %-16s %-14s %s", stripControl(job.ID), stripControl(job.Phase), orDash(stripControl(job.Error))))
		}
	} else {
		lines = append(lines, "", styleSubtle.Render("  (sin trabajos en curso)"))
	}
	return styleCard.Width(m.viewport.Width() - 2).Render(strings.Join(lines, "\n"))
}

func (m *tuiModel) renderFooter() string {
	running, total := m.serviceCounts()
	status := fmt.Sprintf("Servicios %d/%d %s  %s", running, total, m.renderBar(10), m.message)
	if m.busy != "" {
		status = m.spinner.View() + " " + m.busy
	}
	var b strings.Builder
	b.WriteString(styleSubtle.Render(strings.Repeat("─", m.width)) + "\n")
	if p := m.jobProgress; p != nil && p.Total > 0 {
		pct := int(100 * p.Downloaded / p.Total)
		if pct < 0 {
			pct = 0
		}
		if pct > 100 {
			pct = 100
		}
		width := 24
		filled := pct * width / 100
		status = fmt.Sprintf("%s %s %3d%%", styleBarOn.Render(strings.Repeat("█", filled)),
			styleBarOff.Render(strings.Repeat("░", width-filled)), pct)
	}
	b.WriteString(status + "\n")
	b.WriteString(m.help.View(m.keys) + "\n")
	return b.String()
}

func (m *tuiModel) renderBar(width int) string {
	filled := int(math.Round(m.barPos * float64(width)))
	if filled < 0 {
		filled = 0
	}
	if filled > width {
		filled = width
	}
	return styleBarOn.Render(strings.Repeat("█", filled)) + styleBarOff.Render(strings.Repeat("░", width-filled))
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func orDash(v string) string {
	if v == "" {
		return "—"
	}
	return v
}
