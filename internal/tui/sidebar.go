package tui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	zone "github.com/lrstanley/bubblezone/v2"
)

type navKind int

const (
	navPage navKind = iota
	navSite
	navService
)

type navItem struct {
	kind  navKind
	page  page
	name  string
	label string
	group string
}

func (m *Model) buildNav() []navItem {
	items := []navItem{}
	pages := []page{pageDashboard, pageRuntimes, pageDatabases, pageServices, pageSites, pageMail, pageDumps, pageDebugger, pageLogs, pageSystem}
	for _, p := range pages {
		items = append(items, navItem{kind: navPage, page: p, label: p.title(), group: "Navigate"})
	}
	// Sites are grouped by their first project dir; the label keeps the host.
	for _, site := range m.snap.Status.Sites {
		group := "Sites"
		for _, dir := range m.snap.ProjectDirs {
			if strings.HasPrefix(site.Root, dir) {
				group = "  " + truncate(dir, 14)
				break
			}
		}
		name := site.Name
		label := name
		if !site.Enabled {
			label += " (off)"
		}
		items = append(items, navItem{kind: navSite, page: pageSites, name: name, label: label, group: group})
	}
	for id, svc := range m.snap.Status.Services {
		if len(svc.Installed) == 0 {
			continue
		}
		status := "○"
		if svc.Running {
			status = "●"
		}
		items = append(items, navItem{kind: navService, page: pageServices, name: id, label: status + " " + id, group: "Services"})
	}
	return items
}

func (m *Model) selectedNav() (navItem, bool) {
	if m.navIdx < 0 || m.navIdx >= len(m.nav) {
		return navItem{}, false
	}
	return m.nav[m.navIdx], true
}

func (m *Model) openNav(item navItem) tea.Cmd {
	switch item.kind {
	case navSite:
		m.selectedSite = item.name
		m.page = pageSites
		m.tab = 0
	case navService:
		m.selectedService = item.name
		m.page = pageServices
		m.tab = 0
	default:
		m.page = item.page
	}
	m.sidebarOverlay = false
	return m.snapshotCmd()
}

func (m *Model) renderSidebar(w, h int) string {
	if w <= 0 || h <= 0 {
		return ""
	}
	group := ""
	out := []string{m.th.accent.Render(" lair ")}
	for i, item := range m.nav {
		if item.group != group {
			group = item.group
			out = append(out, m.th.muted.Render(" "+strings.ToUpper(group)))
		}
		label := item.label
		if w < 26 {
			label = truncate(label, w-4)
		} else {
			label = pad(label, w-4)
		}
		row := " " + label
		if i == m.navIdx && m.focus == focusSidebar {
			row = m.th.selected.Render(row)
		} else if i == m.navIdx {
			row = m.th.card.Render(row)
		} else {
			row = m.th.fg.Render(row)
		}
		out = append(out, zone.Mark(fmt.Sprintf("nav-%d", i), row))
	}
	if len(out) > h {
		out = out[:h]
	}
	view := strings.Join(out, "\n")
	return m.th.sidebar.Render(lipgloss.PlaceVertical(h, lipgloss.Top, padBlock(view, w)))
}

func padBlock(s string, w int) string {
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		lines[i] = pad(line, w)
	}
	return strings.Join(lines, "\n")
}
