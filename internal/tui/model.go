package tui

import (
	"context"
	"strings"
	"time"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	"charm.land/huh/v2"
	"charm.land/lipgloss/v2"
	zone "github.com/lrstanley/bubblezone/v2"

	"github.com/lair-sh/lair/internal/daemon"
)

type focus int

const (
	focusSidebar focus = iota
	focusMain
)

// Model is the single root of the Bubble Tea program. It owns UI state only;
// all domain data lives in Snapshot, which is replaced as a whole on refresh.
type Model struct {
	ctx    context.Context
	cancel context.CancelFunc
	caller daemon.Caller
	send   func(tea.Msg)

	snap     Snapshot
	versions map[string][]string
	th       theme
	dark     bool

	width, height int
	page          page
	focus         focus
	nav           []navItem
	navIdx        int

	selectedSite    string
	selectedService string
	tab             int
	settingIdx      int
	lens            int
	logSourceIdx    int

	sidebarOverlay bool
	helpOn         bool
	paletteOn      bool
	paletteFilter  string
	paletteIdx     int
	filterOn       bool
	filterBuf      string
	promptOn       bool
	promptBuf      string

	form     *huh.Form
	formKind formKind
	formVals formValues

	envEntries []daemon.SiteEnvEntry
	configText string
	logText    string
	toasts     []Toast
	spin       spinner.Model
	busy       string
	started    bool
}

func newModel(ctx context.Context, cancel context.CancelFunc, caller daemon.Caller) *Model {
	zone.NewGlobal()
	m := &Model{
		ctx: ctx, cancel: cancel, caller: caller,
		versions: map[string][]string{},
		dark:     true,
		navIdx:   0,
		spin:     spinner.New(spinner.WithSpinner(spinner.Dot)),
	}
	m.th = newTheme(m.dark)
	return m
}

func (m *Model) Init() tea.Cmd {
	m.started = true
	return tea.Batch(m.snapshotCmd(), m.loadVersionsCmd("php"), m.loadVersionsCmd("node"), tickCmd(3*time.Second))
}

func (m *Model) focusSidebar() bool { return m.focus == focusSidebar }

func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch ev := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = ev.Width, ev.Height
		return m, nil
	case tea.BackgroundColorMsg:
		m.dark = ev.IsDark()
		m.th = newTheme(m.dark)
		return m, nil
	case snapshotMsg:
		oldSite, oldService := m.selectedSite, m.selectedService
		m.snap = ev.snap
		m.rebuildNav(oldSite, oldService)
		return m, nil
	case versionsMsg:
		for k, v := range ev.versions {
			m.versions[k] = v
		}
		return m, nil
	case tickMsg:
		m.pruneToasts(time.Now())
		return m, tea.Batch(m.snapshotCmd(), tickCmd(3*time.Second))
	case actionDoneMsg:
		if ev.err != nil {
			m.notify(ev.label+": "+ev.err.Error(), ev.detail, true)
		} else {
			m.notify(ev.label, ev.detail, false)
		}
		m.busy = ""
		return m, m.snapshotCmd()
	case tea.MouseClickMsg:
		return m, m.handleMouse(ev.Mouse())
	case tea.KeyPressMsg:
		return m, m.handleKey(ev)
	}
	if m.form != nil {
		updated, cmd := m.form.Update(msg)
		if f, ok := updated.(*huh.Form); ok {
			m.form = f
		}
		if m.form.State == huh.StateCompleted {
			return m, tea.Batch(m.submitForm(), m.snapshotCmd())
		}
		if m.form.State == huh.StateAborted {
			m.form, m.formKind = nil, formNone
		}
		return m, cmd
	}
	var cmd tea.Cmd
	m.spin, cmd = m.spin.Update(msg)
	return m, cmd
}

func (m *Model) rebuildNav(oldSite, oldService string) {
	items := m.buildNav()
	m.nav = items
	m.navIdx = 0
	for i, item := range items {
		if (item.kind == navSite && item.name == oldSite) || (item.kind == navService && item.name == oldService) {
			m.navIdx = i
			break
		}
	}
}

// handleMouse resolves clickable regions built with bubblezone. Zones are
// stored by id after View(), so lookups here always refer to the last frame.
func (m *Model) handleMouse(mouse tea.Mouse) tea.Cmd {
	if mouse.Button != tea.MouseLeft {
		return nil
	}
	for i := range m.nav {
		z := zone.Get("nav-" + itoa(i))
		if z != nil && z.InBounds(tea.MouseClickMsg(mouse)) {
			m.navIdx = i
			m.focus = focusSidebar
			return m.openNav(m.nav[i])
		}
	}
	if m.paletteOn {
		entries := m.filteredPalette()
		for i := range entries {
			z := zone.Get("pal-" + itoa(i))
			if z != nil && z.InBounds(tea.MouseClickMsg(mouse)) {
				m.paletteOn = false
				return entries[i].run(m)
			}
		}
	}
	return nil
}

func (m *Model) handleKey(key tea.KeyPressMsg) tea.Cmd {
	s := key.String()

	// Forms capture every key.
	if m.form != nil {
		_, cmd := m.form.Update(key)
		if m.form.State == huh.StateCompleted {
			return tea.Batch(m.submitForm(), m.snapshotCmd())
		}
		if m.form.State == huh.StateAborted {
			m.form, m.formKind = nil, formNone
		}
		return cmd
	}
	if m.helpOn {
		if s == "esc" || s == "?" || s == "q" {
			m.helpOn = false
		}
		return nil
	}
	if m.paletteOn {
		return m.handlePaletteKey(s)
	}
	if m.filterOn {
		return m.handleFilterKey(s)
	}
	if m.promptOn {
		return m.handlePromptKey(s)
	}

	switch s {
	case "ctrl+p":
		m.paletteOn, m.paletteIdx, m.paletteFilter = true, 0, ""
		return nil
	case "q", "ctrl+c":
		return tea.Quit
	case "esc":
		if m.sidebarOverlay {
			m.sidebarOverlay = false
		}
		return nil
	case "tab":
		if m.focus == focusSidebar {
			m.focus = focusMain
		} else {
			m.focus = focusSidebar
		}
		return nil
	case `\`:
		m.sidebarOverlay = !m.sidebarOverlay
		return nil
	case "?":
		m.helpOn = true
		return nil
	case ":":
		m.promptOn, m.promptBuf = true, ""
		return nil
	case "/":
		m.filterOn, m.filterBuf = true, ""
		return nil
	case "R":
		m.busy = "refreshing"
		return m.snapshotCmd()
	}

	if m.focus == focusSidebar {
		switch s {
		case "up", "k":
			m.navIdx--
			if m.navIdx < 0 {
				m.navIdx = len(m.nav) - 1
			}
		case "down", "j":
			m.navIdx++
			if m.navIdx >= len(m.nav) {
				m.navIdx = 0
			}
		case "enter":
			if item, ok := m.selectedNav(); ok {
				return m.openNav(item)
			}
		}
		return nil
	}

	// Main area keys.
	switch s {
	case "up", "k":
		m.moveUp()
	case "down", "j":
		m.moveDown()
	case "enter":
		return m.activate()
	case "1", "2", "3", "4", "5", "6":
		m.tab = int(s[0] - '1')
	case "[":
		m.lens = (m.lens + len(debugLenses) - 1) % len(debugLenses)
		m.logSourceIdx = (m.logSourceIdx + len(m.snap.LogSources) - 1) % max(1, len(m.snap.LogSources))
	case "]":
		m.lens = (m.lens + 1) % len(debugLenses)
		m.logSourceIdx = (m.logSourceIdx + 1) % max(1, len(m.snap.LogSources))
	case "o":
		m.navIdx = (m.navIdx + 1) % max(1, len(m.nav))
	case "i":
		m.openInstallForm("php")
	case "p":
		m.openProjectForm()
	case "l", "v":
		m.busy = "loading logs"
		return m.loadLogs()
	case "s":
		m.busy = "restarting"
		return m.restartService()
	case "x":
		if m.page == pageSites {
			m.openDomainForm()
		}
	case "d":
		m.busy = "updating default"
		return m.setDefault()
	case "e":
		if m.page == pageSites {
			m.openDomainForm()
		}
	case "c":
		if m.page == pageSites {
			m.busy = "loading vhost"
			return m.loadConfig()
		}
	case "n":
		m.openCreateDBForm()
	case "h", "b":
		if m.page == pageSites {
			m.busy = "toggling HTTPS"
			return m.toggleHTTPS()
		}
	case " ":
		if m.page == pageSystem && m.lens == 0 {
			settings := m.settings()
			if len(settings) > 0 {
				m.settingIdx = (m.settingIdx + len(settings)) % len(settings)
				m.busy = "updating setting"
				return m.toggleSetting(settings[m.settingIdx].action)
			}
		}
	}
	return nil
}

func (m *Model) moveUp() {
	switch m.page {
	case pageSystem:
		m.settingIdx--
		if m.settingIdx < 0 {
			m.settingIdx = len(m.settings()) - 1
		}
	default:
		m.navIdx--
		if m.navIdx < 0 {
			m.navIdx = len(m.nav) - 1
		}
	}
}

func (m *Model) moveDown() {
	switch m.page {
	case pageSystem:
		m.settingIdx++
		if m.settingIdx >= len(m.settings()) {
			m.settingIdx = 0
		}
	default:
		m.navIdx++
		if m.navIdx >= len(m.nav) {
			m.navIdx = 0
		}
	}
}

func (m *Model) activate() tea.Cmd {
	if item, ok := m.selectedNav(); ok && m.navIdx >= 0 {
		return m.openNav(item)
	}
	return nil
}

func (m *Model) handlePaletteKey(s string) tea.Cmd {
	switch s {
	case "esc", "ctrl+p":
		m.paletteOn = false
	case "up":
		m.paletteIdx--
	case "down":
		m.paletteIdx++
	case "enter":
		entries := m.filteredPalette()
		if len(entries) == 0 {
			return nil
		}
		if m.paletteIdx < 0 || m.paletteIdx >= len(entries) {
			m.paletteIdx = 0
		}
		entry := entries[m.paletteIdx]
		m.paletteOn = false
		return entry.run(m)
	case "backspace":
		if len(m.paletteFilter) > 0 {
			m.paletteFilter = m.paletteFilter[:len(m.paletteFilter)-1]
		}
	default:
		if len(s) == 1 && s[0] >= 32 {
			m.paletteFilter += s
			m.paletteIdx = 0
		}
	}
	if entries := m.filteredPalette(); len(entries) > 0 {
		if m.paletteIdx < 0 {
			m.paletteIdx = 0
		}
		if m.paletteIdx >= len(entries) {
			m.paletteIdx = len(entries) - 1
		}
	}
	return nil
}

func (m *Model) handleFilterKey(s string) tea.Cmd {
	switch s {
	case "esc":
		m.filterOn = false
	case "enter":
		m.filterOn = false
		return m.snapshotCmd()
	case "backspace":
		if len(m.filterBuf) > 0 {
			m.filterBuf = m.filterBuf[:len(m.filterBuf)-1]
		}
	default:
		if len(s) == 1 && s[0] >= 32 {
			m.filterBuf += s
		}
	}
	return nil
}

func (m *Model) handlePromptKey(s string) tea.Cmd {
	switch s {
	case "esc":
		m.promptOn = false
	case "enter":
		command := strings.TrimSpace(strings.ToLower(m.promptBuf))
		m.promptOn, m.promptBuf = false, ""
		switch command {
		case "r", "refresh":
			return m.snapshotCmd()
		case "?", "help":
			m.helpOn = true
		case "install php":
			m.openInstallForm("php")
		case "install node":
			m.openInstallForm("node")
		case "new", "project":
			m.openProjectForm()
		case "":
		default:
			m.notify("unknown command: "+command, "", true)
		}
	case "backspace":
		if len(m.promptBuf) > 0 {
			m.promptBuf = m.promptBuf[:len(m.promptBuf)-1]
		}
	default:
		if len(s) == 1 && s[0] >= 32 {
			m.promptBuf += s
		}
	}
	return nil
}

func (m *Model) View() tea.View {
	if m.width < 60 || m.height < 12 {
		v := tea.NewView(m.th.warn.Render("Terminal too small — please resize to at least 60x12"))
		v.AltScreen = true
		return v
	}
	w, h := m.width, m.height
	sideW, mainW, bodyH := m.layout()
	header := m.renderHeader(w)
	body := m.renderPage(mainW, bodyH)
	hint := m.renderHint(w)

	content := header + "\n"
	if sideW > 0 {
		content += lipgloss.JoinHorizontal(lipgloss.Top,
			m.renderSidebar(sideW, bodyH), " ", body)
	} else {
		content += body
	}
	content += "\n" + hint

	toasts := m.renderToasts(w, h)
	if toasts != "" {
		content = overlay(content, toasts, w, h)
	}
	if m.helpOn {
		content = overlay(content, m.renderHelpOverlay(w, h), w, h)
	}
	if m.paletteOn {
		content = overlay(content, m.renderPaletteOverlay(w, h), w, h)
	}
	if m.form != nil {
		content = overlay(content, m.form.View(), w, h)
	}
	if m.promptOn {
		line := m.th.input.Render(": " + m.promptBuf + "_")
		content = overlay(content, lipgloss.Place(w, 3, lipgloss.Center, lipgloss.Bottom, line), w, h)
	}

	v := tea.NewView(zone.Scan(content))
	v.AltScreen = true
	v.MouseMode = tea.MouseModeCellMotion
	return v
}

func (m *Model) renderPage(w, h int) string {
	switch m.page {
	case pageRuntimes:
		return m.renderRuntimes(w, h)
	case pageDatabases:
		return m.renderDatabases(w, h)
	case pageServices:
		return m.renderServices(w, h)
	case pageSites:
		return m.renderSite(w, h)
	case pageMail, pageDumps, pageDebugger:
		return m.renderDebug(w, h, m.page)
	case pageLogs:
		return m.renderLogs(w, h)
	case pageSystem:
		if m.lens > 0 {
			return m.renderDebug(w, h, pageSystem)
		}
		return m.renderSystem(w, h)
	default:
		return m.renderDashboard(w, h)
	}
}

// overlay draws top over base at the top-left corner (top is already placed).
func overlay(base, top string, w, h int) string {
	baseLines := strings.Split(base, "\n")
	topLines := strings.Split(top, "\n")
	for i, line := range topLines {
		if i >= len(baseLines) {
			break
		}
		if lipgloss.Width(line) < lipgloss.Width(baseLines[i]) {
			line += strings.Repeat(" ", lipgloss.Width(baseLines[i])-lipgloss.Width(line))
		}
		baseLines[i] = line
	}
	return strings.Join(baseLines, "\n")
}
