package tui

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/mattn/go-isatty"

	"github.com/lair-sh/lair/internal/daemon"
)

// Run starts the full-screen dashboard. The caller is supplied by the CLI so
// the TUI talks to the daemon (or the in-process dispatcher) exactly like the
// rest of the application.
func Run(args []string, caller daemon.Caller, out, errOut io.Writer) error {
	if len(args) > 0 && isHelpArg(args[0]) {
		fmt.Fprintln(out, strings.TrimSpace(helpText))
		return nil
	}
	if !isatty.IsTerminal(os.Stdin.Fd()) && !isatty.IsCygwinTerminal(os.Stdin.Fd()) {
		return fmt.Errorf("lair tui requiere una terminal TTY real; ejecútalo desde una shell interactiva")
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	m := newModel(ctx, cancel, caller)
	p := tea.NewProgram(m, tea.WithContext(ctx), tea.WithInput(os.Stdin), tea.WithOutput(out))
	m.send = p.Send
	_, err := p.Run()
	if err != nil {
		return err
	}
	fmt.Fprintln(out)
	return nil
}

func isHelpArg(s string) bool {
	return s == "--help" || s == "-h" || s == "help"
}

const helpText = `
lair tui — dashboard interactivo

  lair tui             abre el dashboard a pantalla completa
  lair tui --help      muestra esta ayuda

Teclas: ctrl+p paleta · ↑↓ navegar · enter abrir · tab cambiar foco
        / filtrar · ? ayuda · R refrescar · q salir

Todas las acciones se ejecutan a través del mismo daemon que usa la CLI y
la app de escritorio. Las acciones destructivas no están disponibles aquí.
`
