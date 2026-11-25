// Package tui implements the full-screen terminal dashboard for lair.
// Every action goes through the same daemon.Caller used by the CLI and the
// desktop app: the TUI never owns domain state or reads config.json directly.
package tui

import (
	"image/color"

	"charm.land/lipgloss/v2"
)

// theme holds the styles used by every view. Colors are derived from the
// terminal's ANSI palette, so no user configuration is needed and both light
// and dark backgrounds work.
type theme struct {
	fg, dim, muted                   lipgloss.Style
	title, accent                    lipgloss.Style
	good, warn, bad, idle            lipgloss.Style
	sidebar, content, card, selected lipgloss.Style
	hint, overlay, tab, tabActive    lipgloss.Style
	input                            lipgloss.Style
}

func pick(dark bool, lightColor, darkColor string) lipgloss.Style {
	c := lipgloss.LightDark(dark)(lipgloss.Color(lightColor), lipgloss.Color(darkColor))
	return lipgloss.NewStyle().Foreground(c)
}

func newTheme(dark bool) theme {
	t := theme{}
	t.fg = lipgloss.NewStyle()
	t.dim = pick(dark, "243", "244")
	t.muted = pick(dark, "240", "245")
	t.title = pick(dark, "17", "15").Bold(true)
	t.accent = pick(dark, "21", "81").Bold(true)
	t.good = pick(dark, "22", "42")
	t.warn = pick(dark, "130", "214")
	t.bad = pick(dark, "124", "203")
	t.idle = pick(dark, "244", "240")
	t.sidebar = lipgloss.NewStyle().Background(pickColor(dark, "252", "235"))
	t.content = lipgloss.NewStyle().Background(pickColor(dark, "255", "233"))
	t.card = lipgloss.NewStyle().Background(pickColor(dark, "254", "236"))
	t.selected = lipgloss.NewStyle().Background(pickColor(dark, "20", "238")).Foreground(pickColor(dark, "17", "255")).Bold(true)
	t.hint = pick(dark, "244", "245")
	t.overlay = lipgloss.NewStyle().Background(pickColor(dark, "252", "234")).Foreground(pickColor(dark, "16", "252"))
	t.tab = pick(dark, "240", "245")
	t.tabActive = pick(dark, "21", "81").Bold(true).Underline(true)
	t.input = lipgloss.NewStyle().Background(pickColor(dark, "255", "234")).Foreground(pickColor(dark, "16", "252"))
	return t
}

func pickColor(dark bool, lightColor, darkColor string) color.Color {
	return lipgloss.LightDark(dark)(lipgloss.Color(lightColor), lipgloss.Color(darkColor))
}

// pad truncates or pads s to exactly n cells (ANSI-aware width).
func pad(s string, n int) string {
	w := lipgloss.Width(s)
	if w > n {
		if n <= 1 {
			return sliceCells(s, n)
		}
		return sliceCells(s, n-1) + "…"
	}
	return s + spaces(n-w)
}

func spaces(n int) string {
	if n <= 0 {
		return ""
	}
	b := make([]byte, n)
	for i := range b {
		b[i] = ' '
	}
	return string(b)
}

func sliceCells(s string, n int) string {
	if n <= 0 {
		return ""
	}
	// Inputs here are plain (unstyled) labels: byte slicing is safe for ASCII,
	// and rune slicing keeps UTF-8 intact otherwise.
	r := []rune(s)
	if len(r) > n {
		r = r[:n]
	}
	return string(r)
}
