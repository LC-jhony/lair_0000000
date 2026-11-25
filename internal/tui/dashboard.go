package tui

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
)

// renderDashboard is the initial screen. Needs-attention cards come first;
// Resources and System end on the same row when there is room.
func (m *Model) renderDashboard(w, h int) string {
	if w < 40 || h < 10 {
		return m.th.dim.Render("Window too small — resize to at least 60x12")
	}
	attention := m.attentionLines()
	headHeight := max(4, min(9, len(attention)+2))
	head := m.panel("NEEDS ATTENTION", strings.Join(attention, "\n"), w, headHeight)
	rest := h - headHeight - 1
	if rest <= 0 {
		return head
	}
	left := m.resourceLines()
	right := m.systemLines()
	if w >= 96 {
		colW := w / 2
		if colW < 32 {
			colW = 32
		}
		l := m.panel("RESOURCES", strings.Join(left, "\n"), colW-1, rest)
		r := m.panel("SYSTEM", strings.Join(right, "\n"), w-colW, rest)
		return head + "\n" + lipgloss.JoinHorizontal(lipgloss.Top, l, r)
	}
	// Narrow: stack, system first.
	out := head + "\n" + m.panel("SYSTEM", strings.Join(right, "\n"), w, max(4, rest/2))
	if rest > 5 {
		out += "\n" + m.panel("RESOURCES", strings.Join(left, "\n"), w, rest/2)
	}
	return out
}

func (m *Model) attentionLines() []string {
	out := []string{}
	for _, h := range m.snap.Health {
		if !h.Healthy {
			out = append(out, m.th.bad.Render("✖ service ")+h.ID+"  "+truncate(h.Detail, 32))
		}
	}
	if s := m.snap.Status; s.Web.Error != "" || s.Web.Warning != "" {
		text := s.Web.Error
		if text == "" {
			text = s.Web.Warning
		}
		out = append(out, m.th.warn.Render("◐ web  ")+truncate(text, 44))
	}
	if d := m.snap.Status.DNS; d.Host != "" && !d.OK {
		out = append(out, m.th.bad.Render("✖ dns  ")+truncate(d.Detail, 44))
	}
	for _, name := range m.siteNames() {
		if detail, ok := m.snap.Details[name]; ok && detail.RootState != "" && detail.RootState != "ok" {
			out = append(out, m.th.warn.Render("◐ ")+name+"  root: "+detail.RootState)
		}
	}
	for _, j := range m.snap.Jobs {
		if j.Error != "" && !j.Canceled {
			out = append(out, m.th.bad.Render("✖ job ")+j.Package+" "+truncate(j.Error, 30))
		}
	}
	if len(out) == 0 {
		out = append(out, m.th.good.Render("● Everything is healthy — no action needed"))
	}
	return out
}

func (m *Model) resourceLines() []string {
	s := m.snap.Status
	lines := []string{
		fmt.Sprintf("services   ● %d running   ○ %d installed", runningServices(m.snap), len(s.Services)),
		fmt.Sprintf("sites      %d parked    %d enabled", len(s.Sites), enabledSites(m.snap)),
		fmt.Sprintf("php        %d versions   default %s", len(s.PHP), s.Defaults["php"]),
		fmt.Sprintf("node       %d versions   default %s", len(s.Node), s.Defaults["node"]),
	}
	if len(m.snap.Jobs) > 0 {
		j := m.snap.Jobs[0]
		pct := 0
		if j.Total > 0 {
			pct = int(100 * j.Downloaded / j.Total)
		}
		bar := strings.Repeat("█", max(0, pct/5)) + strings.Repeat("░", 20-max(0, pct/5))
		lines = append(lines, "job        "+truncate(j.Package+" "+j.Version, 18), "           "+bar+fmt.Sprintf(" %d%%", pct))
	}
	return lines
}

func (m *Model) systemLines() []string {
	s := m.snap.Status
	lines := []string{
		fmt.Sprintf("web        %s  http %d  https %d", bindText(s.Web.Bind), s.Web.HTTPPort, s.Web.HTTPSPort),
		fmt.Sprintf("domain     %s   redirect %v", s.Web.Domain, s.Web.RedirectHTTP),
		fmt.Sprintf("dns        %s", boolText(s.DNS.OK)),
		fmt.Sprintf("ca         exists %v  trusted %v/%v", s.CA.Exists, s.CA.SystemTrusted, s.CA.BrowserTrusted),
		fmt.Sprintf("defaults   db %s   mail %d   xdebug %v", s.Defaults["mariadb"], m.snap.Mail.Port, m.snap.Debugger.Detect),
	}
	if len(m.snap.Warnings) > 0 {
		lines = append(lines, m.th.warn.Render("warn       ")+truncate(m.snap.Warnings[0], 40))
	}
	return lines
}

func enabledSites(s Snapshot) int {
	n := 0
	for _, site := range s.Status.Sites {
		if site.Enabled {
			n++
		}
	}
	return n
}

func boolText(v bool) string {
	if v {
		return "ok"
	}
	return "down"
}

func bindText(bind string) string {
	if bind == "" {
		return "127.0.0.1"
	}
	return bind
}

func (m *Model) siteNames() []string {
	out := make([]string, 0, len(m.snap.Status.Sites))
	for _, site := range m.snap.Status.Sites {
		out = append(out, site.Name)
	}
	return out
}
