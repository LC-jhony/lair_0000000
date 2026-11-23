package cli

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/lair-sh/lair/internal/app"
	"github.com/lair-sh/lair/internal/daemon"
)

// runDesktop prints the connection details used by the native app. There is no
// browser UI any more. If the daemon is not running yet it is started
// detached; the URL and token are read from 0600 files in the run directory.
func runDesktop(args []string, out, errOut io.Writer) error {
	for _, a := range args {
		if isHelp(a) {
			fmt.Fprintln(out, "usage: lair desktop   Muestra la conexión de la app nativa con el daemon")
			return nil
		}
	}
	paths, err := app.Resolve()
	if err != nil {
		return err
	}
	if err := paths.Ensure(); err != nil {
		return err
	}
	sock, err := paths.SocketPath("api")
	if err != nil {
		return err
	}
	urlPath := filepath.Join(paths.Run, "desktop.url")
	tokenPath := filepath.Join(paths.Run, "desktop.token")

	if !daemon.Available(sock) {
		exe, err := os.Executable()
		if err != nil {
			return err
		}
		logPath := filepath.Join(paths.Log, "daemon-desktop.log")
		log, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
		if err != nil {
			return err
		}
		defer log.Close()

		cmd := exec.Command(exe, "daemon", "--foreground")
		cmd.Stdout = log
		cmd.Stderr = log
		cmd.Dir = paths.Root
		cmd.Env = os.Environ()
		cmd.SysProcAttr = detachAttr()
		if err := cmd.Start(); err != nil {
			return fmt.Errorf("no se pudo iniciar el daemon: %w", err)
		}
		deadline := time.Now().Add(10 * time.Second)
		for time.Now().Before(deadline) && !daemon.Available(sock) {
			time.Sleep(100 * time.Millisecond)
		}
		if !daemon.Available(sock) {
			return fmt.Errorf("el daemon no arrancó; revisa %s", logPath)
		}
	}

	apiURL, err := readTrimmed(urlPath)
	if err != nil || apiURL == "" {
		return fmt.Errorf("el daemon está activo pero no publicó la API de escritorio; reinícialo con `lair daemon --foreground --desktop-port 9797`")
	}
	if _, err := readTrimmed(tokenPath); err != nil {
		return fmt.Errorf("el daemon está activo pero no publicó el token de escritorio: %w", err)
	}

	fmt.Fprintf(out, "Lair desktop API: %s\n", apiURL)
	fmt.Fprintf(out, "Token: %s (0600, lo lee la app automáticamente)\n", tokenPath)
	fmt.Fprintln(out, "Abre la app nativa de Lair; se conectará sola al daemon.")
	_ = errOut
	return nil
}

func readTrimmed(path string) (string, error) {
	b, err := os.ReadFile(path)
	return strings.TrimSpace(string(b)), err
}
