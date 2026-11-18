package cli

import (
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/lair-sh/lair/internal/daemon"
	"github.com/lair-sh/lair/internal/engine"
	"github.com/lair-sh/lair/internal/installer"
	"github.com/lair-sh/lair/internal/php"
)

const tuiHelp = `lair tui (alias: lair dashboard) - panel de control interactivo

Uso:
  lair tui
  lair dashboard

Teclas:
  tab/→/←       cambiar de sección
  n             instalar el último LTS de Node
  p             instalar la última versión de PHP
  m             instalar MariaDB (base de datos por defecto)
  u             actualizar el catálogo firmado
  r             actualizar el estado
  s             reiniciar los servicios
  q / ctrl+c    salir
`

// runTUI starts the interactive Charmbracelet dashboard. When stdin is not a
// terminal (CI, pipes, tests) it prints a one-shot textual dashboard instead of
// leaving a stuck full-screen program behind.
func runTUI(args []string, out, errOut io.Writer) error {
	for _, a := range args {
		if isHelp(a) {
			fmt.Fprint(out, tuiHelp)
			return nil
		}
	}
	c, err := newCtx(out, errOut)
	if err != nil {
		return err
	}
	if !isTerminal(os.Stdin) {
		return printDashboard(c, out)
	}

	m := newTUIModel(c)
	p := tea.NewProgram(m, tea.WithInput(os.Stdin), tea.WithOutput(out), tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		// Some hosts advertise a TTY but cannot provide a cancel reader (CI,
		// nested terminals). Never leave the caller stuck or without data.
		return printDashboard(c, out)
	}
	return nil
}

// printDashboard is the non-interactive fallback: the same data, plain text.
func printDashboard(c *ctx, out io.Writer) error {
	var st engine.Status
	if err := c.call(daemon.ActionStatus, nil, &st); err != nil {
		return err
	}
	fmt.Fprintf(out, "Lair · %s\n", st.Home)
	fmt.Fprintf(out, "PHP:     %s (default: %s)\n", joinMinors(st.PHP), st.Defaults["php"])
	fmt.Fprintf(out, "Node:    %s (default: %s)\n", strings.Join(st.Node, ", "), st.Defaults["node"])
	defaultDB := st.Defaults["database"]
	if defaultDB == "" {
		defaultDB = engine.DefaultDatabaseID
	}
	fmt.Fprintf(out, "BD:      %s\n", defaultDB)
	ids := make([]string, 0, len(st.Services))
	for id := range st.Services {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		v := st.Services[id]
		state := "stopped"
		if v.Running {
			state = "running"
		}
		fmt.Fprintf(out, "  %-12s %-10s port=%-6d %s\n", id, orDash(v.Active), v.Port, state)
	}
	fmt.Fprintf(out, "Sitios:  %d\n", len(st.Sites))
	if len(st.Jobs) > 0 {
		fmt.Fprintf(out, "Trabajos:\n")
		for _, j := range st.Jobs {
			fmt.Fprintf(out, "  %s: %s\n", j.ID, j.Phase)
		}
	}
	return nil
}

func joinMinors(list []php.Info) string {
	parts := make([]string, 0, len(list))
	for _, p := range list {
		parts = append(parts, p.Minor)
	}
	if len(parts) == 0 {
		return "(ninguna)"
	}
	return strings.Join(parts, ", ")
}

var (
	styleTitle   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("205"))
	styleSubtle  = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
	styleOK      = lipgloss.NewStyle().Foreground(lipgloss.Color("42"))
	styleWarn    = lipgloss.NewStyle().Foreground(lipgloss.Color("214"))
	styleErr     = lipgloss.NewStyle().Foreground(lipgloss.Color("203"))
	styleTab     = lipgloss.NewStyle().Padding(0, 2).Foreground(lipgloss.Color("252"))
	styleTabOn   = lipgloss.NewStyle().Padding(0, 2).Bold(true).Foreground(lipgloss.Color("205")).Background(lipgloss.Color("235"))
	stylePanel   = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("238")).Padding(0, 1)
	stylePanelOn = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("205")).Padding(0, 1)
	styleKey     = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("111"))
)

type statusMsg struct {
	st  engine.Status
	err error
}

type actionMsg struct {
	label string
	err   error
}

type tuiModel struct {
	c         *ctx
	tabs      []string
	tab       int
	status    engine.Status
	statusErr error
	loaded    bool
	spinner   spinner.Model
	busy      string
	message   string
	width     int
	height    int
}

func newTUIModel(c *ctx) tuiModel {
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = styleOK
	m := tuiModel{
		c:       c,
		tabs:    []string{"Resumen", "Runtimes", "Bases de datos", "Servicios", "Sitios", "Sistema"},
		spinner: s,
		message: "Listo",
	}
	m.status = engine.Status{Services: map[string]engine.ServiceView{}}
	return m
}

func (m tuiModel) Init() tea.Cmd {
	return tea.Batch(m.refresh(), m.spinner.Tick)
}

func (m tuiModel) refresh() tea.Cmd {
	return func() tea.Msg {
		var st engine.Status
		err := m.c.call(daemon.ActionStatus, nil, &st)
		return statusMsg{st: st, err: err}
	}
}

func (m tuiModel) run(label, action string, payload any) tea.Cmd {
	return func() tea.Msg {
		var raw installer.Status
		if err := m.c.call(action, payload, &raw); err != nil {
			return actionMsg{label: label, err: err}
		}
		if raw.ID != "" && !raw.Done {
			for {
				var st installer.Status
				if err := m.c.call(daemon.ActionJob, daemon.JobRequest{ID: raw.ID}, &st); err != nil {
					return actionMsg{label: label, err: err}
				}
				if st.Done {
					if st.Error != "" {
						return actionMsg{label: label, err: fmt.Errorf("%s", st.Error)}
					}
					break
				}
				time.Sleep(150 * time.Millisecond)
			}
		}
		return actionMsg{label: label}
	}
}

func (m tuiModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c", "esc":
			return m, tea.Quit
		case "tab", "right", "l":
			m.tab = (m.tab + 1) % len(m.tabs)
		case "left", "h":
			m.tab = (m.tab + len(m.tabs) - 1) % len(m.tabs)
		case "r":
			return m, m.refresh()
		case "n":
			m.busy = "Instalando Node LTS…"
			return m, tea.Batch(m.spinner.Tick, m.run("Node LTS", daemon.ActionInstallNode, daemon.InstallRequest{}))
		case "p":
			m.busy = "Instalando PHP…"
			return m, tea.Batch(m.spinner.Tick, m.run("PHP", daemon.ActionInstallPHP, daemon.InstallRequest{}))
		case "m":
			m.busy = "Instalando MariaDB…"
			return m, tea.Batch(m.spinner.Tick, m.run("MariaDB", daemon.ActionEnsureDatabase, daemon.InstallRequest{}))
		case "u":
			m.busy = "Actualizando catálogo…"
			return m, tea.Batch(m.spinner.Tick, m.run("catálogo", daemon.ActionUpdate, nil))
		case "s":
			m.busy = "Reiniciando servicios…"
			return m, tea.Batch(m.spinner.Tick, m.run("servicios", daemon.ActionRestart, nil))
		}
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
	case statusMsg:
		m.loaded = true
		m.statusErr = msg.err
		if msg.err == nil {
			m.status = msg.st
		}
	case actionMsg:
		m.busy = ""
		if msg.err != nil {
			m.message = styleErr.Render(msg.label + ": " + msg.err.Error())
		} else {
			m.message = styleOK.Render(msg.label + ": listo")
		}
		return m, m.refresh()
	default:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd
	}
	return m, nil
}

func (m tuiModel) View() string {
	var b strings.Builder
	b.WriteString(styleTitle.Render("◆ LAIR") + "  " + styleSubtle.Render("entorno nativo PHP/Laravel") + "\n")
	if m.width > 0 {
		b.WriteString(styleSubtle.Render(strings.Repeat("─", m.width)) + "\n")
	}
	tabs := make([]string, len(m.tabs))
	for i, name := range m.tabs {
		if i == m.tab {
			tabs[i] = styleTabOn.Render(name)
		} else {
			tabs[i] = styleTab.Render(name)
		}
	}
	b.WriteString(lipgloss.JoinHorizontal(lipgloss.Top, tabs...) + "\n\n")

	body := m.content()
	maxWidth := m.width - 2
	if maxWidth > 20 {
		body = lipgloss.NewStyle().Width(maxWidth).Render(body)
	}
	b.WriteString(body + "\n\n")

	status := m.message
	if m.busy != "" {
		status = m.spinner.View() + " " + m.busy
	} else if m.statusErr != nil {
		status = styleErr.Render("sin conexión: " + m.statusErr.Error())
	}
	keys := styleKey.Render("tab") + " sección  " + styleKey.Render("n") + " Node LTS  " +
		styleKey.Render("p") + " PHP  " + styleKey.Render("m") + " MariaDB  " +
		styleKey.Render("u") + " catálogo  " + styleKey.Render("s") + " servicios  " +
		styleKey.Render("r") + " refrescar  " + styleKey.Render("q") + " salir"
	b.WriteString(styleSubtle.Render(strings.Repeat("─", maxWidthOr(m.width, 40))) + "\n")
	b.WriteString(status + "\n" + keys + "\n")
	return b.String()
}

func maxWidthOr(v, fallback int) int {
	if v > 4 {
		return v
	}
	return fallback
}

func (m tuiModel) content() string {
	if !m.loaded {
		return styleSubtle.Render("Cargando estado…")
	}
	switch m.tabs[m.tab] {
	case "Runtimes":
		return m.viewRuntimes()
	case "Bases de datos":
		return m.viewDatabases()
	case "Servicios":
		return m.viewServices(false)
	case "Sitios":
		return m.viewSites()
	case "Sistema":
		return m.viewSystem()
	default:
		return m.viewOverview()
	}
}

func (m tuiModel) viewOverview() string {
	s := m.status
	services := s.Services
	running := 0
	for _, v := range services {
		if v.Running {
			running++
		}
	}
	left := stylePanel.Render(strings.Join([]string{
		styleTitle.Render("Versiones"),
		fmt.Sprintf("PHP    %s  (default %s)", joinMinors(s.PHP), orDash(s.Defaults["php"])),
		fmt.Sprintf("Node   %s  (default %s)", strings.Join(s.Node, ", "), orDash(s.Defaults["node"])),
		fmt.Sprintf("BD     %s", orDash(s.Defaults["database"])),
	}, "\n"))
	right := stylePanel.Render(strings.Join([]string{
		styleTitle.Render("Entorno"),
		fmt.Sprintf("Home      %s", s.Home),
		fmt.Sprintf("Sitios    %d", len(s.Sites)),
		fmt.Sprintf("Servicios %d/%d activos", running, len(services)),
		fmt.Sprintf("Trabajos  %d", len(s.Jobs)),
	}, "\n"))
	return lipgloss.JoinHorizontal(lipgloss.Top, left, "  ", right)
}

func (m tuiModel) viewRuntimes() string {
	s := m.status
	phpList := joinMinors(s.PHP)
	nodeList := "(ninguna)"
	if len(s.Node) > 0 {
		nodeList = strings.Join(s.Node, ", ")
	}
	return stylePanel.Render(strings.Join([]string{
		styleTitle.Render("Runtimes instalados"),
		"",
		fmt.Sprintf("  PHP   %s", phpList),
		fmt.Sprintf("  Node  %s", nodeList),
		"",
		styleSubtle.Render("n = instalar último LTS de Node  ·  p = instalar PHP más reciente"),
	}, "\n"))
}

func (m tuiModel) viewDatabases() string {
	s := m.status
	ids := []string{"mariadb", "mysql", "postgres"}
	lines := []string{styleTitle.Render("Bases de datos"), ""}
	for _, id := range ids {
		v, ok := s.Services[id]
		if !ok {
			continue
		}
		mark := "  "
		if s.Defaults["database"] == id {
			mark = styleOK.Render("* ")
		}
		state := styleSubtle.Render("detenida")
		if v.Running {
			state = styleOK.Render("activa")
		}
		lines = append(lines, fmt.Sprintf("%s%-10s %-28s %s", mark, id,
			"activa="+orDash(v.Active)+" puerto="+fmt.Sprint(v.Port), state))
	}
	lines = append(lines, "", styleSubtle.Render("m = instalar MariaDB por defecto  ·  usa el panel web para crear bases de datos"))
	return stylePanel.Render(strings.Join(lines, "\n"))
}

func (m tuiModel) viewServices(all bool) string {
	s := m.status
	ids := make([]string, 0, len(s.Services))
	for id := range s.Services {
		if !all && id != "mariadb" && id != "mysql" && id != "postgres" {
			continue
		}
		ids = append(ids, id)
	}
	sort.Strings(ids)
	lines := []string{styleTitle.Render("Servicios"), ""}
	for _, id := range ids {
		v := s.Services[id]
		state := styleSubtle.Render("detenido")
		if v.Running {
			state = styleOK.Render("ejecutando")
		} else if v.LastError != "" {
			state = styleWarn.Render(v.LastError)
		}
		lines = append(lines, fmt.Sprintf("  %-12s %s", id, state))
	}
	return stylePanel.Render(strings.Join(lines, "\n"))
}

func (m tuiModel) viewSites() string {
	s := m.status
	lines := []string{styleTitle.Render("Sitios aparcados"), ""}
	if len(s.Sites) == 0 {
		lines = append(lines, styleSubtle.Render("  (ninguno)"))
	}
	for _, site := range s.Sites {
		lines = append(lines, fmt.Sprintf("  https://%s -> %s", site.Name, site.Root))
	}
	return stylePanel.Render(strings.Join(lines, "\n"))
}

func (m tuiModel) viewSystem() string {
	return stylePanel.Render(strings.Join([]string{
		styleTitle.Render("Sistema"),
		"",
		"  " + styleSubtle.Render("Catálogo firmado con Ed25519; actualízalo con 'u'."),
		"  " + styleSubtle.Render("El panel web ofrece el resto de acciones: lair ui"),
	}, "\n"))
}

func orDash(v string) string {
	if v == "" {
		return "—"
	}
	return v
}
