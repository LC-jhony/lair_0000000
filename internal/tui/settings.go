package tui

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
)

type setting struct {
	key    string
	label  string
	on     bool
	action string
}

func (m *Model) settings() []setting {
	s := m.snap.Status
	return []setting{
		{key: "b", label: "bind to LAN (0.0.0.0)", on: s.Web.Bind == "0.0.0.0", action: "web-bind"},
		{key: "t", label: "HTTPS for *.test", on: s.Web.HTTPS, action: "web-https"},
		{key: "r", label: "redirect HTTP to HTTPS", on: s.Web.RedirectHTTP, action: "web-redirect"},
		{key: "m", label: "SMTP catcher", on: m.snap.Mail.Running, action: "mail-toggle"},
		{key: "d", label: "dump capture", on: m.snap.Dumps.Capturing, action: "dumps-toggle"},
		{key: "x", label: "Xdebug detection", on: m.snap.Debugger.Detect, action: "debugger-detect"},
		{key: "p", label: "Xdebug breakpoints", on: m.snap.Debugger.Breakpoints, action: "debugger-breakpoints"},
	}
}

func (m *Model) renderSettings(w, h int) string {
	lines := []string{m.th.title.Render(" Settings "), ""}
	for i, s := range m.settings() {
		mark := "  "
		if m.settingIdx == i {
			mark = m.th.accent.Render("▸ ")
		}
		state := m.th.idle.Render("○ off")
		if s.on {
			state = m.th.good.Render("● on ")
		}
		lines = append(lines, mark+pad(s.key, 4)+state+"  "+s.label)
	}
	web := m.snap.Status.Web
	lines = append(lines, "", fmt.Sprintf(" http %d  https %d  domain %s", web.HTTPPort, web.HTTPSPort, web.Domain))
	lines = append(lines, m.th.dim.Render(" space toggle · [ ] change ports/domain · R refresh"))
	return lipgloss.PlaceVertical(max(2, h-1), lipgloss.Top, truncateLines(strings.Join(lines, "\n"), w))
}
