package tui

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
)

var serviceTabs = []string{"Overview", "Logs"}

func (m *Model) renderServices(w, h int) string {
	id := m.selectedService
	if id == "" {
		for name := range m.snap.Status.Services {
			id = name
			break
		}
	}
	if id == "" {
		return m.th.dim.Render("No services available on this platform.")
	}
	svc := m.snap.Status.Services[id]
	health := ""
	for _, s := range m.snap.Health {
		if s.ID == id {
			health = boolText(s.Healthy) + " " + s.Detail
		}
	}
	dot := "○"
	if svc.Running {
		dot = "●"
	}
	head := m.th.title.Render(" "+id) + "  " + dot + " " + statusText(svc.Active) +
		"\n " + m.th.dim.Render(fmt.Sprintf("port %d · installed %s · %s", svc.Port, strings.Join(svc.Installed, ", "), health))
	tabLine := m.tabs(w, serviceTabs, m.tab)
	if m.tab == 1 {
		return head + "\n" + tabLine + "\n" + truncateLines(m.logText, w)
	}
	body := []string{
		" id          " + id,
		" active      " + svc.Active,
		" port        " + fmt.Sprint(svc.Port),
		" running     " + boolText(svc.Running),
		" available   " + boolText(svc.Available),
		" platform    " + svc.Platform,
		" default     " + m.snap.Status.Defaults[id],
	}
	users := []string{}
	for _, site := range m.snap.Status.Sites {
		if site.Database == id {
			users = append(users, site.Name)
		}
	}
	body = append(body, " sites       "+strings.Join(users, ", "))
	if svc.LastError != "" {
		body = append(body, m.th.bad.Render(" last error  "+truncate(svc.LastError, w-14)))
	}
	if svc.UnavailableReason != "" {
		body = append(body, m.th.warn.Render(" note        "+truncate(svc.UnavailableReason, w-14)))
	}
	body = append(body, "", " keys: s restart · x stop · d default · i install · l logs · R refresh")
	return head + "\n" + tabLine + "\n" + lipgloss.PlaceVertical(max(2, h-4), lipgloss.Top, truncateLines(strings.Join(body, "\n"), w))
}
