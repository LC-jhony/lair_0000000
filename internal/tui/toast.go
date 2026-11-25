package tui

import (
	"fmt"
	"strings"
	"time"

	"charm.land/lipgloss/v2"
)

// Toast is a transient action result. Toasts are drawn over the content and
// never change the layout.
type Toast struct {
	Text    string
	Detail  string
	Err     bool
	Created time.Time
}

func (t Toast) key() string {
	return fmt.Sprintf("%t:%s:%s", t.Err, t.Text, t.Detail)
}

func (m *Model) notify(text, detail string, isErr bool) {
	if text == "" {
		return
	}
	now := time.Now()
	for i := range m.toasts {
		if m.toasts[i].key() == (Toast{Text: text, Detail: detail, Err: isErr}).key() {
			m.toasts[i].Created = now
			return
		}
	}
	m.toasts = append(m.toasts, Toast{Text: text, Detail: detail, Err: isErr, Created: now})
	if len(m.toasts) > 3 {
		m.toasts = m.toasts[len(m.toasts)-3:]
	}
}

func (m *Model) pruneToasts(now time.Time) {
	kept := m.toasts[:0]
	for _, t := range m.toasts {
		if now.Sub(t.Created) < 30*time.Second {
			kept = append(kept, t)
		}
	}
	m.toasts = kept
}

func (m *Model) renderToasts(w, h int) string {
	if len(m.toasts) == 0 || w < 30 || h < 6 {
		return ""
	}
	lines := make([]string, 0, len(m.toasts))
	for _, t := range m.toasts {
		style := m.th.good
		mark := "✓"
		if t.Err {
			style, mark = m.th.bad, "✖"
		}
		text := pad(mark+" "+t.Text, min(46, w-4))
		line := style.Render(text)
		if t.Detail != "" {
			line += " " + m.th.dim.Render(truncate(t.Detail, 24))
		}
		lines = append(lines, line)
	}
	box := strings.Join(lines, "\n")
	return lipgloss.Place(w, len(lines), lipgloss.Right, lipgloss.Bottom, box)
}

func truncate(s string, n int) string {
	if len([]rune(s)) <= n {
		return s
	}
	return string([]rune(s)[:n-1]) + "…"
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
