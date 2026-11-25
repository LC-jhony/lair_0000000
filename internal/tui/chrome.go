package tui

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
)

type page int

const (
	pageDashboard page = iota
	pageRuntimes
	pageDatabases
	pageServices
	pageSites
	pageMail
	pageDumps
	pageDebugger
	pageLogs
	pageSystem
)

var pageTitles = map[page]string{
	pageDashboard: "Dashboard",
	pageRuntimes:  "PHP & Node",
	pageDatabases: "Databases",
	pageServices:  "Services",
	pageSites:     "Sites",
	pageMail:      "Mail",
	pageDumps:     "Dumps",
	pageDebugger:  "Debugger",
	pageLogs:      "Logs",
	pageSystem:    "System",
}

func (p page) title() string {
	if t, ok := pageTitles[p]; ok {
		return t
	}
	return "Dashboard"
}

// layout returns the column widths and the height available to the main view.
func (m *Model) layout() (sideW, mainW, bodyH int) {
	bodyH = max(0, m.height-3)
	switch {
	case m.width >= 110:
		sideW = 32
	case m.width >= 96:
		sideW = 22
	default:
		sideW = 0
	}
	if m.sidebarOverlay {
		sideW = min(32, m.width-8)
	}
	if sideW > 0 {
		mainW = max(0, m.width-sideW-1)
	} else {
		mainW = m.width
	}
	return
}

func (m *Model) renderHeader(w int) string {
	title := m.page.title()
	if m.page == pageSites && m.selectedSite != "" {
		title += "  /  " + m.selectedSite
	}
	if m.page == pageServices && m.selectedService != "" {
		title += "  /  " + m.selectedService
	}
	if m.busy != "" {
		title += "  " + m.th.accent.Render(m.spin.View()+" "+m.busy)
	}
	version := m.snap.Status.Home
	if len(version) > 28 {
		version = "…" + version[len(version)-27:]
	}
	left := m.th.title.Render(" lair  " + title)
	right := m.th.dim.Render(version + "  ")
	gap := w - lipgloss.Width(left) - lipgloss.Width(right)
	if gap < 1 {
		return truncate(left, w)
	}
	return left + strings.Repeat(" ", gap) + right
}

// hint renders the single bottom line: health counters on the left, keys of
// the focused surface on the right.
func (m *Model) renderHint(w int) string {
	left := fmt.Sprintf("%d needs attention · %d services · %d sites", m.snap.attention(), runningServices(m.snap), len(m.snap.Status.Sites))
	if len(m.snap.Jobs) > 0 {
		left += fmt.Sprintf(" · %d jobs", len(m.snap.Jobs))
	}
	keys := "↑↓ navigate · enter open · tab focus · ctrl+p palette · ? help · q quit"
	if m.focusSidebar() {
		keys = "↑↓ navigate · enter open · tab main · R refresh · q quit"
	}
	if m.paletteOn {
		keys = "type filter · ↑↓ select · enter run · esc close"
	} else if m.helpOn {
		keys = "esc close help"
	} else if m.filterOn {
		keys = "type filter · enter apply · esc cancel"
	}
	gap := w - len(left) - len(keys)
	if gap < 2 {
		keys = "ctrl+p palette · ? help · q quit"
		gap = w - len(left) - len(keys)
	}
	if gap < 1 {
		return truncate(left, w)
	}
	return m.th.hint.Render(left + strings.Repeat(" ", gap) + keys)
}

func runningServices(s Snapshot) int {
	n := 0
	for _, svc := range s.Status.Services {
		if svc.Running {
			n++
		}
	}
	return n
}

// panel renders a titled card. Sections that do not fit are omitted instead
// of being cut in half.
func (m *Model) panel(title, body string, w, h int) string {
	if w < 16 || h < 3 {
		return ""
	}
	head := m.th.accent.Render(" " + title + " ")
	innerW := w - 2
	innerH := h - 2
	if lipgloss.Height(body) > innerH {
		body = strings.Join(strings.Split(body, "\n")[:max(1, innerH)], "\n")
	}
	body = m.th.card.Render(lipgloss.PlaceHorizontal(innerW, lipgloss.Left, truncateLines(body, innerW)))
	return head + "\n" + body + "\n" + m.th.dim.Render(strings.Repeat("─", min(w, innerW)))
}

func truncateLines(s string, w int) string {
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		lines[i] = truncate(line, w)
	}
	return strings.Join(lines, "\n")
}

func statusDot(ok bool, th theme) string {
	if ok {
		return th.good.Render("●")
	}
	return th.bad.Render("✖")
}

func truncatedName(s string, n int) string { return truncate(s, n) }

// tabs renders a one-line tab bar and marks the active tab.
func (m *Model) tabs(w int, names []string, active int) string {
	out := []string{}
	for i, name := range names {
		label := fmt.Sprintf(" %d %s ", i+1, name)
		if i == active {
			out = append(out, m.th.tabActive.Render(label))
		} else {
			out = append(out, m.th.tab.Render(label))
		}
	}
	return truncateLines(strings.Join(out, "│"), w)
}

// kv renders a two-column fact table.
func (m *Model) kv(w int, pairs [][2]string) string {
	lines := []string{}
	for _, p := range pairs {
		lines = append(lines, " "+pad(p[0], 14)+m.th.dim.Render(p[1]))
	}
	return truncateLines(strings.Join(lines, "\n"), w)
}
