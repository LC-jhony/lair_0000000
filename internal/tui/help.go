package tui

import (
	"strings"

	"charm.land/lipgloss/v2"
)

var helpRows = [][2]string{
	{"ctrl+p", "command palette"},
	{"↑↓ / j k", "move in the focused list"},
	{"enter", "open / act on the selection"},
	{"tab / esc", "switch sidebar and main area"},
	{"\\", "toggle sidebar on narrow screens"},
	{"/", "filter the current list"},
	{"o", "cycle sort order"},
	{"1–5", "switch page tabs"},
	{"b", "toggle site HTTPS"},
	{"e", "edit domains of the selected site"},
	{"c", "show the generated vhost"},
	{"v / l", "load logs"},
	{"s / x", "restart / stop the selected service"},
	{"d", "set the selected version or database as default"},
	{"i", "install a version or create a project"},
	{"R", "refresh all data"},
	{"?", "show this help"},
	{":", "run a command"},
	{"q / ctrl+c", "quit"},
}

func (m *Model) renderHelpOverlay(w, h int) string {
	lines := []string{m.th.accent.Render(" Help "), ""}
	for _, row := range helpRows {
		lines = append(lines, " "+pad(row[0], 14)+m.th.dim.Render(row[1]))
	}
	lines = append(lines, "", m.th.dim.Render(" esc to close"))
	ow := min(w-4, 66)
	oh := min(h-4, len(lines)+2)
	box := lipgloss.NewStyle().Width(ow).Height(oh).Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("63")).Render(strings.Join(lines, "\n"))
	return lipgloss.Place(w, h, lipgloss.Center, lipgloss.Center, box)
}
