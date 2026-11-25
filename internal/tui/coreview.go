package tui

import (
	"runtime"
	"strings"

	"charm.land/lipgloss/v2"
)

func (m *Model) renderSystem(w, h int) string {
	s := m.snap.Status
	lines := []string{m.th.title.Render(" System "), ""}
	lines = append(lines, " core processes")
	lines = append(lines, "  "+statusDot(true, m.th)+" daemon      local API + dispatcher")
	lines = append(lines, "  "+statusDot(s.Web.Error == "", m.th)+" web         http "+portString(s.Web.HTTPPort)+" https "+portString(s.Web.HTTPSPort))
	lines = append(lines, "  "+statusDot(s.DNS.OK, m.th)+" dns         "+truncate(s.DNS.Detail, w-24))
	lines = append(lines, "  "+statusDot(s.CA.Exists, m.th)+" ca          "+truncate(s.CA.Path, w-24))
	lines = append(lines, "  "+statusDot(m.snap.Mail.Running, m.th)+" mail        "+portString(m.snap.Mail.Port))
	lines = append(lines, "  "+statusDot(m.snap.Debugger.Port > 0, m.th)+" debugger    "+portString(m.snap.Debugger.Port))
	lines = append(lines, "", " notifications & bridge")
	lines = append(lines, "  jobs        "+count(len(m.snap.Jobs)))
	lines = append(lines, "  warnings    "+count(len(m.snap.Warnings)))
	lines = append(lines, "  platform    "+runtime.GOOS+"/"+runtime.GOARCH)
	lines = append(lines, "", m.th.dim.Render(" Y debug window · l logs · R refresh"))
	return lipgloss.PlaceVertical(max(2, h-1), lipgloss.Top, truncateLines(strings.Join(lines, "\n"), w))
}

func portString(p int) string {
	if p == 0 {
		return "—"
	}
	return ":" + itoa(p)
}

func count(n int) string {
	if n == 0 {
		return "none"
	}
	return itoa(n)
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	buf := [20]byte{}
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
