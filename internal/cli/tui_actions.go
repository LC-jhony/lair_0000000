package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/huh/v2"
	"github.com/lair-sh/lair/internal/daemon"
	"github.com/lair-sh/lair/internal/engine"
	"github.com/lair-sh/lair/internal/project"
	"github.com/lair-sh/lair/internal/services"
	"github.com/lair-sh/lair/internal/shims"
)

// This file holds the interactive dialogs shared with the desktop app. Keeping
// them apart from the render/update loop makes it easy to check that both
// interfaces expose exactly the same daemon actions.

// logNamesMsg carries the result of `logs-list` back into the event loop.
// Sources are optional: older daemons return only the plain file names.
type logNamesMsg struct {
	items   []string
	sources []logSource
	err     error
}

// logTextMsg carries the contents of one log into the viewport.
type logTextMsg struct {
	name string
	text string
}

// ---------- Tooling panes: wire types ----------
//
// The daemon exposes Mail, Dumps and Debugger through one action each, selected
// by an `op` field. These payloads mirror the fixed wire contract; keeping them
// in the CLI lets the TUI decode responses without depending on the engine's
// internal types, while the action names come from the daemon itself.
const (
	paneActionMail     = daemon.ActionMail
	paneActionDumps    = daemon.ActionDumps
	paneActionDebugger = daemon.ActionDebugger
)

type mailRequest struct {
	Op string `json:"op"`
	ID string `json:"id,omitempty"`
}

type mailAttachment struct {
	Name        string `json:"name"`
	ContentType string `json:"content_type"`
	Size        int    `json:"size"`
}

type mailMessage struct {
	ID          string           `json:"id"`
	From        string           `json:"from"`
	To          string           `json:"to"`
	Subject     string           `json:"subject"`
	Date        string           `json:"date"`
	Body        string           `json:"body"`
	Attachments []mailAttachment `json:"attachments,omitempty"`
}

type mailResult struct {
	Running  bool          `json:"running"`
	Port     int           `json:"port"`
	Count    int           `json:"count"`
	Env      string        `json:"env"`
	Messages []mailMessage `json:"messages,omitempty"`
	Message  *mailMessage  `json:"message,omitempty"`
	Output   string        `json:"output,omitempty"`
}

type dumpsRequest struct {
	Op         string   `json:"op"`
	Types      []string `json:"types,omitempty"`
	ShowInPage *bool    `json:"show_in_page,omitempty"`
	Site       string   `json:"site,omitempty"`
}

type dumpsOptions struct {
	Queries    bool `json:"queries"`
	Logs       bool `json:"logs"`
	Views      bool `json:"views"`
	Jobs       bool `json:"jobs"`
	Requests   bool `json:"requests"`
	ShowInPage bool `json:"show_in_page"`
}

type dumpEvent struct {
	ID      string `json:"id"`
	Site    string `json:"site"`
	Type    string `json:"type"`
	Time    string `json:"time"`
	Request string `json:"request"`
	Message string `json:"message"`
}

type dumpsResult struct {
	Capturing bool         `json:"capturing"`
	Options   dumpsOptions `json:"options"`
	Count     int          `json:"count"`
	Events    []dumpEvent  `json:"events,omitempty"`
	Output    string       `json:"output,omitempty"`
}

type debuggerRequest struct {
	Op string `json:"op"`
}

type debuggerVersion struct {
	Version   string `json:"version"`
	Available bool   `json:"available"`
}

type debuggerResult struct {
	Detect      bool              `json:"detect"`
	Breakpoints bool              `json:"breakpoints"`
	Port        int               `json:"port"`
	Mode        string            `json:"mode"`
	Versions    []debuggerVersion `json:"versions,omitempty"`
	Domains     []string          `json:"domains,omitempty"`
}

// logsListResult extends the classic name list with the optional sources
// reported by newer daemons. Old daemons simply omit `sources`, so `v` keeps
// working unchanged.
type logSource struct {
	Kind  string   `json:"kind"`
	ID    string   `json:"id"`
	Name  string   `json:"name"`
	Path  string   `json:"path"`
	Files []string `json:"files,omitempty"`
}

type logsListResult struct {
	Items   []string    `json:"items"`
	Sources []logSource `json:"sources,omitempty"`
}

// Typed results from the asynchronous tooling commands.
type mailMsg struct {
	op    string
	state mailResult
	err   error
}

type dumpsMsg struct {
	op    string
	state dumpsResult
	err   error
}

type debuggerMsg struct {
	op    string
	state debuggerResult
	err   error
}

// ---------- Site detail ----------
//
// The daemon exposes one `site` action selected by an `op` field. The TUI
// decodes the fixed wire contract into these local types instead of depending
// on engine internals. `detail` carries the overview, `env` the (possibly
// secret) environment values and `domains` the virtual-host names.
const paneActionSite = daemon.ActionSite

// Aliases keep the TUI on the daemon's fixed wire contract instead of carrying
// a second copy of it. The CLI never edits the engine/daemon definitions.
type (
	siteRequest   = daemon.SiteRequest
	unlinkRequest = daemon.UnlinkRequest
	siteResult    = daemon.SiteResult
	siteDetail    = engine.SiteDetail
	siteEnvVar    = engine.SiteEnvEntry
)

// The fixed `site` contract adds two fields to the original detail/env/domains
// implementation: `detail.enabled` and a top-level `config` reply from the
// `config` op. These local wire views decode both using only JSON tags, so the
// TUI keeps compiling and running against daemons that predate them (unknown
// fields are simply ignored by encoding/json and stay zero-valued). They also
// keep the dashboard independent of engine internals.
type siteDetailWire struct {
	siteDetail
	Enabled bool `json:"enabled"`
}

type siteWireResult struct {
	Detail  *siteDetailWire `json:"detail,omitempty"`
	Env     []siteEnvVar    `json:"env,omitempty"`
	Domains []string        `json:"domains,omitempty"`
	Output  string          `json:"output,omitempty"`
	Config  string          `json:"config,omitempty"`
}

// siteEnabledRequest is the payload of the `site set-enabled` op. It stays
// local for the same compatibility reason: an older daemon ignores `enabled`
// and rejects the op, which the TUI surfaces as a normal action error.
type siteEnabledRequest struct {
	Op      string `json:"op"`
	Name    string `json:"name"`
	Enabled bool   `json:"enabled"`
}

// siteDetailMsg is the typed reply of one `site` op. The model merges the three
// ops into one read-only detail view. `enabled` is the detail.enabled flag that
// drives the contextual toggle; `config` carries the generated vhost.
type siteDetailMsg struct {
	op      string
	name    string
	state   siteResult
	enabled bool
	config  string
	err     error
}

// siteConfigMsg is the typed reply of a `site config` call. A disabled site has
// no vhost in nginx, so config may legitimately be empty.
type siteConfigMsg struct {
	name   string
	config string
	err    error
}

// siteEnabledMsg is the typed reply of `site set-enabled`.
type siteEnabledMsg struct {
	name    string
	enabled bool
	err     error
}

// ensureShimsSilently refreshes the shims after a runtime install started from
// the TUI. Failing here must not fail an otherwise successful installation.
func (m *tuiModel) ensureShimsSilently() {
	if err := m.c.call(daemon.ActionEnsureShims, daemon.EnsureShimsRequest{}, &struct{}{}); err == nil {
		return
	}
	// An older daemon may not know the action yet; regenerate locally so the
	// install is still usable from the current binary.
	exe, errExe := os.Executable()
	home, _ := os.UserHomeDir()
	if errExe != nil {
		return
	}
	_ = shims.Generate(m.c.paths.Bin, exe)
	_ = shims.EnsureProfiles(home, m.c.paths.Bin)
}

func (m *tuiModel) openForm(dialog string, fields ...huh.Field) (tea.Model, tea.Cmd) {
	m.dialog = dialog
	m.form = huh.NewForm(huh.NewGroup(fields...)).
		WithShowHelp(false).WithWidth(52).WithTheme(huh.ThemeFunc(huh.ThemeCharm))
	return m, m.form.Init()
}

// openFormCmd is the command-only flavour of openForm, used when a dialog has
// to be chained from applyDialog (which returns only a tea.Cmd).
func (m *tuiModel) openFormCmd(dialog string, fields ...huh.Field) tea.Cmd {
	_, cmd := m.openForm(dialog, fields...)
	return cmd
}

// runPane starts a short tooling action, showing the spinner and honouring the
// same reentrancy guard as the installer actions.
func (m *tuiModel) runPane(busy string, cmd tea.Cmd) tea.Cmd {
	if m.busy != "" {
		_, notice := m.busyNotice()
		return notice
	}
	m.busy = busy
	m.logger.Info("pane action requested", "busy", busy)
	return tea.Batch(m.spinner.Tick, cmd)
}

// fetch helpers: each returns a tea.Cmd that performs one daemon call and
// reports a typed message, so Update never blocks on the socket.
func (m *tuiModel) mailFetch(op, id string) tea.Cmd {
	c := m.c
	return func() tea.Msg {
		var result mailResult
		err := c.call(paneActionMail, mailRequest{Op: op, ID: id}, &result)
		return mailMsg{op: op, state: result, err: cleanErr(err)}
	}
}

func (m *tuiModel) dumpsFetch(op string, show *bool, site string) tea.Cmd {
	c := m.c
	return func() tea.Msg {
		var result dumpsResult
		err := c.call(paneActionDumps, dumpsRequest{Op: op, ShowInPage: show, Site: site}, &result)
		return dumpsMsg{op: op, state: result, err: cleanErr(err)}
	}
}

func (m *tuiModel) debuggerFetch(op string) tea.Cmd {
	c := m.c
	return func() tea.Msg {
		var result debuggerResult
		err := c.call(paneActionDebugger, debuggerRequest{Op: op}, &result)
		return debuggerMsg{op: op, state: result, err: cleanErr(err)}
	}
}

// siteFetch loads one op of the site detail in the background. As with the
// other panes, Update never blocks on the daemon socket.
func (m *tuiModel) siteFetch(op, name string) tea.Cmd {
	c := m.c
	return func() tea.Msg {
		var wire siteWireResult
		err := c.call(paneActionSite, siteRequest{Op: op, Name: name}, &wire)
		var detail *siteDetail
		enabled := false
		if wire.Detail != nil {
			d := wire.Detail.siteDetail
			detail = &d
			enabled = wire.Detail.Enabled
		}
		return siteDetailMsg{
			op:   op,
			name: name,
			state: siteResult{
				Detail: detail, Env: wire.Env,
				Domains: wire.Domains, Output: wire.Output,
			},
			enabled: enabled,
			config:  wire.Config,
			err:     cleanErr(err),
		}
	}
}

// siteVhostFetch loads the generated vhost of one site through `site config`.
// The work happens in the command, never in Update.
func (m *tuiModel) siteVhostFetch(name string) tea.Cmd {
	c := m.c
	return func() tea.Msg {
		var wire siteWireResult
		err := c.call(paneActionSite, siteRequest{Op: "config", Name: name}, &wire)
		return siteConfigMsg{name: name, config: wire.Config, err: cleanErr(err)}
	}
}

// siteSetEnabled flips the `enabled` flag of a site through `site set-enabled`.
func (m *tuiModel) siteSetEnabled(name string, enabled bool) tea.Cmd {
	c := m.c
	return func() tea.Msg {
		var wire siteWireResult
		err := c.call(paneActionSite, siteEnabledRequest{
			Op: "set-enabled", Name: name, Enabled: enabled,
		}, &wire)
		return siteEnabledMsg{name: name, enabled: enabled, err: cleanErr(err)}
	}
}

// loadSiteDetail fetches the overview, environment and domains at once so a
// single `E` press fills the whole detail view.
func (m *tuiModel) loadSiteDetail(name string) tea.Cmd {
	return tea.Batch(m.siteFetch("detail", name), m.siteFetch("env", name), m.siteFetch("domains", name))
}

func (m *tuiModel) loadMail() tea.Cmd {
	return tea.Batch(m.mailFetch("status", ""), m.mailFetch("list", ""))
}

func (m *tuiModel) loadDumps() tea.Cmd {
	return tea.Batch(m.dumpsFetch("status", nil, ""), m.dumpsFetch("list", nil, ""))
}

func (m *tuiModel) loadDebugger() tea.Cmd {
	return m.debuggerFetch("status")
}

func (m *tuiModel) selectField(title string, options []string, value *string) huh.Field {
	opts := make([]huh.Option[string], 0, len(options))
	for _, name := range options {
		opts = append(opts, huh.NewOption(name, name))
	}
	return huh.NewSelect[string]().Title(title).Options(opts...).Value(value)
}

func (m *tuiModel) inputField(title, placeholder string, value *string) huh.Field {
	return huh.NewInput().Title(title).Placeholder(placeholder).Value(value)
}

func (m *tuiModel) fail(label, message string) tea.Cmd {
	return func() tea.Msg { return actionMsg{label: label, err: fmt.Errorf("%s", message)} }
}

func (m *tuiModel) run(busy, label, action string, payload any) tea.Cmd {
	_, cmd := m.start(busy, label, action, payload)
	return cmd
}

func (m *tuiModel) askDefault() (tea.Model, tea.Cmd) {
	if m.pane == paneDatabases {
		ids := make([]string, 0)
		for _, id := range []string{"mariadb", "mysql", "postgres"} {
			if len(m.status.Services[id].Installed) > 0 {
				ids = append(ids, id)
			}
		}
		if len(ids) == 0 {
			m.message = styleSubtle.Render("_no hay motores instalados_")
			return m, nil
		}
		m.formText = ""
		return m.openForm("defaultDB", m.selectField("Base de datos predeterminada", ids, &m.formText))
	}
	if m.pane != paneRuntimes {
		m.message = styleSubtle.Render("usa Runtimes o Bases de datos para cambiar la predeterminada")
		return m, nil
	}
	options := make([]string, 0, len(m.status.PHP)+len(m.status.Node))
	for _, p := range m.status.PHP {
		options = append(options, "php:"+p.Minor)
	}
	for _, v := range m.status.Node {
		options = append(options, "node:"+v)
	}
	if len(options) == 0 {
		m.message = styleSubtle.Render("no hay versiones instaladas")
		return m, nil
	}
	m.formText = options[0]
	return m.openForm("default", m.selectField("Versión predeterminada", options, &m.formText))
}

// installOptions renders catalog versions as "id:version" choices shared by
// the database and generic service install dialogs.
func (m *tuiModel) installOptions(ids []string) []string {
	out := []string{}
	for _, id := range ids {
		for _, version := range m.catalogVersions(id) {
			out = append(out, id+":"+version)
		}
	}
	return out
}

func (m *tuiModel) servicePort(id string) int {
	if svc, ok := m.status.Services[id]; ok && svc.Port > 0 {
		return svc.Port
	}
	return services.DefaultPort(id)
}

func (m *tuiModel) askInstallPHP() (tea.Model, tea.Cmd) {
	versions := m.catalogVersions("php")
	if len(versions) == 0 {
		m.message = styleSubtle.Render("no hay versiones de PHP en el catálogo")
		return m, nil
	}
	m.formText = versions[0]
	return m.openForm("installPHP", m.selectField("Versión de PHP", versions, &m.formText))
}

func (m *tuiModel) askInstallNode() (tea.Model, tea.Cmd) {
	versions := m.catalogVersions("node")
	m.formText = ""
	m.formText2 = ""
	fields := make([]huh.Field, 0, 2)
	if len(versions) > 0 {
		m.formText = versions[0]
		fields = append(fields, m.selectField("Versión de Node (catálogo)", versions, &m.formText))
	}
	fields = append(fields, m.inputField("Otra versión para nvm (vacío = catálogo): 26, lts, 22.16.0", "26", &m.formText2))
	return m.openForm("installNode", fields...)
}

func (m *tuiModel) askInstallDatabase() (tea.Model, tea.Cmd) {
	options := m.installOptions([]string{"mariadb", "mysql", "postgres"})
	if len(options) == 0 {
		m.message = styleSubtle.Render("no hay motores de base de datos en el catálogo")
		return m, nil
	}
	m.formText = options[0]
	m.formText2 = strconv.Itoa(m.servicePort(strings.SplitN(options[0], ":", 2)[0]))
	return m.openForm("installService",
		m.selectField("Motor y versión", options, &m.formText),
		m.inputField("Puerto (0 = predeterminado)", "0", &m.formText2),
	)
}

func (m *tuiModel) askInstallService() (tea.Model, tea.Cmd) {
	options := m.installOptions(services.IDs())
	if len(options) == 0 {
		m.message = styleSubtle.Render("no hay servicios en el catálogo")
		return m, nil
	}
	m.formText = options[0]
	m.formText2 = strconv.Itoa(m.servicePort(strings.SplitN(options[0], ":", 2)[0]))
	return m.openForm("installService",
		m.selectField("Servicio y versión", options, &m.formText),
		m.inputField("Puerto (0 = predeterminado)", "0", &m.formText2),
	)
}

func (m *tuiModel) askLogs() (tea.Model, tea.Cmd) {
	if m.pane == paneServices {
		ids := m.serviceIDs()
		if len(ids) > 0 && m.selected < len(ids) {
			return m.showLog(ids[m.selected])
		}
	}
	// From a site detail, `v` scopes the picker to that site and closes the
	// detail so the log viewer (the existing logTextMsg path) takes over.
	if m.detailText != "" && m.siteDetailName != "" {
		m.siteLogFilter = m.siteDetailName
		m.detailText, m.detailTitle = "", ""
		m.siteDetailName = ""
	}
	return m, func() tea.Msg {
		var result logsListResult
		err := m.c.call(daemon.ActionLogsList, nil, &result)
		return logNamesMsg{items: result.Items, sources: result.Sources, err: cleanErr(err)}
	}
}

func (m *tuiModel) fetchLog(name string) tea.Cmd {
	return func() tea.Msg {
		var result daemon.LogsResult
		err := m.c.call(daemon.ActionLogs, daemon.LogsRequest{Name: name}, &result)
		if err != nil {
			return actionMsg{label: "logs", err: cleanErr(err)}
		}
		return logTextMsg{name: name, text: result.Text}
	}
}

func (m *tuiModel) showLog(name string) (tea.Model, tea.Cmd) {
	return m, m.fetchLog(name)
}

func (m *tuiModel) askCancelJob() (tea.Model, tea.Cmd) {
	ids := make([]string, 0, len(m.status.Jobs))
	for _, job := range m.status.Jobs {
		ids = append(ids, job.ID)
	}
	if len(ids) == 0 {
		m.message = styleSubtle.Render("no hay trabajos en curso")
		return m, nil
	}
	m.formText = ids[0]
	return m.openForm("cancelJob", m.selectField("Trabajo a cancelar", ids, &m.formText))
}

func (m *tuiModel) siteNames() []string {
	names := make([]string, 0, len(m.status.Sites))
	for _, site := range m.status.Sites {
		names = append(names, site.Name)
	}
	return names
}

// selectedSiteName is the site highlighted in the Sitios pane. It falls back
// to the first one so existing dialogs keep their behaviour on a fresh start.
func (m *tuiModel) selectedSiteName() string {
	names := m.siteNames()
	if len(names) == 0 {
		return ""
	}
	if m.siteSelected < 0 || m.siteSelected >= len(names) {
		m.siteSelected = 0
	}
	return names[m.siteSelected]
}

func (m *tuiModel) moveSiteSelection(delta int) {
	n := len(m.status.Sites)
	if n == 0 {
		return
	}
	m.siteSelected = (m.siteSelected + delta + n) % n
}

// openSiteDetail starts the asynchronous load of the selected site's detail.
func (m *tuiModel) openSiteDetail() (tea.Model, tea.Cmd) {
	name := m.selectedSiteName()
	if name == "" {
		m.message = styleSubtle.Render("no hay sitios registrados")
		return m, nil
	}
	m.siteDetailName = name
	m.siteDetailData = siteResult{}
	m.siteDetailErr = nil
	m.siteDetailPending = 3
	m.siteConfigView = false
	m.logText, m.logTitle = "", ""
	m.detailText, m.detailTitle = "", ""
	m.viewport.GotoTop()
	m.message = styleSubtle.Render("cargando detalle de " + stripControl(name) + "…")
	return m, m.loadSiteDetail(name)
}

// applySiteDetail merges one site op into the detail state. Stale replies from
// a site that is no longer open are ignored.
func (m *tuiModel) applySiteDetail(msg siteDetailMsg) (tea.Model, tea.Cmd) {
	if m.siteDetailName == "" || (msg.name != "" && msg.name != m.siteDetailName) {
		return m, nil
	}
	// An in-flight detail reply must never replace an open vhost viewer.
	if m.siteConfigView {
		return m, nil
	}
	if m.siteDetailPending > 0 {
		m.siteDetailPending--
	}
	if msg.err != nil {
		m.siteDetailErr = msg.err
		m.logger.Warn("site detail failed", "op", msg.op, "err", msg.err)
	}
	switch msg.op {
	case "detail":
		if msg.state.Detail != nil {
			m.siteDetailData.Detail = msg.state.Detail
			m.siteEnabled = msg.enabled
		}
	case "env":
		m.siteDetailData.Env = msg.state.Env
	case "domains":
		if len(msg.state.Domains) > 0 {
			m.siteDetailData.Domains = msg.state.Domains
		} else if m.siteDetailData.Detail != nil && len(m.siteDetailData.Detail.Domains) > 0 {
			m.siteDetailData.Domains = m.siteDetailData.Detail.Domains
		}
	}
	if m.siteDetailPending > 0 {
		return m, nil
	}
	m.detailTitle = "Sitio · " + m.siteDetailName
	m.detailText = renderSiteDetail(m.siteDetailData, m.siteEnabled, m.siteDetailErr)
	m.viewport.GotoTop()
	m.message = styleSubtle.Render("esc / retroceso: volver · v logs · V vhost · t alternar enabled")
	return m, nil
}

// openSiteVhost loads the generated vhost of the highlighted site and shows it
// in the same scrollable viewer used by the detail and the log panes.
func (m *tuiModel) openSiteVhost() (tea.Model, tea.Cmd) {
	name := m.selectedSiteName()
	if name == "" {
		m.message = styleSubtle.Render("no hay sitios registrados")
		return m, nil
	}
	m.siteDetailName = name
	m.siteConfigView = true
	m.siteDetailPending = 0
	m.siteLogFilter = ""
	m.logText, m.logTitle = "", ""
	m.detailText, m.detailTitle = "", ""
	m.viewport.GotoTop()
	m.message = styleSubtle.Render("cargando vhost de " + stripControl(name) + "…")
	return m, m.siteVhostFetch(name)
}

// applySiteConfig renders a `site config` reply. An empty config is expected for
// a disabled site: nginx has no vhost for it, so say so instead of showing a
// blank card.
func (m *tuiModel) applySiteConfig(msg siteConfigMsg) (tea.Model, tea.Cmd) {
	if m.siteDetailName == "" || (msg.name != "" && msg.name != m.siteDetailName) {
		return m, nil
	}
	m.siteConfigView = true
	m.detailTitle = "Vhost · " + m.siteDetailName
	switch {
	case msg.err != nil:
		m.detailText = styleErr.Render("  sin conexión: " + stripControl(msg.err.Error()))
		m.message = styleErr.Render("vhost: " + stripControl(msg.err.Error()))
	default:
		config := strings.TrimSpace(stripControl(msg.config))
		if config == "" {
			m.detailText = styleWarn.Render("(sin vhost generado: el sitio está desactivado)")
			m.message = styleSubtle.Render("el sitio no tiene vhost (desactivado)")
		} else {
			m.detailText = config
			m.message = styleSubtle.Render("esc / retroceso: volver · t alternar enabled")
		}
	}
	m.viewport.GotoTop()
	return m, nil
}

// askSiteToggle confirms the enabled flip from the site detail view. It reuses
// the generic confirm dialog so cancellation behaves like every other action.
func (m *tuiModel) askSiteToggle() (tea.Model, tea.Cmd) {
	name := m.siteDetailName
	if name == "" {
		m.message = styleSubtle.Render("no hay sitio abierto")
		return m, nil
	}
	verb := "Activar"
	if m.siteEnabled {
		verb = "Desactivar"
	}
	m.confirmValue = false
	m.pendingID = name
	m.pendingStart = !m.siteEnabled
	m.dialog = "siteToggle"
	return m.openForm("siteToggle",
		huh.NewConfirm().
			Title(verb+" "+stripControl(name)+"?").
			Description("El daemon aplicará el cambio en el entorno local.").
			Affirmative("Sí, "+strings.ToLower(verb)).
			Negative("Cancelar").
			Value(&m.confirmValue),
	)
}

// applySiteToggle sends `site set-enabled` and reports the typed reply.
func (m *tuiModel) applySiteToggle(name string, enabled bool) (tea.Model, tea.Cmd) {
	if m.busy != "" {
		return m.busyNotice()
	}
	m.busy = "Actualizando sitio " + stripControl(name) + "…"
	m.logger.Info("site enabled toggle requested", "site", name, "enabled", enabled)
	return m, tea.Batch(m.spinner.Tick, m.siteSetEnabled(name, enabled))
}

// applySiteEnabled consumes the `set-enabled` reply and refreshes the detail so
// the enabled badge (and the presence of a vhost) reflect the persisted state.
func (m *tuiModel) applySiteEnabled(msg siteEnabledMsg) (tea.Model, tea.Cmd) {
	m.busy = ""
	if msg.err != nil {
		m.message = styleErr.Render("enabled: " + stripControl(msg.err.Error()))
		m.logger.Error("site toggle failed", "site", msg.name, "err", msg.err)
		return m, nil
	}
	m.siteEnabled = msg.enabled
	state := "activado"
	if !msg.enabled {
		state = "desactivado"
	}
	m.message = styleOK.Render("sitio " + msg.name + ": " + state)
	m.logger.Info("site toggle ok", "site", msg.name, "enabled", msg.enabled)
	// A disabled site has no vhost; reloading the detail also re-renders the
	// viewer with the authoritative flag returned by the daemon.
	return m, tea.Batch(m.refresh(), m.siteFetch("detail", msg.name))
}

// maskSiteEnvValue never reveals a secret value: it prints a fixed-length
// placeholder instead. Unset variables do not print a value at all.
func maskSiteEnvValue(v siteEnvVar) string {
	if !v.Set {
		return styleSubtle.Render("(sin definir)")
	}
	if v.Secret {
		return "••••••••"
	}
	if v.Value == "" {
		return styleSubtle.Render("(vacío)")
	}
	return stripControl(v.Value)
}

// renderSiteDetail builds the read-only detail body used by renderDetail.
// The enabled flag comes from the fixed `detail.enabled` contract field.
func renderSiteDetail(state siteResult, enabled bool, err error) string {
	lines := []string{styleTitle.Render("Overview"), ""}
	if err != nil {
		lines = append(lines, styleErr.Render("  sin conexión: "+stripControl(err.Error())))
	}
	badge := styleToggleOf.Render("○ DESACTIVADO")
	if enabled {
		badge = styleToggleOn.Render("● ACTIVADO")
	}
	lines = append(lines, fmt.Sprintf("  Enabled   %s   %s", badge, styleSubtle.Render("t: alternar")))
	d := state.Detail
	if d == nil {
		lines = append(lines, styleSubtle.Render("  (detalle no disponible)"))
	} else {
		domains := state.Domains
		if len(domains) == 0 {
			domains = d.Domains
		}
		https := styleSubtle.Render("desactivado")
		if d.Secure {
			https = styleOK.Render("activo")
		}
		lines = append(lines,
			fmt.Sprintf("  URL       %s", orDash(stripControl(d.URL))),
			fmt.Sprintf("  Ruta      %s", orDash(stripControl(d.Path))),
			fmt.Sprintf("  PHP       %s", orDash(stripControl(d.PHP))),
			fmt.Sprintf("  Node      %s", orDash(stripControl(d.Node))),
			fmt.Sprintf("  BD        %s", orDash(stripControl(d.Database))),
			fmt.Sprintf("  HTTPS     %s", https),
			fmt.Sprintf("  root      %s", orDash(stripControl(d.Root))),
		)
		if d.RootState != "" {
			lines = append(lines, fmt.Sprintf("  estado    %s", stripControl(d.RootState)))
		}
		// A broken document root is the most common reason a site does not
		// serve; make it impossible to miss even in a long detail view.
		if d.RootState != "ok" {
			lines = append(lines, "", styleWarn.Render(
				"  aviso: root no válido (root_state="+orDash(stripControl(d.RootState))+")"))
			if len(d.RootSuggestions) > 0 {
				clean := make([]string, 0, len(d.RootSuggestions))
				for _, s := range d.RootSuggestions {
					clean = append(clean, stripControl(s))
				}
				lines = append(lines, styleSubtle.Render("    sugerencias: "+strings.Join(clean, ", ")))
			}
		}
		if len(domains) == 0 {
			lines = append(lines, "  Dominios  "+styleSubtle.Render("(ninguno)"))
		} else {
			clean := make([]string, 0, len(domains))
			for _, domain := range domains {
				clean = append(clean, stripControl(domain))
			}
			lines = append(lines, "  Dominios  "+strings.Join(clean, ", "))
		}
		if d.Problem != "" {
			lines = append(lines, "", styleWarn.Render("  problema: "+stripControl(d.Problem)))
		}
		if d.Notice != "" {
			lines = append(lines, "", styleSubtle.Render("  nota: "+stripControl(d.Notice)))
		}
	}

	lines = append(lines, "", styleTitle.Render("Environment"), "")
	if len(state.Env) == 0 {
		lines = append(lines, styleSubtle.Render("  (sin variables)"))
	}
	for _, v := range state.Env {
		key := stripControl(v.Key)
		if key == "" {
			continue
		}
		lines = append(lines, fmt.Sprintf("  %-16s %s", key, maskSiteEnvValue(v)))
	}

	lines = append(lines, "", styleTitle.Render("Vhost"), "",
		styleSubtle.Render("  V ver el vhost generado (site config)"),
		"", styleTitle.Render("Logs"), "",
		styleSubtle.Render("  v ver registros del sitio (logs-list / logs)"),
		"", styleSubtle.Render("t alternar enabled  ·  esc / retroceso: volver"))
	return strings.Join(lines, "\n")
}

// askDeleteSite confirms both the unlink and whether the project folder must
// be removed. The folder flag is sent to `unlink` as delete_files.
func (m *tuiModel) askDeleteSite() (tea.Model, tea.Cmd) {
	name := m.selectedSiteName()
	if name == "" {
		m.message = styleSubtle.Render("no hay sitios registrados")
		return m, nil
	}
	m.formText = name
	m.confirmValue = false
	m.siteDeleteFiles = false
	return m.openForm("deleteSite",
		m.selectField("Sitio a borrar", m.siteNames(), &m.formText),
		huh.NewConfirm().
			Title("¿Borrar el sitio?").
			Description("Se desvinculará de Lair.").
			Affirmative("Sí, borrar").
			Negative("Cancelar").
			Value(&m.confirmValue),
		huh.NewConfirm().
			Title("¿Borrar también la carpeta del proyecto?").
			Description("Elimina los archivos del proyecto (delete_files).").
			Affirmative("Sí, borrar carpeta").
			Negative("No, conservarla").
			Value(&m.siteDeleteFiles),
	)
}

func (m *tuiModel) askPark() (tea.Model, tea.Cmd) {
	dir, _ := os.Getwd()
	m.formText = dir
	return m.openForm("park", m.inputField("Ruta del proyecto a aparcar", dir, &m.formText))
}

func (m *tuiModel) askLink() (tea.Model, tea.Cmd) {
	m.formText, m.formText2 = "", ""
	return m.openForm("link",
		m.inputField("Nombre del sitio (p. ej. acme.test)", "acme.test", &m.formText),
		m.inputField("Ruta del proyecto", ".", &m.formText2),
	)
}

func (m *tuiModel) askSiteAction(dialog string) (tea.Model, tea.Cmd) {
	names := m.siteNames()
	if len(names) == 0 {
		m.message = styleSubtle.Render("no hay sitios registrados")
		return m, nil
	}
	m.formText = m.selectedSiteName()
	title := "Sitio"
	if dialog == "secure" {
		title = "Emitir certificado para"
	}
	return m.openForm(dialog, m.selectField(title, names, &m.formText))
}

func (m *tuiModel) sitePath(name string) string {
	for _, site := range m.status.Sites {
		if site.Name == name {
			return site.Root
		}
	}
	return ""
}

func (m *tuiModel) versionOptions() []string {
	ids := make([]string, 0, len(m.status.Services))
	for id := range m.status.Services {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	options := make([]string, 0)
	for _, id := range ids {
		for _, version := range m.status.Services[id].Installed {
			options = append(options, id+":"+version)
		}
	}
	return options
}

func (m *tuiModel) askUseVersion() (tea.Model, tea.Cmd) {
	options := m.versionOptions()
	if len(options) == 0 {
		m.message = styleSubtle.Render("no hay versiones de servicio instaladas")
		return m, nil
	}
	m.formText = options[0]
	return m.openForm("useVersion", m.selectField("Versión activa del servicio", options, &m.formText))
}

func (m *tuiModel) askUninstall() (tea.Model, tea.Cmd) {
	options := m.versionOptions()
	if len(options) == 0 {
		m.message = styleSubtle.Render("no hay versiones de servicio instaladas")
		return m, nil
	}
	m.formText, m.confirmValue = options[0], false
	return m.openForm("uninstall",
		m.selectField("Versión a desinstalar", options, &m.formText),
		huh.NewConfirm().
			Title("¿Confirmar desinstalación?").
			Description("Los datos se conservan; el binario se elimina.").
			Affirmative("Sí, desinstalar").
			Negative("Cancelar").
			Value(&m.confirmValue),
	)
}

func (m *tuiModel) askCreateDB() (tea.Model, tea.Cmd) {
	ids := make([]string, 0)
	for _, id := range []string{"mariadb", "mysql", "postgres"} {
		if len(m.status.Services[id].Installed) > 0 {
			ids = append(ids, id)
		}
	}
	if len(ids) == 0 {
		m.message = styleSubtle.Render("no hay motores instalados")
		return m, nil
	}
	m.formText, m.formText2 = ids[0], ""
	return m.openForm("createDB",
		m.selectField("Motor", ids, &m.formText),
		m.inputField("Nombre de la base de datos", "acme_dev", &m.formText2),
	)
}

func (m *tuiModel) askPin() (tea.Model, tea.Cmd) {
	m.formTool, m.formText = "php", ""
	fields := []huh.Field{m.selectField("Herramienta", []string{"php", "node"}, &m.formTool)}
	names := m.siteNames()
	m.formText2 = ""
	if len(names) > 0 {
		m.formText2 = m.selectedSiteName()
		fields = append(fields, m.selectField("Sitio (escribe .lairrc)", names, &m.formText2))
	}
	fields = append(fields, m.inputField("Versión (se escribe .lairrc)", "8.4", &m.formText))
	return m.openForm("pin", fields...)
}

func (m *tuiModel) askIsolate() (tea.Model, tea.Cmd) {
	names := m.siteNames()
	if len(names) == 0 {
		m.message = styleSubtle.Render("no hay sitios registrados")
		return m, nil
	}
	m.formText, m.formTool, m.formText2 = m.selectedSiteName(), "php", ""
	return m.openForm("isolate",
		m.selectField("Sitio", names, &m.formText),
		m.selectField("Herramienta", []string{"php", "node"}, &m.formTool),
		m.inputField("Versión (o --off)", "8.4", &m.formText2),
	)
}

// filterOptions keeps the canonical order of all while retaining only the
// choices the daemon reported as available on this machine.
func filterOptions(all, allowed []string) []string {
	if len(allowed) == 0 {
		return append([]string(nil), all...)
	}
	available := make(map[string]bool, len(allowed))
	for _, value := range allowed {
		available[value] = true
	}
	out := make([]string, 0, len(all))
	for _, value := range all {
		if available[value] {
			out = append(out, value)
		}
	}
	return out
}

// projectForm builds the wizard from the capabilities loaded with
// daemon.ActionProject{Op:"info"}. It is also used to rebuild the form in place
// when that information arrives after the dialog was opened; every field is
// bound to a model value, so already typed answers are preserved.
func (m *tuiModel) projectForm() *huh.Form {
	info := m.projectInfo
	nodeManagers := filterOptions(project.NodeManagers, info.NodeManagers)
	if len(nodeManagers) == 0 {
		nodeManagers = append([]string(nil), project.NodeManagers...)
	}
	databases := filterOptions(project.Databases, info.Databases)
	if len(databases) == 0 {
		databases = []string{"sqlite"}
	}
	if !hasString(nodeManagers, m.projectNode) {
		m.projectNode = nodeManagers[0]
	}
	if !hasString(databases, m.projectDB) {
		m.projectDB = databases[0]
	}

	parent := strings.TrimSpace(info.ProjectsDir)
	if parent == "" {
		parent = defaultProjectParent()
	}
	fields := make([]huh.Field, 0, 14)
	if !info.LaravelInstalled && strings.TrimSpace(info.Hint) != "" {
		hint := stripControl(info.Hint)
		// The note can be scrolled out of the form viewport; mirror it in the
		// status bar so the warning is always visible.
		m.message = styleWarn.Render(hint)
		fields = append(fields, huh.NewNote().
			Title(hint).
			Description("El asistente instalará lo que falte en un entorno aislado de Lair."))
	}
	fields = append(fields,
		m.inputField("Nombre del proyecto (minúsculas y guiones)", "mi-app", &m.formText),
		m.inputField("Carpeta padre ( existe )", parent, &m.formText2),
		m.selectField("Interfaz", project.Stacks, &m.projectStack),
		m.selectField("Pruebas", project.Testings, &m.projectTest),
		m.selectField("Gestor de Node", nodeManagers, &m.projectNode),
		m.selectField("Base de datos", databases, &m.projectDB),
		m.inputField("Nombre de la BD (no aplica a SQLite)", "mi_app_dev", &m.projectDBName),
		m.inputField("Usuario de la BD", "root", &m.projectDBUser),
		huh.NewInput().Title("Contraseña de la BD").EchoMode(huh.EchoModePassword).Value(&m.projectDBPass),
		huh.NewConfirm().Title("¿HTTPS para el sitio?").Affirmative("Sí").Negative("No").Value(&m.projectSecure),
		huh.NewConfirm().Title("¿Crear la base de datos?").Affirmative("Sí").Negative("No").Value(&m.projectCreateDB),
		huh.NewConfirm().Title("¿Arrancar el servicio si está parado?").Affirmative("Sí").Negative("No").Value(&m.projectStart),
	)
	return huh.NewForm(huh.NewGroup(fields...)).
		WithShowHelp(false).WithWidth(52).WithTheme(huh.ThemeFunc(huh.ThemeCharm))
}

func hasString(list []string, want string) bool {
	for _, value := range list {
		if value == want {
			return true
		}
	}
	return false
}

// askCreateProject opens the same wizard offered by the desktop app. The
// folder is a text field so the TUI and the desktop bridge stay in parity.
func (m *tuiModel) askCreateProject() (tea.Model, tea.Cmd) {
	parent := strings.TrimSpace(m.projectInfo.ProjectsDir)
	if parent == "" {
		parent = defaultProjectParent()
	}
	m.formText, m.formText2 = "", parent
	m.projectStack, m.projectTest = "blade", "pest"
	m.projectDBName, m.projectDBUser, m.projectDBPass = "", "root", ""
	m.projectSecure, m.projectCreateDB, m.projectStart = false, true, true
	m.dialog = "project"
	form := m.projectForm()
	m.form = form
	// Capabilities are loaded in Init too; refreshing here keeps the dialog
	// accurate when it is opened long after startup.
	return m, tea.Batch(form.Init(), m.loadProjectInfo())
}

// ---------- Project directories ----------
//
// The single `project` daemon action also owns the list of folders where new
// projects live. The reply is decoded through a local wire view so the TUI
// keeps working against daemons that do not report `dirs` yet.
// Choices of the `F` dialog. Keeping them as constants makes the mapping in
// applyDialog explicit and testable.
const (
	dirsRefresh     = "actualizar lista"
	dirsAdd         = "añadir carpeta"
	dirsRemove      = "quitar carpeta"
	dirsImportValet = "importar carpetas de Laravel Valet"
	dirsMigrate     = "migrar la antigua ~/lair a ~/Lair"
)

// projectDirsResult extends the shared project result with the `dirs` field of
// the `project dirs` reply. JSON decoding ignores it on older daemons.
type projectDirsResult struct {
	project.Result
	Dirs []string `json:"dirs,omitempty"`
}

// defaultProjectParent is the folder offered when `project info` has no
// configured parent. New projects live under ~/Lair, never under LAIR_HOME.
func defaultProjectParent() string {
	home, _ := os.UserHomeDir()
	if home == "" {
		return "Lair"
	}
	return filepath.Join(home, "Lair")
}

func (m *tuiModel) loadProjectDirs() tea.Cmd {
	c := m.c
	return func() tea.Msg {
		var result projectDirsResult
		err := c.call(daemon.ActionProject, daemon.ProjectRequest{Op: "dirs"}, &result)
		return projectDirsMsg{dirs: result.Dirs, err: cleanErr(err)}
	}
}

// projectDirsPane lists the general sections where F is available. It stays
// away from Sitios so none of the site-only keys changes meaning.
func (m *tuiModel) projectDirsPane() bool {
	switch m.pane {
	case paneOverview, paneRuntimes, paneSystem:
		return true
	}
	return false
}

func (m *tuiModel) projectDirsOptions() []string {
	return []string{dirsRefresh, dirsAdd, dirsRemove, dirsImportValet, dirsMigrate}
}

func (m *tuiModel) projectDirsForm() *huh.Form {
	note := "  (sin carpetas cargadas)"
	if len(m.projectDirs) > 0 {
		items := make([]string, 0, len(m.projectDirs))
		for _, dir := range m.projectDirs {
			items = append(items, "• "+stripControl(dir))
		}
		note = strings.Join(items, "\n")
	}
	if m.projectDirsErr != nil {
		note += "\n\nerror: " + stripControl(m.projectDirsErr.Error())
	}
	if !hasString(m.projectDirsOptions(), m.formText) {
		m.formText = dirsRefresh
	}
	return huh.NewForm(huh.NewGroup(
		huh.NewNote().Title("Carpetas de proyectos").Description(note),
		m.selectField("Acción", m.projectDirsOptions(), &m.formText),
	)).WithShowHelp(false).WithWidth(52).WithTheme(huh.ThemeFunc(huh.ThemeCharm))
}

func (m *tuiModel) askProjectDirs() (tea.Model, tea.Cmd) {
	m.formText, m.formText2 = dirsRefresh, ""
	m.dialog = "projectDirs"
	m.form = m.projectDirsForm()
	// Reload on open so a change made outside the dashboard is visible.
	return m, tea.Batch(m.form.Init(), m.loadProjectDirs())
}

// runDirsOp starts a project op that changes the directory list and marks the
// list for a refresh once the daemon replies.
func (m *tuiModel) runDirsOp(busy, label string, req daemon.ProjectRequest) tea.Cmd {
	m.dirsDirty = true
	return m.run(busy, label, daemon.ActionProject, req)
}

// Selection labels for the tooling dialogs. Keeping them as constants makes
// the mapping in applyDialog explicit and testable.
const (
	mailRefresh    = "actualizar estado"
	mailViewMsg    = "ver mensaje"
	mailStartSMTP  = "iniciar SMTP"
	mailStopSMTP   = "detener SMTP"
	mailClearInbox = "limpiar bandeja"

	dumpsRefresh     = "actualizar estado"
	dumpsCaptureOn   = "activar captura"
	dumpsCaptureOff  = "desactivar captura"
	dumpsShowPage    = "alternar show_in_page"
	dumpsClearEvents = "limpiar eventos"

	debuggerRefresh        = "actualizar estado"
	debuggerDetectOn       = "activar detección"
	debuggerDetectOff      = "desactivar detección"
	debuggerBreakpointsOn  = "activar breakpoints"
	debuggerBreakpointsOff = "desactivar breakpoints"
)

func (m *tuiModel) askMail() (tea.Model, tea.Cmd) {
	options := []string{mailRefresh}
	if m.mail.Running {
		options = append(options, mailStopSMTP)
	} else {
		options = append(options, mailStartSMTP)
	}
	if len(m.mail.Messages) > 0 {
		options = append(options, mailViewMsg)
	}
	options = append(options, mailClearInbox)
	m.formText = options[0]
	return m.openForm("mail", m.selectField("Acción de correo", options, &m.formText))
}

func (m *tuiModel) askDumps() (tea.Model, tea.Cmd) {
	options := []string{dumpsRefresh}
	if m.dumps.Capturing {
		options = append(options, dumpsCaptureOff)
	} else {
		options = append(options, dumpsCaptureOn)
	}
	options = append(options, dumpsShowPage, dumpsClearEvents)
	m.formText = options[0]
	return m.openForm("dumps", m.selectField("Acción de dumps", options, &m.formText))
}

func (m *tuiModel) askDebugger() (tea.Model, tea.Cmd) {
	options := []string{debuggerRefresh}
	if m.debugger.Detect {
		options = append(options, debuggerDetectOff)
	} else {
		options = append(options, debuggerDetectOn)
	}
	if m.debugger.Breakpoints {
		options = append(options, debuggerBreakpointsOff)
	} else {
		options = append(options, debuggerBreakpointsOn)
	}
	m.formText = options[0]
	return m.openForm("debugger", m.selectField("Acción del debugger", options, &m.formText))
}

func (m *tuiModel) applyDialog(dialog string) tea.Cmd {
	value := strings.TrimSpace(m.formText)
	switch dialog {
	case "default":
		parts := strings.SplitN(value, ":", 2)
		if len(parts) != 2 || parts[1] == "" {
			return m.fail("predeterminada", "versión no válida")
		}
		if parts[0] == "php" {
			return m.run("Cambiando PHP…", "PHP predeterminado", daemon.ActionPHPUse, daemon.PHPUseRequest{Minor: parts[1]})
		}
		return m.run("Cambiando Node…", "Node predeterminado", daemon.ActionNodeDefault, daemon.NodeDefaultRequest{Version: parts[1]})
	case "defaultDB":
		return m.run("Cambiando base de datos…", "base de datos", daemon.ActionDBDefault, daemon.DBDefaultRequest{ID: value})
	case "installPHP":
		return m.run("Instalando PHP "+value+"…", "PHP", daemon.ActionInstallPHP, daemon.InstallRequest{Version: value})
	case "installNode":
		custom := strings.TrimSpace(m.formText2)
		if custom != "" {
			return m.run("Instalando Node "+custom+"…", "Node", daemon.ActionInstallNode, daemon.InstallRequest{Version: custom})
		}
		if value == "" {
			return m.fail("instalar", "escribe una versión de Node, por ejemplo 22.16.0")
		}
		return m.run("Instalando Node "+value+"…", "Node", daemon.ActionInstallNode, daemon.InstallRequest{Version: value})
	case "installService":
		parts := strings.SplitN(value, ":", 2)
		if len(parts) != 2 || parts[1] == "" {
			return m.fail("instalar", "selección no válida")
		}
		port, _ := strconv.Atoi(strings.TrimSpace(m.formText2))
		return m.run("Instalando "+parts[0]+" "+parts[1]+"…", parts[0], daemon.ActionInstallService,
			daemon.InstallRequest{ID: parts[0], Version: parts[1], Port: port})
	case "logs":
		// Sources from `logs-list` are shown with a richer label; map the
		// selection back to the actual log name understood by `logs`.
		if name := m.logNameKeys[value]; name != "" {
			return m.fetchLog(name)
		}
		return m.fetchLog(value)
	case "mail":
		switch value {
		case mailRefresh:
			return m.loadMail()
		case mailViewMsg:
			if len(m.mail.Messages) == 0 {
				return m.fail("correo", "no hay mensajes en la bandeja")
			}
			opts := make([]string, 0, len(m.mail.Messages))
			m.mailMessageIDs = map[string]string{}
			for _, msg := range m.mail.Messages {
				label := stripControl(msg.Subject)
				if label == "" {
					label = stripControl(msg.ID)
				}
				if _, exists := m.mailMessageIDs[label]; exists {
					label = stripControl(msg.ID)
				}
				m.mailMessageIDs[label] = stripControl(msg.ID)
				opts = append(opts, label)
			}
			m.formText = opts[0]
			return m.openFormCmd("mailMessage", m.selectField("Mensaje", opts, &m.formText))
		case mailStartSMTP:
			return m.runPane("Iniciando SMTP…", m.mailFetch("start", ""))
		case mailStopSMTP:
			return m.runPane("Deteniendo SMTP…", m.mailFetch("stop", ""))
		case mailClearInbox:
			return m.runPane("Limpiando bandeja…", m.mailFetch("clear", ""))
		}
		return m.fail("correo", "acción no reconocida")
	case "mailMessage":
		id := m.mailMessageIDs[value]
		if id == "" {
			id = value
		}
		return m.runPane("Abriendo mensaje…", m.mailFetch("message", id))
	case "dumps":
		switch value {
		case dumpsRefresh:
			return m.loadDumps()
		case dumpsCaptureOn:
			return m.runPane("Activando captura…", m.dumpsFetch("capture-on", nil, ""))
		case dumpsCaptureOff:
			return m.runPane("Desactivando captura…", m.dumpsFetch("capture-off", nil, ""))
		case dumpsShowPage:
			show := !m.dumps.Options.ShowInPage
			return m.runPane("Actualizando opciones…", m.dumpsFetch("options", &show, ""))
		case dumpsClearEvents:
			return m.runPane("Limpiando eventos…", m.dumpsFetch("clear", nil, ""))
		}
		return m.fail("dumps", "acción no reconocida")
	case "debugger":
		switch value {
		case debuggerRefresh:
			return m.loadDebugger()
		case debuggerDetectOn:
			return m.runPane("Activando detección…", m.debuggerFetch("detect-on"))
		case debuggerDetectOff:
			return m.runPane("Desactivando detección…", m.debuggerFetch("detect-off"))
		case debuggerBreakpointsOn:
			return m.runPane("Activando breakpoints…", m.debuggerFetch("breakpoints-on"))
		case debuggerBreakpointsOff:
			return m.runPane("Desactivando breakpoints…", m.debuggerFetch("breakpoints-off"))
		}
		return m.fail("debugger", "acción no reconocida")
	case "cancelJob":
		return m.run("Cancelando trabajo…", "cancelar trabajo", daemon.ActionJobCancel, daemon.JobRequest{ID: value})
	case "park":
		return m.run("Aparcando sitio…", "park", daemon.ActionPark, daemon.PathRequest{Path: value})
	case "link":
		name := strings.TrimSpace(m.formText)
		path := strings.TrimSpace(m.formText2)
		if name == "" || path == "" {
			return m.fail("enlace", "nombre y ruta son obligatorios")
		}
		return m.run("Enlazando sitio…", "enlace", daemon.ActionLink, daemon.LinkRequest{Name: name, Path: path})
	case "unlink":
		return m.run("Desvinculando sitio…", "desvincular", daemon.ActionUnlink, daemon.LogsRequest{Name: value})
	case "deleteSite":
		name := strings.TrimSpace(value)
		if name == "" {
			return m.fail("borrar sitio", "selecciona un sitio")
		}
		m.detailText, m.detailTitle = "", ""
		m.siteDetailName, m.siteLogFilter = "", ""
		m.siteConfigView = false
		return m.run("Borrando sitio…", "borrar sitio", daemon.ActionUnlink,
			unlinkRequest{Name: name, DeleteFiles: m.siteDeleteFiles})
	case "secure":
		return m.run("Emitiendo certificado…", "certificado", daemon.ActionSecure, daemon.LogsRequest{Name: value})
	case "project":
		name := strings.TrimSpace(m.formText)
		parent := strings.TrimSpace(m.formText2)
		if name == "" || parent == "" {
			return m.fail("nuevo proyecto", "el nombre y la carpeta son obligatorios")
		}
		if m.projectDB != "sqlite" {
			if strings.TrimSpace(m.projectDBUser) == "" {
				return m.fail("nuevo proyecto", "el usuario de la base de datos es obligatorio")
			}
			if m.projectDBPass == "" {
				return m.fail("nuevo proyecto", "la contraseña de la base de datos es obligatoria")
			}
		}
		dbName := strings.TrimSpace(m.projectDBName)
		if dbName == "" {
			dbName = strings.ReplaceAll(name, "-", "_") + "_dev"
		}
		return m.run("Creando proyecto…", "nuevo proyecto", daemon.ActionProject, daemon.ProjectRequest{
			Op: "create",
			CreateRequest: project.CreateRequest{
				Name: name, Parent: parent, Secure: m.projectSecure,
				Stack: m.projectStack, Auth: true, Testing: m.projectTest,
				Boost: false, NodeManager: m.projectNode, Git: false,
				Database: m.projectDB, DBName: dbName,
				DBUser: strings.TrimSpace(m.projectDBUser), DBPassword: m.projectDBPass,
				DBHost: "127.0.0.1", CreateDB: m.projectCreateDB,
				StartService: m.projectStart,
			},
		})
	case "projectDirs":
		switch value {
		case dirsRefresh:
			return m.loadProjectDirs()
		case dirsAdd:
			m.formText2 = ""
			return m.openFormCmd("projectDirsAdd", m.inputField("Carpeta a añadir", "~/Lair", &m.formText2))
		case dirsRemove:
			m.formText2 = ""
			return m.openFormCmd("projectDirsRemove", m.inputField("Carpeta a quitar", "~/Lair", &m.formText2))
		case dirsImportValet:
			return m.runDirsOp("Importando carpetas de Valet…", "import-valet",
				daemon.ProjectRequest{Op: "import-valet"})
		case dirsMigrate:
			return m.runDirsOp("Migrando la antigua ~/lair…", "migrate-dirs",
				daemon.ProjectRequest{Op: "migrate-dirs", From: "~/lair", To: "~/Lair"})
		}
		return m.fail("carpetas de proyectos", "acción no reconocida")
	case "projectDirsAdd":
		dir := strings.TrimSpace(m.formText2)
		if dir == "" {
			return m.fail("añadir carpeta", "escribe una carpeta")
		}
		return m.runDirsOp("Añadiendo carpeta…", "dirs-add",
			daemon.ProjectRequest{Op: "dirs-add", Dir: dir})
	case "projectDirsRemove":
		dir := strings.TrimSpace(m.formText2)
		if dir == "" {
			return m.fail("quitar carpeta", "escribe una carpeta")
		}
		return m.runDirsOp("Quitando carpeta…", "dirs-remove",
			daemon.ProjectRequest{Op: "dirs-remove", Dir: dir})
	case "useVersion":
		parts := strings.SplitN(value, ":", 2)
		if len(parts) != 2 {
			return m.fail("versión", "selección no válida")
		}
		return m.run("Activando versión…", "versión del servicio", daemon.ActionServiceUse, daemon.ServiceUseRequest{ID: parts[0], Version: parts[1]})
	case "uninstall":
		if !m.confirmValue {
			m.message = styleSubtle.Render("desinstalación cancelada")
			return nil
		}
		parts := strings.SplitN(value, ":", 2)
		if len(parts) != 2 {
			return m.fail("desinstalar", "selección no válida")
		}
		return m.run("Desinstalando…", "desinstalar", daemon.ActionServiceUninstall, daemon.ServiceUninstallRequest{ID: parts[0], Version: parts[1]})
	case "createDB":
		name := strings.TrimSpace(m.formText2)
		if name == "" {
			return m.fail("crear BD", "el nombre es obligatorio")
		}
		return m.run("Creando base de datos…", "crear BD", daemon.ActionCreateDB, daemon.CreateDBRequest{ID: value, Name: name})
	case "pin":
		version := strings.TrimSpace(m.formText)
		if version == "" {
			return m.fail("pin", "la versión es obligatoria")
		}
		return m.run("Fijando versión…", "pin", daemon.ActionPin, daemon.PinRequest{Tool: m.formTool, Value: version, Dir: m.sitePath(m.formText2)})
	case "isolate":
		version := strings.TrimSpace(m.formText2)
		if version == "--off" || strings.EqualFold(version, "off") {
			version = ""
		}
		return m.run("Aislando sitio…", "aislar", daemon.ActionIsolate, daemon.IsolateRequest{
			Tool: m.formTool, Value: version, Dir: m.sitePath(value),
		})
	default:
		return m.fail("acción", "diálogo desconocido")
	}
}
