package tui

import (
	"sort"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	zone "github.com/lrstanley/bubblezone/v2"
)

type paletteEntry struct {
	label string
	group string
	run   func(*Model) tea.Cmd
}

func (m *Model) paletteEntries() []paletteEntry {
	entries := []paletteEntry{
		{label: "Refresh all data", group: "Fixes", run: func(m *Model) tea.Cmd { return m.snapshotCmd() }},
		{label: "Open dashboard", group: "Places", run: func(m *Model) tea.Cmd { m.page = pageDashboard; return nil }},
		{label: "Open PHP & Node", group: "Places", run: func(m *Model) tea.Cmd { m.page = pageRuntimes; return nil }},
		{label: "Open Databases", group: "Places", run: func(m *Model) tea.Cmd { m.page = pageDatabases; return nil }},
		{label: "Open Services", group: "Places", run: func(m *Model) tea.Cmd { m.page = pageServices; return nil }},
		{label: "Open Sites", group: "Places", run: func(m *Model) tea.Cmd { m.page = pageSites; return nil }},
		{label: "Open Mail", group: "Places", run: func(m *Model) tea.Cmd { m.page = pageMail; return nil }},
		{label: "Open Dumps", group: "Places", run: func(m *Model) tea.Cmd { m.page = pageDumps; return nil }},
		{label: "Open Debugger", group: "Places", run: func(m *Model) tea.Cmd { m.page = pageDebugger; return nil }},
		{label: "Open Logs", group: "Places", run: func(m *Model) tea.Cmd { m.page = pageLogs; return nil }},
		{label: "Open System", group: "Places", run: func(m *Model) tea.Cmd { m.page = pageSystem; return nil }},
		{label: "Create new Laravel project", group: "Actions", run: func(m *Model) tea.Cmd { m.openProjectForm(); return nil }},
		{label: "Install PHP version", group: "Actions", run: func(m *Model) tea.Cmd { m.openInstallForm("php"); return nil }},
		{label: "Install Node version", group: "Actions", run: func(m *Model) tea.Cmd { m.openInstallForm("node"); return nil }},
		{label: "Install a service", group: "Actions", run: func(m *Model) tea.Cmd { m.openInstallForm("service"); return nil }},
		{label: "Toggle HTTPS on selected site", group: "Actions", run: func(m *Model) tea.Cmd { return m.toggleHTTPS() }},
		{label: "Reload generated vhost", group: "Actions", run: func(m *Model) tea.Cmd { return m.loadConfig() }},
		{label: "Refresh site logs", group: "Actions", run: func(m *Model) tea.Cmd { return m.loadLogs() }},
		{label: "Restart selected service", group: "Actions", run: func(m *Model) tea.Cmd { return m.restartService() }},
	}
	for _, name := range m.siteNames() {
		site := name
		entries = append(entries, paletteEntry{label: "Site: " + site, group: "Places", run: func(m *Model) tea.Cmd {
			m.selectedSite, m.page, m.tab = site, pageSites, 0
			return m.snapshotCmd()
		}})
	}
	for id := range m.snap.Status.Services {
		svc := id
		entries = append(entries, paletteEntry{label: "Service: " + svc, group: "Places", run: func(m *Model) tea.Cmd {
			m.selectedService, m.page, m.tab = svc, pageServices, 0
			return m.snapshotCmd()
		}})
	}
	for _, s := range m.settings() {
		setting := s
		entries = append(entries, paletteEntry{label: "Toggle " + setting.label, group: "Settings", run: func(m *Model) tea.Cmd {
			return m.toggleSetting(setting.action)
		}})
	}
	sort.SliceStable(entries, func(i, j int) bool { return entries[i].label < entries[j].label })
	return entries
}

// fuzzy reports whether all runes of the filter appear in order in label.
func fuzzy(filter, label string) bool {
	f, l := []rune(strings.ToLower(filter)), []rune(strings.ToLower(label))
	if len(f) == 0 {
		return true
	}
	j := 0
	for _, r := range l {
		if j < len(f) && r == f[j] {
			j++
		}
	}
	return j == len(f)
}

func (m *Model) filteredPalette() []paletteEntry {
	all := m.paletteEntries()
	out := make([]paletteEntry, 0, len(all))
	for _, e := range all {
		if fuzzy(m.paletteFilter, e.label+" "+e.group) {
			out = append(out, e)
		}
	}
	return out
}

func (m *Model) renderPaletteOverlay(w, h int) string {
	entries := m.filteredPalette()
	if len(entries) > 12 {
		entries = entries[:12]
	}
	ow := min(w-4, 72)
	oh := min(h-4, len(entries)+4)
	lines := []string{m.th.accent.Render(" Command palette "), "> " + m.paletteFilter + "_"}
	for i, e := range entries {
		label := pad(e.group+": "+e.label, ow-4)
		if i == m.paletteIdx {
			lines = append(lines, zone.Mark("pal-"+itoa(i), m.th.selected.Render("▸ "+label)))
		} else {
			lines = append(lines, zone.Mark("pal-"+itoa(i), "  "+label))
		}
	}
	box := lipgloss.NewStyle().Width(ow).Height(oh).Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("63")).Render(strings.Join(lines, "\n"))
	return lipgloss.Place(w, h, lipgloss.Center, lipgloss.Center, box)
}
