package tui

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
)

func (m *Model) renderRuntimes(w, h int) string {
	s := m.snap.Status
	lines := []string{m.th.title.Render(" PHP & Node "), "", " PHP"}
	for _, info := range s.PHP {
		mark := "  "
		if info.Minor == s.Defaults["php"] {
			mark = m.th.accent.Render("▸ ")
		}
		lines = append(lines, mark+pad(info.Minor, 10)+m.th.dim.Render(info.Version+"  "+info.Dir))
	}
	if len(m.versions["php"]) > 0 {
		lines = append(lines, "  catalog: "+truncate(strings.Join(m.versions["php"][:min(6, len(m.versions["php"]))], " "), w-12))
	}
	lines = append(lines, "", " Node")
	for _, v := range s.Node {
		mark := "  "
		if v == s.Defaults["node"] {
			mark = m.th.accent.Render("▸ ")
		}
		lines = append(lines, mark+v)
	}
	if len(m.versions["node"]) > 0 {
		lines = append(lines, "  catalog: "+truncate(strings.Join(m.versions["node"][:min(6, len(m.versions["node"]))], " "), w-12))
	}
	lines = append(lines, "", fmt.Sprintf(" composer %s · default php %s · node %s", s.Composer, s.Defaults["php"], s.Defaults["node"]))
	lines = append(lines, m.th.dim.Render(" i install version · d set default · x toggle Xdebug · u update composer/npm · R refresh"))
	return lipgloss.PlaceVertical(max(2, h-1), lipgloss.Top, truncateLines(strings.Join(lines, "\n"), w))
}
