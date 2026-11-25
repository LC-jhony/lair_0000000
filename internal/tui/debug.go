package tui

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
)

var debugLenses = []string{"Dumps", "Queries", "Views", "Jobs", "Requests", "Mail", "Logs", "Debugger"}

func (m *Model) renderDebug(w, h int, pageKind page) string {
	title := " Debug window "
	if pageKind == pageMail {
		title = " Mail "
	} else if pageKind == pageDumps {
		title = " Dumps "
	} else if pageKind == pageDebugger {
		title = " Debugger "
	}
	lines := []string{m.th.title.Render(title)}
	if pageKind == pageSystem || pageKind == pageDebugger {
		lines = append(lines, m.tabs(w, debugLenses, m.lens))
	}
	switch {
	case pageKind == pageMail || (pageKind == pageSystem && debugLenses[m.lens] == "Mail"):
		r := m.snap.Mail
		lines = append(lines, boolText(r.Running)+" · port "+itoa(r.Port)+" · "+count(r.Count)+" messages")
		for _, msg := range r.Messages {
			lines = append(lines, "  "+pad(truncate(msg.ID, 12), 14)+truncate(msg.From, 24)+" → "+truncate(msg.Subject, w-46))
		}
	case pageKind == pageDumps || (pageKind == pageSystem && debugLenses[m.lens] == "Dumps"):
		r := m.snap.Dumps
		lines = append(lines, "capture "+boolText(r.Capturing)+" · "+count(r.Count)+" events")
		lines = append(lines, fmt.Sprintf(" types: queries=%v logs=%v views=%v jobs=%v requests=%v",
			r.Options.Queries, r.Options.Logs, r.Options.Views, r.Options.Jobs, r.Options.Requests))
		for _, ev := range r.Events {
			lines = append(lines, "  "+pad(truncate(ev.Time, 16), 18)+truncate(ev.Type, 12)+" "+truncate(ev.Message, w-34))
		}
	case pageKind == pageDebugger || (pageKind == pageSystem && debugLenses[m.lens] == "Debugger"):
		r := m.snap.Debugger
		lines = append(lines, " detect "+boolText(r.Detect)+" · breakpoints "+boolText(r.Breakpoints)+" · port "+itoa(r.Port)+" · mode "+r.Mode)
		for _, v := range r.Versions {
			lines = append(lines, "  php "+v.Version+" "+boolText(v.Available))
		}
	case debugLenses[m.lens] == "Jobs":
		for _, j := range m.snap.Jobs {
			lines = append(lines, "  "+pad(truncate(j.Package, 18), 20)+j.Phase+" "+truncate(j.Version, 12))
		}
	default:
		lines = append(lines, m.th.dim.Render(" lens "+debugLenses[m.lens]+" — press [ ] to switch, R to refresh"))
	}
	lines = append(lines, "", m.th.dim.Render(" [ ] lens · / filter · c clear (dumps only) · T bridge · R refresh"))
	return lipgloss.PlaceVertical(max(2, h-1), lipgloss.Top, truncateLines(strings.Join(lines, "\n"), w))
}

func (m *Model) renderRecent(w int) string {
	lines := []string{" Recent"}
	for _, j := range m.snap.Jobs {
		lines = append(lines, "  "+fmt.Sprintf("%-14s", truncate(j.Package, 14))+j.Phase)
	}
	return truncateLines(strings.Join(lines, "\n"), w)
}
