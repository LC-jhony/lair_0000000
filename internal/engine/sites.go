package engine

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/lair-sh/lair/internal/app"
	"github.com/lair-sh/lair/internal/config"
	"github.com/lair-sh/lair/internal/pki"
	"github.com/lair-sh/lair/internal/shims"
)

// Park registers the current directory as a *.test site named after it.
func (e *Engine) Park(projectPath string) (string, error) {
	abs, err := filepath.Abs(projectPath)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(abs)
	if err != nil || !info.IsDir() {
		return "", fmt.Errorf("not a directory: %s", abs)
	}
	name := strings.ToLower(filepath.Base(abs)) + ".test"
	if err := e.Link(name, abs); err != nil {
		return "", err
	}
	return name, nil
}

// Link registers a site with an explicit host name.
func (e *Engine) Link(name, projectPath string) error {
	name = strings.ToLower(strings.TrimSuffix(strings.TrimSpace(name), "."))
	if name == "" || !strings.Contains(name, ".") || strings.ContainsAny(name, "/\\ ") {
		return fmt.Errorf("invalid site name %q", name)
	}
	abs, err := filepath.Abs(projectPath)
	if err != nil {
		return err
	}
	if info, err := os.Stat(abs); err != nil || !info.IsDir() {
		return fmt.Errorf("not a directory: %s", abs)
	}
	if err := e.Config.Update(func(c *config.Config) error {
		s := c.Sites[name]
		// A brand-new site starts enabled; re-linking an existing one keeps
		// its current flag (including an explicit disable).
		if s.Path == "" {
			s.Enabled = true
		}
		s.Path = abs
		c.Sites[name] = s
		return nil
	}); err != nil {
		return err
	}
	e.reloadSites()
	e.syncWeb()
	return nil
}

// Unlink removes a parked site from routing (project files are untouched).
func (e *Engine) Unlink(name string) error {
	name = strings.ToLower(strings.TrimSuffix(name, "."))
	if err := e.Config.Update(func(c *config.Config) error {
		delete(c.Sites, name)
		return nil
	}); err != nil {
		return err
	}
	e.Web.Router.Remove(name)
	e.syncWeb()
	return nil
}

// Secure ensures the local CA exists and issues a certificate for the site.
// The CA still needs to be trusted via the one-time privileged setup.
func (e *Engine) Secure(name string) error {
	name = strings.ToLower(strings.TrimSuffix(name, "."))
	if _, _, err := pki.EnsureCA(e.Paths.Certs); err != nil {
		return err
	}
	var domains []string
	if c, err := e.Config.View(); err == nil {
		if site, ok := c.Sites[name]; ok {
			domains = site.Domains
		}
	}
	sans := e.siteSANs(name, domains)
	if len(sans) == 0 {
		return fmt.Errorf("invalid site name %q for domain %q", name, e.WebSettings().Domain)
	}
	_, _, err := pki.IssueSite(e.Paths.Certs, name, sans...)
	if err != nil {
		return err
	}
	e.reloadSites()
	e.syncWeb()
	return nil
}

// SetSiteHTTPS turns HTTPS on or off for one site. Turning it off removes the
// local certificate; the project files are never touched.
func (e *Engine) SetSiteHTTPS(name string, on bool) error {
	name = strings.ToLower(strings.TrimSuffix(strings.TrimSpace(name), "."))
	if name == "" {
		return fmt.Errorf("invalid site name")
	}
	if on {
		return e.Secure(name)
	}
	cert := filepath.Join(e.Paths.Certs, name+".pem")
	key := filepath.Join(e.Paths.Certs, name+"-key.pem")
	if err := os.Remove(cert); err != nil && !os.IsNotExist(err) {
		return err
	}
	if err := os.Remove(key); err != nil && !os.IsNotExist(err) {
		return err
	}
	e.Web.Router.SetSecure(name, false)
	e.syncWeb()
	return nil
}

// SetSiteDatabase stores the database driver chosen for a site so the UI can
// show it in the Sites card and detail pane.
func (e *Engine) SetSiteDatabase(name, database string) error {
	name = strings.ToLower(strings.TrimSuffix(strings.TrimSpace(name), "."))
	database = strings.TrimSpace(strings.ToLower(database))
	if name == "" || database == "" {
		return fmt.Errorf("site name and database are required")
	}
	return e.Config.Update(func(c *config.Config) error {
		site := c.Sites[name]
		site.Database = database
		c.Sites[name] = site
		return nil
	})
}

// Pin writes a php= or node= key into the project's versionable .lairrc. If a
// legacy .flockrc exists, its contents are carried over so no pin is lost.
func (e *Engine) Pin(tool, value, projectDir string) error {
	if tool != "php" && tool != "node" && tool != "root" {
		return fmt.Errorf("pin accepts only php, node or root")
	}
	if tool == "root" {
		validated, err := app.ValidateProjectRoot(value)
		if err != nil {
			return fmt.Errorf("pin root: %w", err)
		}
		value = validated
	}
	path := filepath.Join(projectDir, ".lairrc")
	body, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		// Migrate existing projects without deleting the legacy file.
		body, err = os.ReadFile(filepath.Join(projectDir, ".flockrc"))
	}
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	lines := strings.Split(strings.TrimSuffix(string(body), "\n"), "\n")
	out := []string{}
	pfx := tool + "="
	for _, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), pfx) {
			continue
		}
		if line != "" {
			out = append(out, line)
		}
	}
	out = append(out, pfx+value)
	return os.WriteFile(path, []byte(strings.Join(out, "\n")+"\n"), 0o644)
}

// Unpin removes a php=, node= or root= line from the project's .lairrc. It
// leaves every other line (including comments and legacy content) untouched.
func (e *Engine) Unpin(tool, projectDir string) error {
	if tool != "php" && tool != "node" && tool != "root" {
		return fmt.Errorf("unpin accepts only php, node or root")
	}
	path := filepath.Join(projectDir, ".lairrc")
	body, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	prefix := tool + "="
	lines := []string{}
	for _, line := range strings.Split(strings.TrimSuffix(string(body), "\n"), "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), prefix) {
			continue
		}
		lines = append(lines, line)
	}
	if len(lines) == 1 && lines[0] == "" {
		return os.Remove(path)
	}
	return os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o644)
}

// PinnedRoot reads the optional root= override from a project's .lairrc.
// The shims use it to select a different data root for that project.
func (e *Engine) PinnedRoot(projectDir string) string {
	for _, name := range []string{".lairrc", ".flockrc"} {
		body, err := os.ReadFile(filepath.Join(projectDir, name))
		if err != nil {
			continue
		}
		for _, line := range strings.Split(string(body), "\n") {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "root=") {
				root, err := app.ValidateProjectRoot(strings.TrimSpace(strings.TrimPrefix(line, "root=")))
				if err != nil {
					return ""
				}
				return root
			}
		}
	}
	return ""
}

// Isolate pins a version for a site in config.json (higher precedence than
// .lairrc). It is reversible with `lair isolate <tool> --off`.
func (e *Engine) Isolate(tool, value, projectDir string) error {
	if tool != "php" && tool != "node" {
		return fmt.Errorf("isolate accepts only php or node")
	}
	abs, err := filepath.Abs(projectDir)
	if err != nil {
		return err
	}
	return e.Config.Update(func(c *config.Config) error {
		for name, site := range c.Sites {
			if site.Path == abs {
				if tool == "php" {
					site.PHP = value
				} else {
					site.Node = value
				}
				c.Sites[name] = site
				return nil
			}
		}
		return fmt.Errorf("directory %s is not a parked site; run 'lair park' first", abs)
	})
}

// IsolatedVersion returns the site-specific pin for a directory, if any.
func (e *Engine) IsolatedVersion(tool, projectDir string) string {
	c, err := e.Config.View()
	if err != nil {
		return ""
	}
	abs, _ := filepath.Abs(projectDir)
	best := ""
	bestLen := -1
	for _, site := range c.Sites {
		if site.Path == "" || abs != site.Path && !strings.HasPrefix(abs, site.Path+string(os.PathSeparator)) {
			continue
		}
		var value string
		if tool == "php" {
			value = site.PHP
		} else {
			value = site.Node
		}
		if value != "" && len(site.Path) > bestLen {
			best, bestLen = value, len(site.Path)
		}
	}
	return best
}

// Uninstall removes every component Lair installed: shims and PATH snippets,
// PHP, Composer, Node, all service binaries, the local CA, daemon state and
// generated cache/log files. Service databases are preserved unless deleteData
// is set. The reversal scripts are deliberately kept by the CLI so the user can
// still remove the privileged system changes (unit, polkit, resolved, CA trust).
func (e *Engine) Uninstall(_, _ string, deleteData bool) error {
	// Never trust paths received over the daemon API: the data root belongs to
	// this engine and the profile snippets belong to the real user's HOME. A
	// hostile payload must not be able to redirect the deletion elsewhere.
	home, err := os.UserHomeDir()
	if err == nil && home != "" {
		_ = shims.RemoveProfiles(home)
	}
	dataDir := e.Paths.Root
	e.Supervisors.StopAll()
	e.FPM.StopAll()
	e.CGI.StopAll()

	if deleteData {
		return os.RemoveAll(dataDir)
	}

	// All runtimes and tools managed by Lair.
	for _, dir := range []string{
		e.Paths.Bin,
		e.Paths.Tools,
		e.Paths.PHP,
		e.Paths.Node,
		e.Paths.Cache,
		e.Paths.Certs,
		e.Paths.Run,
		e.Paths.Log,
	} {
		_ = os.RemoveAll(dir)
	}
	_ = os.RemoveAll(e.Paths.ConfigFile)

	// Remove every installed service version (MariaDB, PostgreSQL, Redis, ...)
	// but keep services/<id>/data so a later install can reuse the databases.
	serviceRoot := e.Paths.Services
	entries, _ := os.ReadDir(serviceRoot)
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		idDir := filepath.Join(serviceRoot, entry.Name())
		sub, _ := os.ReadDir(idDir)
		for _, s := range sub {
			if s.IsDir() && s.Name() == "data" {
				continue
			}
			_ = os.RemoveAll(filepath.Join(idDir, s.Name()))
		}
	}

	// Older stacks could leave these under the data root. Lair itself never
	// installs system nginx/Laravel Valet/Mailcatcher, but clean up any legacy
	// directory found inside its own root.
	for _, name := range []string{"nginx", "valet", "laravel-valet", "mailcatcher", "mailpit"} {
		_ = os.RemoveAll(filepath.Join(dataDir, name))
	}
	return nil
}
