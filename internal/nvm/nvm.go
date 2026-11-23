// Package nvm is the single bridge between Lair and nvm. Node operations never
// call a bare `nvm` binary: the daemon invokes the lair executable with
// `lair nvm ...`, and that command sources nvm.sh in a bash subprocess.
package nvm

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/lair-sh/lair/internal/app"
	"github.com/lair-sh/lair/internal/version"
)

var (
	safeArgToken    = regexp.MustCompile(`^[A-Za-z0-9._/*+=@:-]+$`)
	installedName   = regexp.MustCompile(`^v\d+\.\d+\.\d+(?:-[0-9A-Za-z.-]+)?$`)
	versionInOutput = regexp.MustCompile(`v\d+\.\d+\.\d+(?:-[0-9A-Za-z.-]+)?`)
)

// Script returns the nvm directory and the path to nvm.sh. NVM_DIR wins so the
// user's own installation is always preferred.
func Script() (dir, script string, err error) {
	dirs := []string{}
	if d := strings.TrimSpace(os.Getenv("NVM_DIR")); d != "" {
		dirs = append(dirs, d)
	}
	if home, e := os.UserHomeDir(); e == nil {
		dirs = append(dirs, filepath.Join(home, ".nvm"))
	}
	for _, d := range dirs {
		path := filepath.Join(d, "nvm.sh")
		if info, e := os.Stat(path); e == nil && !info.IsDir() {
			return d, path, nil
		}
	}
	return "", "", fmt.Errorf("nvm is not installed (no nvm.sh found in NVM_DIR or ~/.nvm); install it from https://github.com/nvm-sh/nvm")
}

// Available reports whether nvm can be used on this machine.
func Available() bool {
	_, _, err := Script()
	return err == nil
}

// ValidateArgs rejects anything that is not a plain nvm argument. Defense in
// depth: Exec passes argv to bash, but user-supplied version strings must never
// contain shell metacharacters or whitespace.
func ValidateArgs(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("missing nvm subcommand")
	}
	for _, arg := range args {
		if !safeArgToken.MatchString(arg) {
			return fmt.Errorf("invalid nvm argument %q", arg)
		}
	}
	return nil
}

// Exec runs `nvm <args...>` in a bash subprocess with nvm.sh sourced. The
// arguments are passed as positional parameters, so nothing is interpolated
// into the shell source. This is used by the `lair nvm` CLI command.
func Exec(ctx context.Context, args ...string) (string, error) {
	if err := ValidateArgs(args); err != nil {
		return "", err
	}
	dir, script, err := Script()
	if err != nil {
		return "", err
	}
	const src = `export NVM_DIR="$1"; . "$2"; nvm "${@:3}"`
	cmd := exec.CommandContext(ctx, "bash", "-c", src, "lair-nvm", dir, script)
	cmd.Args = append(cmd.Args, args...)
	cmd.Env = execEnvironment(args, dir)
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// execEnvironment keeps `lair nvm exec` from recursively calling the Lair
// shims. The shims live in LAIR_HOME/bin and invoke `lair exec <tool>`, which
// resolves back to `lair nvm exec <version> <tool>`; without removing that
// directory from PATH, tools like npm would call themselves forever.
func execEnvironment(args []string, nvmDir string) []string {
	env := append([]string(nil), os.Environ()...)
	path := envValue(env, "PATH")

	if paths, err := app.Resolve(); err == nil {
		path = removePathEntry(path, paths.Bin)
	}
	if len(args) >= 3 && args[0] == "exec" {
		versionArg := args[1]
		if versionArg != "current" && !strings.HasPrefix(versionArg, "--") {
			if installed, ok := Installed(versionArg); ok {
				path = prependPath(path, filepath.Join(installed, "bin"))
			}
		}
	}
	env = setEnvValue(env, "PATH", path)
	return append(env, "NVM_DIR="+nvmDir)
}

func envValue(env []string, key string) string {
	for _, item := range env {
		if strings.HasPrefix(item, key+"=") {
			return strings.TrimPrefix(item, key+"=")
		}
	}
	return ""
}

func setEnvValue(env []string, key, value string) []string {
	for i, item := range env {
		if strings.HasPrefix(item, key+"=") {
			env[i] = key + "=" + value
			return env
		}
	}
	return append(env, key+"="+value)
}

func removePathEntry(path, victim string) string {
	if victim == "" {
		return path
	}
	parts := strings.Split(path, string(os.PathListSeparator))
	out := parts[:0]
	for _, part := range parts {
		if part != victim {
			out = append(out, part)
		}
	}
	return strings.Join(out, string(os.PathListSeparator))
}

func prependPath(path, dir string) string {
	if path == "" {
		return dir
	}
	return dir + string(os.PathListSeparator) + path
}

// Command runs a nvm operation through the lair executable itself. This is the
// path used by the daemon: every command goes through lair, never straight to
// bash/nvm. Tests replace this variable with a fake.
var Command = func(ctx context.Context, args ...string) (string, error) {
	bin, err := os.Executable()
	if err != nil {
		return "", err
	}
	return RunBinary(ctx, bin, args...)
}

// RunBinary invokes <bin> nvm <args...> and returns its combined output.
func RunBinary(ctx context.Context, bin string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, bin, append([]string{"nvm"}, args...)...)
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// Installed returns the absolute directory of an installed nvm version.
func Installed(ver string) (string, bool) {
	if !installedName.MatchString(ver) {
		return "", false
	}
	dir, _, err := Script()
	if err != nil {
		return "", false
	}
	path := filepath.Join(dir, "versions", "node", ver)
	if st, err := os.Stat(filepath.Join(path, "bin", "node")); err == nil && !st.IsDir() {
		return path, true
	}
	return "", false
}

// InstalledVersions lists the Node versions managed by nvm, newest first.
func InstalledVersions() []string {
	dir, _, err := Script()
	if err != nil {
		return nil
	}
	entries, err := os.ReadDir(filepath.Join(dir, "versions", "node"))
	if err != nil {
		return nil
	}
	out := []string{}
	for _, entry := range entries {
		if !entry.IsDir() || !installedName.MatchString(entry.Name()) {
			continue
		}
		if st, err := os.Stat(filepath.Join(dir, "versions", "node", entry.Name(), "bin", "node")); err == nil && !st.IsDir() {
			out = append(out, entry.Name())
		}
	}
	version.SortNewest(out)
	return out
}

// NewInstalled returns the version that appeared between two listings.
func NewInstalled(before, after []string) string {
	seen := make(map[string]bool, len(before))
	for _, v := range before {
		seen[v] = true
	}
	for _, v := range after {
		if !seen[v] {
			return v
		}
	}
	return ""
}

// Install asks nvm to install a version spec and returns the exact installed
// version plus nvm's output for the job log.
func Install(ctx context.Context, spec string) (resolved string, output string, err error) {
	before := InstalledVersions()
	out, err := Command(ctx, "install", spec)
	if err != nil {
		return "", out, fmt.Errorf("nvm install %s failed: %w: %s", spec, err, strings.TrimSpace(out))
	}
	resolved = NewInstalled(before, InstalledVersions())
	if resolved == "" {
		if v := versionInOutput.FindString(out); v != "" {
			resolved = v
		}
	}
	if resolved == "" {
		resolved = strings.TrimPrefix(spec, "v")
	}
	return resolved, out, nil
}
