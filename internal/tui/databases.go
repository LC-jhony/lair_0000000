package tui

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
)

var dbServices = []string{"sqlite3", "mariadb", "mysql", "postgres", "mongodb", "redis", "valkey"}

func (m *Model) renderDatabases(w, h int) string {
	lines := []string{m.th.title.Render(" Databases "), ""}
	selected := m.selectedService
	if selected == "" {
		selected = m.snap.Status.Defaults["mariadb"]
	}
	for _, id := range dbServices {
		svc, ok := m.snap.Status.Services[id]
		if !ok {
			continue
		}
		mark := "  "
		if id == selected {
			mark = m.th.accent.Render("▸ ")
		}
		state := "○"
		if svc.Running {
			state = m.th.good.Render("●")
		}
		def := ""
		if m.snap.Status.Defaults["mariadb"] == id || m.snap.Status.Defaults[id] != "" {
			def = " default"
		}
		lines = append(lines, mark+pad(id, 12)+state+"  "+strings.Join(svc.Installed, ", ")+def)
	}
	lines = append(lines, "", m.th.dim.Render(" n create database · d set default · R refresh"))
	lines = append(lines, "", " Sites using a database:")
	for _, site := range m.snap.Status.Sites {
		if site.Database != "" {
			data := m.snap.SiteData[site.Name]
			lines = append(lines, "  "+pad(site.Name, 22)+site.Database+fmt.Sprintf(" (%d tables)", len(data.Tables)))
		}
	}
	return lipgloss.PlaceVertical(max(2, h-1), lipgloss.Top, truncateLines(strings.Join(lines, "\n"), w))
}
