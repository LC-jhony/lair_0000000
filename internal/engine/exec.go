package engine

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/lair-sh/lair/internal/composer"
	"github.com/lair-sh/lair/internal/config"
	"github.com/lair-sh/lair/internal/node"
	"github.com/lair-sh/lair/internal/nvm"
	"github.com/lair-sh/lair/internal/php"
	"github.com/lair-sh/lair/internal/services"
)

// Invocation is the resolved command a shim should execve.
type Invocation struct {
	Path string   // absolute binary path
	Args []string // prefix args to pass before the user's args
	Env  []string // extra env (defaults first so the user's env wins)
}

// ResolveTool resolves a shim name for the current directory using the
// version precedence: isolated > .lairrc (legacy .flockrc) > default > newest installed.
func (e *Engine) ResolveTool(tool, dir string) (Invocation, error) {
	if dir == "" {
		dir, _ = os.Getwd()
	}
	c, err := e.Config.View()
	if err != nil {
		return Invocation{}, err
	}

	switch tool {
	case "php", "php-fpm":
		return e.resolvePHP(c, tool, dir)
	case "composer":
		return e.resolveComposer(c, dir)
	case "node", "npm", "npx":
		return e.resolveNode(c, tool, dir)
	case "psql", "pg_dump", "pg_restore", "createdb", "dropdb":
		return e.resolveClient(c, "postgres", tool)
	case "mysql", "mysqldump":
		inv, err := e.resolveClient(c, "mysql", tool)
		if err == nil {
			return inv, nil
		}
		// MariaDB ships drop-in mysql/mysqldump clients. With only MariaDB
		// installed the shims must still work.
		if fallback, ferr := e.resolveClient(c, "mariadb", tool); ferr == nil {
			return fallback, nil
		}
		return inv, err
	case "mariadb":
		return e.resolveClient(c, "mariadb", tool)
	case "redis-cli":
		return e.resolveClient(c, "redis", tool)
	case "valkey-cli":
		return e.resolveClient(c, "valkey", tool)
	case "mongosh":
		return e.resolveClient(c, "mongodb", tool)
	case "sqlite3":
		return e.resolveToolBinary(SQLiteToolID)
	default:
		return Invocation{}, fmt.Errorf("unknown tool %q", tool)
	}
}

func (e *Engine) resolvePHP(c *config.Config, tool, dir string) (Invocation, error) {
	installed, _ := php.Installed(e.Paths)
	available := make([]string, 0, len(installed))
	for _, info := range installed {
		available = append(available, info.Minor)
	}
	minor, err := php.ResolveVersion(dir, e.IsolatedVersion("php", dir), c.Defaults["php"], available)
	if err != nil {
		return Invocation{}, err
	}
	var info php.Info
	for _, candidate := range installed {
		if candidate.Minor == minor {
			info = candidate
		}
	}
	subdir := "bin"
	if tool == "php-fpm" {
		subdir = "sbin"
	}
	bin := filepath.Join(info.Dir, subdir, tool)
	if _, err := os.Stat(bin); err != nil {
		return Invocation{}, fmt.Errorf("%s not found in PHP %s", tool, minor)
	}
	return Invocation{Path: bin, Env: php.Env(info, e.Paths)}, nil
}

func (e *Engine) resolveComposer(c *config.Config, dir string) (Invocation, error) {
	phpInfo, err := e.resolvePHP(c, "php", dir)
	if err != nil {
		return Invocation{}, err
	}
	phar := filepath.Join(e.Paths.Tools, "composer.phar")
	if _, err := os.Stat(phar); err != nil {
		return Invocation{}, fmt.Errorf("composer.phar is not installed; run 'lair install'")
	}
	return Invocation{
		Path: phpInfo.Path,
		Args: []string{phar},
		Env:  phpInfo.Env,
	}, nil
}

func (e *Engine) resolveNode(c *config.Config, tool, dir string) (Invocation, error) {
	installed := e.installedNode()
	ver, err := node.ResolveVersion(dir, e.IsolatedVersion("node", dir), c.Defaults["node"], installed)
	if err != nil && c.Defaults["node"] != "" && len(installed) > 0 {
		// A default pinned before moving to nvm may no longer be installed.
		ver, err = node.ResolveVersion(dir, e.IsolatedVersion("node", dir), "", installed)
	}
	if err != nil {
		return Invocation{}, err
	}
	// nvm-managed versions run through `lair nvm exec`, so nvm remains the
	// owner of version selection (including .nvmrc) and PATH setup.
	if _, ok := nvm.Installed(ver); ok {
		bin, err := os.Executable()
		if err != nil {
			return Invocation{}, err
		}
		return Invocation{
			Path: bin,
			Args: []string{"nvm", "exec", ver, tool},
			Env:  []string{"NODE_EXTRA_CA_CERTS=" + filepath.Join(e.Paths.Certs, "ca.pem")},
		}, nil
	}
	major, err := node.MajorOf(ver)
	if err != nil {
		return Invocation{}, err
	}
	versionDir := filepath.Join(e.Paths.Node, ver)
	bin := filepath.Join(versionDir, "bin", tool)
	if _, err := os.Stat(bin); err != nil {
		return Invocation{}, fmt.Errorf("%s not found in Node %s", tool, ver)
	}
	return Invocation{Path: bin, Env: node.Env(e.Paths, versionDir, major)}, nil
}

func (e *Engine) resolveClient(c *config.Config, id, tool string) (Invocation, error) {
	ver := c.Defaults[id]
	if ver == "" {
		return Invocation{}, fmt.Errorf("%s has no active version", id)
	}
	build := c.Installed[id+"/"+ver]
	spec, err := services.Build(e.Paths, id, ver, c.Ports[id], build)
	if err != nil {
		return Invocation{}, err
	}
	found := false
	for _, t := range spec.ClientTools {
		if t == tool {
			found = true
		}
	}
	if !found {
		return Invocation{}, fmt.Errorf("tool %s is not provided by %s", tool, id)
	}
	bin := filepath.Join(spec.Dir, "bin", tool)
	if _, err := os.Stat(bin); err != nil {
		return Invocation{}, fmt.Errorf("%s not found: %w", bin, err)
	}
	inv := Invocation{Path: bin}
	switch id {
	case "postgres":
		inv.Env = []string{
			"PGHOST=127.0.0.1",
			fmt.Sprintf("PGPORT=%d", spec.Port),
			"PGUSER=postgres",
		}
	case "mysql", "mariadb":
		inv.Args = []string{"-h", "127.0.0.1", "-P", fmt.Sprint(spec.Port), "-u", "root"}
	case "redis", "valkey":
		if spec.Port != 6379 {
			inv.Args = []string{"-p", fmt.Sprint(spec.Port)}
		}
	}
	return inv, nil
}

// ComposerCommand builds a direct Composer invocation (used by `lair` itself,
// not by shims). argv[0] is the absolute PHP binary.
func ComposerCommand(phpBin, phar string, args ...string) *exec.Cmd {
	return composer.Command(phpBin, phar, args...)
}
