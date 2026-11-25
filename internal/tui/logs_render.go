package tui

import (
	"strings"

	"charm.land/lipgloss/v2"
)

func (m *Model) renderLogs(w, h int) string {
	lines := []string{m.th.title.Render(" Logs "), ""}
	if len(m.snap.LogSources) == 0 {
		lines = append(lines, m.th.dim.Render(" No log sources discovered yet."))
	} else {
		for i, src := range m.snap.LogSources {
			mark := "  "
			if i == m.logSourceIdx {
				mark = m.th.accent.Render("▸ ")
			}
			lines = append(lines, mark+pad(src.Kind+"/"+src.ID, 28)+truncate(strings.Join(src.Files, ", "), w-32))
		}
	}
	lines = append(lines, "", "[ ] change source · l load tail · f filter · { } scroll · R refresh")
	if m.logText != "" {
		lines = append(lines, "", m.th.dim.Render(strings.Repeat("─", min(w, 40))))
		lines = append(lines, truncateLines(m.logText, w))
	}
	return lipgloss.PlaceVertical(max(2, h-1), lipgloss.Top, truncateLines(strings.Join(lines, "\n"), w))
}
