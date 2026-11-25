package tui

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/lair-sh/lair/internal/engine"
)

var siteTabs = []string{"Overview", "Logs", "Env", "Config", "Doctor", "Stack"}

func (m *Model) renderSite(w, h int) string {
	name := m.selectedSite
	if name == "" {
		names := m.siteNames()
		if len(names) > 0 {
			name = names[0]
			m.selectedSite = name
		} else {
			return m.th.dim.Render("No parked sites. Press i to install or create a project with p.")
		}
	}
	d, ok := m.snap.Details[name]
	if !ok {
		d.Name = name
	}
	head := []string{
		m.th.title.Render(" " + name),
		" " + d.URL + "  " + m.th.dim.Render(d.Framework+"  PHP "+d.PHP+"  Node "+d.Node),
		" " + m.th.dim.Render(d.Path),
		" " + (map[bool]string{true: m.th.good.Render("HTTPS + enabled"), false: m.th.warn.Render("plain HTTP")}[d.Secure]) +
			"  root: " + statusText(d.RootState),
	}
	header := strings.Join(head, "\n")
	tabLine := m.tabs(w, siteTabs, m.tab)
	bodyH := max(2, h-lipgloss.Height(header)-2)
	var body string
	switch m.tab {
	case 1:
		body = m.renderSiteLogs(w, bodyH)
	case 2:
		body = m.renderSiteEnv(w, bodyH)
	case 3:
		body = m.renderSiteConfig(w, bodyH)
	case 4:
		body = m.renderSiteDoctor(w, bodyH, d)
	case 5:
		body = m.renderSiteStack(w, bodyH, d.Name)
	default:
		body = m.renderSiteOverview(w, bodyH, d)
	}
	return header + "\n" + tabLine + "\n" + body
}

// renderSiteStack shows the technologies detected from the project files.
// Credentials are never part of the snapshot, so nothing sensitive is shown.
func (m *Model) renderSiteStack(w, bodyH int, name string) string {
	s, ok := m.snap.Stacks[name]
	if !ok {
		return m.th.dim.Render(" No stack data. Press R to refresh.")
	}
	pairs := [][2]string{
		{"framework", strings.TrimSpace(s.Framework + " " + s.FrameworkVersion)},
		{"database", strings.TrimSpace(s.Database + " " + s.DatabaseName)},
		{"db server", s.DatabaseHost},
		{"cache", s.Cache},
		{"web root", s.WebRoot},
		{"front", s.FrontController},
		{"server", s.Server},
		{"php / node", strings.TrimSpace(s.PHP + " / " + s.Node)},
	}
	if s.Error != "" {
		pairs = append(pairs, [2]string{"error", s.Error})
	}
	lines := m.kv(w/2, pairs)
	if len(s.Components) > 0 {
		lines += "\n components: " + strings.Join(s.Components, ", ")
	}
	return truncateLines(lines, w)
}

func statusText(state string) string {
	switch state {
	case "ok":
		return "ok"
	case "":
		return "unknown"
	default:
		return state
	}
}

func (m *Model) renderSiteOverview(w, h int, d engine.SiteDetail) string {
	data := m.snap.SiteData[d.Name]
	q := m.snap.Queues[d.Name]
	pairs := [][2]string{
		{"framework", d.Framework},
		{"php", d.PHP + " " + d.PHPVersion},
		{"node", d.Node},
		{"database", d.Database + " / " + d.DatabaseName},
		{"domains", strings.Join(d.Domains, ", ")},
		{"root", d.RootState},
		{"tables", fmt.Sprintf("%d tracked", len(data.Tables))},
		{"queue", fmt.Sprintf("%d pending / %d failed", q.Pending, q.Failed)},
	}
	left := m.kv(w/2, pairs)
	right := []string{" Suggested actions:", "  h  toggle HTTPS", "  e  edit domains", "  c  view vhost", "  v  logs", "  R  refresh"}
	if q.Hint != "" {
		right = append(right, "  "+truncate(q.Hint, w/2-2))
	}
	if d.Problem != "" {
		right = append(right, m.th.warn.Render(" "+d.Problem))
	}
	if d.Notice != "" {
		right = append(right, m.th.dim.Render(" "+d.Notice))
	}
	return lipgloss.JoinHorizontal(lipgloss.Top,
		m.th.card.Render(lipgloss.PlaceVertical(h, lipgloss.Top, truncateLines(left, w/2-1))),
		lipgloss.PlaceVertical(h, lipgloss.Top, truncateLines(strings.Join(right, "\n"), w-w/2-2)))
}

func (m *Model) renderSiteLogs(w, h int) string {
	if m.logText == "" {
		return m.th.dim.Render(" No log loaded. Press l to load from the selected source.")
	}
	return truncateLines(m.logText, w)
}

func (m *Model) renderSiteEnv(w, h int) string {
	if len(m.envEntries) == 0 {
		return m.th.dim.Render(" No .env loaded. Press 2 then R to read it (values stay masked).")
	}
	lines := []string{}
	for _, e := range m.envEntries {
		value := e.Value
		if e.Secret {
			value = "••••••••"
		}
		if !e.Set {
			value = "(unset)"
		}
		lines = append(lines, " "+pad(e.Key, 26)+m.th.dim.Render(truncate(value, w-30)))
	}
	return truncateLines(strings.Join(lines, "\n"), w)
}

func (m *Model) renderSiteConfig(w, h int) string {
	if m.configText == "" {
		return m.th.dim.Render(" Press c to load the generated vhost (read-only).")
	}
	return truncateLines(m.configText, w)
}

func (m *Model) renderSiteDoctor(w, h int, d engine.SiteDetail) string {
	lines := []string{
		" read-only health checks for " + d.Name,
		"",
		" root       " + statusText(d.RootState),
		" https      " + boolText(d.Secure),
		" enabled    " + boolText(d.Enabled),
		" path       " + truncate(d.Path, w-14),
	}
	for _, h := range m.snap.Health {
		lines = append(lines, " "+pad(h.ID, 12)+boolText(h.Healthy)+" "+truncate(h.Detail, w-22))
	}
	return truncateLines(strings.Join(lines, "\n"), w)
}
