package engine

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/lair-sh/lair/internal/node"
	"github.com/lair-sh/lair/internal/nvm"
	"github.com/lair-sh/lair/internal/php"
	"github.com/lair-sh/lair/internal/version"
)

var extName = regexp.MustCompile(`^[a-zA-Z0-9_]+$`)

// ExtRequired reports the ext-* requirements found in a project's
// composer.json / composer.lock and which of them its PHP does not provide.
// It is advisory: it never edits project files.
func (e *Engine) ExtRequired(projectDir string) ([]string, []string, error) {
	c, err := e.Config.View()
	if err != nil {
		return nil, nil, err
	}
	inv, err := e.resolvePHP(c, "php", projectDir)
	if err != nil {
		return nil, nil, err
	}
	ini := ""
	for _, env := range inv.Env {
		if strings.HasPrefix(env, "PHPRC=") {
			ini = strings.TrimPrefix(env, "PHPRC=")
		}
	}
	if ini == "" {
		return nil, nil, fmt.Errorf("could not determine php.ini for PHP in %s", projectDir)
	}
	return php.RequiredExtensions(projectDir, ini)
}

// ExtAdd installs a PHP extension requirement with the PHP selected for this
// project. The extension name is strictly validated: it is interpolated into
// a Composer requirement and must never contain shell or Composer metacharacters.
func (e *Engine) ExtAdd(ctx context.Context, projectDir, ext string) (string, error) {
	if ext == "" || !extName.MatchString(ext) {
		return "", fmt.Errorf("invalid PHP extension name %q", ext)
	}
	c, err := e.Config.View()
	if err != nil {
		return "", err
	}
	inv, err := e.resolveComposer(c, projectDir)
	if err != nil {
		return "", err
	}
	args := append(append([]string{}, inv.Args...), "require", "ext-"+ext+":*")
	cmd := exec.CommandContext(ctx, inv.Path, args...)
	cmd.Dir = projectDir
	cmd.Env = append(append([]string{}, inv.Env...), os.Environ()...)
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// nodeVersionDir resolves the version and directory of the Node selected for
// a project, without going through nvm for catalog-owned installs.
func (e *Engine) nodeVersionDir(projectDir string) (string, string, error) {
	c, err := e.Config.View()
	if err != nil {
		return "", "", err
	}
	installed := e.installedNode()
	ver, err := node.ResolveVersion(projectDir, e.IsolatedVersion("node", projectDir), c.Defaults["node"], installed)
	if err != nil {
		return "", "", err
	}
	major, err := node.MajorOf(ver)
	if err != nil {
		return "", "", err
	}
	return ver, major, nil
}

// NpmUpdate installs the newest npm compatible with the Node major selected
// for a project, into that major's isolated global prefix.
func (e *Engine) NpmUpdate(ctx context.Context, projectDir string) (string, error) {
	ver, major, err := e.nodeVersionDir(projectDir)
	if err != nil {
		return "", err
	}
	// nvm-managed versions keep their own npm prefix; update them through nvm.
	if _, ok := nvm.Installed(ver); ok {
		return "", fmt.Errorf("Node %s is managed by nvm; run 'npm install -g npm@latest' in an nvm shell instead", ver)
	}
	return node.UpdateNPM(ctx, e.HTTP, e.Paths, filepath.Join(e.Paths.Node, ver), major)
}

// PHPUninstall removes one PHP minor line. The default version cannot be
// removed: doing so would break every project using it.
func (e *Engine) PHPUninstall(minor string) error {
	c, err := e.Config.View()
	if err != nil {
		return err
	}
	if c.Defaults["php"] == minor {
		return fmt.Errorf("PHP %s is the default; choose another default before uninstalling", minor)
	}
	installed, err := php.Installed(e.Paths)
	if err != nil {
		return err
	}
	for _, info := range installed {
		if info.Minor == minor {
			return os.RemoveAll(info.Dir)
		}
	}
	return fmt.Errorf("PHP %s is not installed", minor)
}

// NodeUpdateSpec resolves a major request to the installed version that would
// be replaced. It is exposed so the UI can label "Update" correctly.
func (e *Engine) NodeUpdateSpec(major string) (string, error) {
	available, err := e.available("node")
	if err != nil {
		return "", err
	}
	picked, err := version.LatestInMajor(major, available)
	if err != nil {
		return "", err
	}
	installed := e.installedNode()
	for _, v := range installed {
		if v == picked {
			return "", fmt.Errorf("Node %s is already up to date", picked)
		}
	}
	return picked, nil
}
