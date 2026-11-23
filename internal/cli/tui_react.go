package cli

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/lair-sh/lair/internal/app"
	"github.com/lair-sh/lair/internal/assets"
	"github.com/lair-sh/lair/internal/nvm"
	"github.com/lair-sh/lair/internal/version"
)

// reactTUIBundle writes the embedded React/Ink dashboard into the Lair cache
// and returns its path. The hash in the directory name makes upgrades atomic:
// an old bundle is never overwritten while a process is still reading it.
func reactTUIBundle(paths app.Paths) (string, error) {
	sum := sha256.Sum256(assets.TUIBundle)
	dir := filepath.Join(paths.Cache, "tui", hex.EncodeToString(sum[:])[:16])
	target := filepath.Join(dir, "lair-tui.mjs")
	if data, err := os.ReadFile(target); err == nil && sha256.Sum256(data) == sum {
		return target, nil
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	tmp, err := os.CreateTemp(dir, ".tui-*")
	if err != nil {
		return "", err
	}
	name := tmp.Name()
	defer os.Remove(name)
	if _, err := tmp.Write(assets.TUIBundle); err != nil {
		tmp.Close()
		return "", err
	}
	if err := tmp.Close(); err != nil {
		return "", err
	}
	if err := os.Rename(name, target); err != nil {
		return "", err
	}
	return target, nil
}

// reactTUINode finds a Node 22+ binary: an explicit override, the PATH, then
// the versions installed through Lair's nvm integration.
func reactTUINode() string {
	if explicit := strings.TrimSpace(os.Getenv("LAIR_NODE")); explicit != "" {
		return explicit
	}
	if found, err := exec.LookPath("node"); err == nil {
		return found
	}
	name := "node"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	for _, installed := range nvm.InstalledVersions() {
		parsed, err := version.Parse(strings.TrimPrefix(installed, "v"))
		if err != nil || parsed.Major < 22 {
			continue
		}
		if dir, ok := nvm.Installed(installed); ok {
			bin := filepath.Join(dir, "bin", name)
			if st, err := os.Stat(bin); err == nil && !st.IsDir() {
				return bin
			}
		}
	}
	return ""
}

// runReactTUI launches the termcn dashboard. It returns an error so runTUI can
// fall back to the Go dashboard on machines without a suitable Node runtime.
func runReactTUI(out, errOut io.Writer) error {
	node := reactTUINode()
	if node == "" {
		return fmt.Errorf("Node 22+ no disponible; instalándolo con `lair node install 22`")
	}
	paths, err := app.Resolve()
	if err != nil {
		return err
	}
	bundle, err := reactTUIBundle(paths)
	if err != nil {
		return err
	}
	var cmd *exec.Cmd
	if isTerminal(os.Stdin) {
		cmd = exec.Command(node, bundle)
	} else if script, serr := exec.LookPath("script"); serr == nil {
		// Ink needs a real TTY for raw mode. script(1) gives it one while
		// forwarding our stdin/stdout, so the dashboard works in IDE shells.
		inner := shellQuote(node) + " " + shellQuote(bundle)
		cmd = exec.Command(script, "-q", "-c", inner, "/dev/null")
	} else {
		return fmt.Errorf("stdin no es una terminal y no hay script(1)")
	}
	cmd.Stdin = os.Stdin
	cmd.Stdout = out
	cmd.Stderr = errOut
	cmd.Env = append(os.Environ(), "LAIR_HOME="+paths.Root)
	return cmd.Run()
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", `'\''`) + "'"
}
