// Package engine holds the orchestration shared by the CLI and the daemon.
// Both front-ends construct an Engine from the resolved paths and the signed
// catalog, then call the same methods. State changes go through the config
// store, which only the daemon owns in production; the CLI uses the same code
// path when no daemon is running.
package engine

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/lair-sh/lair/internal/app"
	"github.com/lair-sh/lair/internal/assets"
	"github.com/lair-sh/lair/internal/config"
	"github.com/lair-sh/lair/internal/download"
	"github.com/lair-sh/lair/internal/installer"
	"github.com/lair-sh/lair/internal/manifest"
	nodepkg "github.com/lair-sh/lair/internal/node"
	"github.com/lair-sh/lair/internal/nvm"
	"github.com/lair-sh/lair/internal/php"
	"github.com/lair-sh/lair/internal/version"
	"github.com/lair-sh/lair/internal/web"
)

// Engine coordinates installs, configuration and routing.
type Engine struct {
	Paths       app.Paths
	Config      *config.Store
	Catalog     *manifest.Catalog
	HTTP        *http.Client
	Installer   *installer.Manager
	Web         *web.Server
	Supervisors *SupervisorRegistry
	FPM         *FPMManager
	CGI         *CGIManager // Windows php-cgi worker pools

	catalogMu sync.Mutex

	// webControlMu serialises the whole configure -> regenerate sequence.
	// Without it two concurrent web actions could persist different settings
	// and leave the generated per-site files out of sync with disk.
	webControlMu sync.Mutex
	webMu        sync.Mutex
	// dbMu serialises snapshot/restore/import/export against each other.
	dbMu       sync.Mutex
	webWarning string
	webError   string

	// mailMu guards the lazy, long-lived SMTP catcher. It is owned by the
	// engine so the CLI fallback and the daemon share the same lifecycle.
	mailMu sync.Mutex
	mail   *mailServer

	// dumpsMu serialises appends to <site>.jsonl across concurrent agents.
	dumpsMu sync.Mutex
}

// New builds an engine and loads parked sites into the web router.
func New(paths app.Paths, catalog *manifest.Catalog) (*Engine, error) {
	if err := paths.Ensure(); err != nil {
		return nil, err
	}
	var baseURLs []string
	if catalog != nil {
		baseURLs = catalog.BaseURLs()
	}
	dl := &download.Downloader{BaseURLs: baseURLs}
	// Installer and Engine must share ONE store: two stores have two mutexes
	// and a concurrent Update from a job can interleave with a CLI/daemon
	// Update between "load from disk" and "rename", losing one of the writes.
	store := config.NewStore(paths.ConfigFile)
	e := &Engine{
		Paths:       paths,
		Config:      store,
		Catalog:     catalog,
		HTTP:        &http.Client{Timeout: 30 * time.Second},
		Installer:   installer.NewManager(paths.Log, dl, store),
		Web:         web.NewServer(paths.Certs, "test"),
		Supervisors: NewSupervisorRegistry(paths.Log),
		FPM:         NewFPMManager(paths.Log),
		CGI:         NewCGIManager(),
	}
	e.Installer.SetOnDone(e.refreshShims)
	s := e.WebSettings()
	e.Web.HTTPPort, e.Web.HTTPSPort, e.Web.Domain, e.Web.Bind = s.HTTPPort, s.HTTPSPort, s.Domain, s.Bind
	e.reloadSites()
	return e, nil
}

func (e *Engine) reloadSites() {
	c, err := e.Config.View()
	if err != nil {
		return
	}
	seen := map[string]bool{}
	for name, site := range c.Sites {
		if site.Path == "" {
			continue
		}
		seen[name] = true
		sock, err := e.Paths.SocketPath("fpm-" + name)
		if err != nil {
			continue
		}
		// Windows uses TCP loopback php-cgi workers instead of unix sockets.
		phpAddr := ""
		if strings.HasPrefix(app.Platform(), "windows-") {
			minor := site.PHP
			if minor == "" {
				minor = c.Defaults["php"]
			}
			if parsed, perr := version.Parse(minor); perr == nil {
				minor = parsed.MinorLine()
			}
			if port, perr := php.CGIPort(minor, false); perr == nil {
				phpAddr = fmt.Sprintf("127.0.0.1:%d", port)
			}
		}
		// The descriptive fields are informational only: routing still uses
		// name/root/php_socket/secure and the three version pins.
		detail := e.siteDetail(c, name, site)
		profile, docRoot, front := siteProfile(site.Path)
		if profile != "" {
			detail.Framework = profile
		}
		_ = e.Web.Router.Add(web.Site{
			Name: name, Root: site.Path, PHPSocket: sock, PHPAddr: phpAddr,
			Secure: e.siteIsSecure(name, site), Enabled: site.Enabled,
			PHP: site.PHP, Node: site.Node, Database: site.Database,
			Type: detail.Type, Framework: detail.Framework,
			PHPVersion: detail.PHPVersion, PHPSource: detail.PHPSource, PHPFile: detail.PHPFile,
			NodeSource: detail.NodeSource, NodeFile: detail.NodeFile,
			RootState: detail.RootState, RootSuggestions: detail.RootSuggestions,
			Domains: detail.Domains, Problem: detail.Problem, Notice: detail.Notice,
			URL: detail.URL, DocRoot: docRoot, FrontController: front,
		})
	}
	for _, registered := range e.Web.Router.List() {
		if !seen[registered.Name] {
			e.Web.Router.Remove(registered.Name)
		}
	}
	// Aliases may have changed: cached certificates must be re-issued with the
	// new SAN list on the next TLS handshake.
	e.Web.ClearCertCache()
	e.syncFPM()
}

// Status is a snapshot of the whole installation.
type Status struct {
	Home     string                 `json:"home"`
	PHP      []php.Info             `json:"php"`
	Node     []string               `json:"node"`
	Composer string                 `json:"composer"`
	Services map[string]ServiceView `json:"services"`
	Sites    []web.Site             `json:"sites"`
	Web      WebView                `json:"web"`
	DNS      DNSStatus              `json:"dns"`
	CA       CAStatus               `json:"ca"`
	Jobs     []installer.Status     `json:"jobs"`
	Defaults map[string]string      `json:"defaults"`
}

// Status collects current state from disk and the config store.
func (e *Engine) Status() (Status, error) {
	// Status is a read-only snapshot: project discovery is a startup/action
	// concern (see daemon.NewRuntime), not something a shim should trigger.
	c, err := e.Config.View()
	if err != nil {
		return Status{}, err
	}
	phpVersions, _ := php.Installed(e.Paths)
	nodeVersions := e.installedNode()
	services := map[string]ServiceView{}
	for _, id := range servicesList() {
		services[id] = e.serviceView(c, id)
	}
	return Status{
		Home: e.Paths.Root, PHP: phpVersions, Node: nodeVersions,
		Composer: e.ComposerVersion(),
		Services: services, Sites: e.Web.Router.List(), Web: e.WebStatus(),
		DNS: e.statusDNS(c.Web.Domain), CA: e.statusCA(),
		Jobs: e.Installer.List(), Defaults: c.Defaults,
	}, nil
}

func (e *Engine) installedNode() []string {
	out := nvm.InstalledVersions()
	seen := make(map[string]bool, len(out))
	for _, v := range out {
		seen[v] = true
	}
	// Versions installed by older Lair releases still live under Paths.Node.
	entries, err := os.ReadDir(e.Paths.Node)
	if err == nil {
		for _, entry := range entries {
			if !entry.IsDir() || entry.Name() == "global" || seen[entry.Name()] {
				continue
			}
			if _, err := os.Stat(filepath.Join(e.Paths.Node, entry.Name(), "bin", "node")); err == nil {
				out = append(out, entry.Name())
				seen[entry.Name()] = true
			}
		}
	}
	version.SortNewest(out)
	return out
}

func (e *Engine) manifest() (*manifest.Manifest, error) {
	if err := e.ensureCatalog(); err != nil {
		return nil, err
	}
	return e.Catalog.Get(), nil
}

// ensureCatalog makes sure the engine can resolve packages even when the
// daemon was started with a nil or empty catalog (for example, an older
// process still running after an upgrade, or a fresh HOME with no cache).
// The signed example catalog embedded in the binary is always accepted as a
// last resort; a real cached or refreshed manifest takes precedence.
func (e *Engine) ensureCatalog() error {
	e.catalogMu.Lock()
	defer e.catalogMu.Unlock()
	if e.Catalog == nil {
		pub, err := assets.PublicKey()
		if err != nil {
			return fmt.Errorf("embedded catalog key: %w", err)
		}
		channel := strings.TrimSpace(os.Getenv("LAIR_CHANNEL"))
		if channel == "" {
			channel = "stable"
		}
		name := "manifest.json"
		if channel != "stable" {
			name = "manifest-" + channel + ".json"
		}
		cat, err := manifest.NewCatalog(manifest.CatalogConfig{
			PublicKey: pub, Channel: channel, BaseURLs: manifest.DefaultBaseURLs,
			Root: e.Paths.Root, ManifestName: name,
		})
		if err != nil {
			return fmt.Errorf("embedded catalog: %w", err)
		}
		e.Catalog = cat
	}
	if e.Catalog.Get() != nil {
		return nil
	}
	data, sig := assets.ExampleCatalog()
	if err := e.Catalog.Load(data, sig); err != nil {
		return fmt.Errorf("catalog is empty and embedded catalog was rejected: %w", err)
	}
	if e.Catalog.Get() == nil {
		return fmt.Errorf("catalog is empty; wait for a refresh or run 'lair update'")
	}
	return nil
}

func (e *Engine) artifact(pkg, ver string) (manifest.Artifact, error) {
	mf, err := e.manifest()
	if err != nil {
		return manifest.Artifact{}, err
	}
	a, ok := mf.Get(pkg, ver, app.Platform())
	if !ok {
		return manifest.Artifact{}, fmt.Errorf("no %s %s build for %s", pkg, ver, app.Platform())
	}
	if err := manifest.VerifyArtifact(a); err != nil {
		return manifest.Artifact{}, err
	}
	return a, nil
}

func (e *Engine) available(pkg string) ([]string, error) {
	mf, err := e.manifest()
	if err != nil {
		return nil, err
	}
	v := mf.Versions(pkg, app.Platform())
	version.SortNewest(v)
	return v, nil
}

// InstallPHP installs a PHP minor line (or exact version) and returns the job.
func (e *Engine) InstallPHP(ctx context.Context, request string) (*installer.Job, error) {
	available, err := e.available("php")
	if err != nil {
		return nil, err
	}
	picked, err := pickPHP(request, available)
	if err != nil {
		return nil, err
	}
	art, err := e.artifact("php", picked)
	if err != nil {
		return nil, err
	}
	parsed, _ := version.Parse(picked)
	minor := parsed.MinorLine()
	// Windows builds from windows.php.net ship their own ext/ directory. A
	// separate php-exts pack is optional: it is merged on top when published.
	dest := filepath.Join(e.Paths.PHP, minor)
	caFile := filepath.Join(e.Paths.Certs, "ca.pem")
	extPack := e.extPackBuild(minor)
	// On Windows the extension pack is a second verified archive merged into
	// the same staged tree before the atomic swap.
	if extPack != "" {
		if mf, merr := e.manifest(); merr == nil {
			if pack, ok := mf.Get("php-exts", minor, app.Platform()); ok {
				art.Components = append(art.Components, manifest.Component{
					URL: pack.URL, SHA256: pack.SHA256, Size: pack.Size,
				})
			}
		}
	}
	info := php.Info{Version: picked, Build: art.Build, Minor: minor, ExtPackBuild: extPack}
	job, err := e.Installer.Start(ctx, installer.Request{
		ID: "php-" + minor, Package: "php", Version: picked, Platform: app.Platform(),
		Artifact: art, Dest: dest, Expected: phpExpected(),
		ConfigKey: "php/" + minor,
		// User-owned configuration survives an in-place upgrade. The staged
		// binaries are probed before the atomic swap.
		Preserve: []string{"php.ini", "conf.d", "conf.d-cli", "conf.d-xdebug", "lair.json"},
		VerifyStaging: func(staging string) error {
			return php.Probe(staging, picked)
		},
		MergeStaging: func(staging string, _ []string) error {
			return php.MergeExtensionPack(staging)
		},
		OnSuccess: func(dir string) error {
			info.Dir = dir
			if err := php.WriteMarker(info); err != nil {
				return err
			}
			if _, err := php.EnsureINI(info, caFile); err != nil {
				return err
			}
			return e.Config.Update(func(c *config.Config) error {
				if c.Defaults["php"] == "" {
					c.Defaults["php"] = minor
				}
				return nil
			})
		},
	})
	if err != nil {
		return nil, err
	}
	return job, nil
}

func pickPHP(request string, available []string) (string, error) {
	if request == "" {
		if len(available) == 0 {
			return "", fmt.Errorf("no PHP versions available")
		}
		return available[0], nil
	}
	// Exact version.
	for _, v := range available {
		if v == request {
			return v, nil
		}
	}
	// Minor line: newest patch in that line.
	parsed, err := version.Parse(request)
	if err != nil {
		return "", err
	}
	line := parsed.MinorLine()
	for _, v := range available {
		if strings.HasPrefix(v, line+".") {
			return v, nil
		}
	}
	return "", fmt.Errorf("PHP %s is not available", request)
}

// phpExpected lists the executables a PHP package must contain on this
// platform. Windows ships the official flat build (no php-fpm); Unix builds
// ship both the CLI and FPM.
func phpExpected() []string {
	if strings.HasPrefix(app.Platform(), "windows-") {
		// The official zip keeps executables at the archive root; the staging
		// merge moves them into bin/ before the version probe runs.
		return []string{"php.exe", "php-cgi.exe"}
	}
	return []string{"bin/php", "sbin/php-fpm"}
}

// extPackBuild returns the Windows extension pack build for a minor line, or
// "" when no pack is published (or the published build was yanked). A minor
// without a pack must not be offered on Windows.
func (e *Engine) extPackBuild(minor string) string {
	if !strings.HasPrefix(app.Platform(), "windows-") {
		return ""
	}
	mf, err := e.manifest()
	if err != nil {
		return ""
	}
	a, ok := mf.Get("php-exts", minor, app.Platform())
	if !ok || a.SHA256 == "" {
		return ""
	}
	if _, yanked := a.IsYanked(); yanked {
		return ""
	}
	return a.Build
}

// PHPUse sets the default PHP minor line.
func (e *Engine) PHPUse(minor string) error {
	installed, err := php.Installed(e.Paths)
	if err != nil {
		return err
	}
	found := ""
	for _, info := range installed {
		if info.Minor == minor {
			found = minor
		}
	}
	if found == "" {
		return fmt.Errorf("PHP %s is not installed", minor)
	}
	return e.Config.Update(func(c *config.Config) error {
		c.Defaults["php"] = minor
		return nil
	})
}

// LatestLTS returns the newest LTS version of a package for this platform, or
// "" when the catalog does not flag any LTS release.
func (e *Engine) LatestLTS(pkg string) string {
	available, err := e.available(pkg)
	if err != nil {
		return ""
	}
	mf, err := e.manifest()
	if err != nil {
		return ""
	}
	for _, v := range available {
		if mf.LTSOf(pkg, v, app.Platform()) != "" {
			return v
		}
	}
	return ""
}

// InstallNode installs a Node version from the signed catalog and returns the
// job. Every runtime Lair installs goes through the same verified download,
// SHA-256 check and atomic swap. Versions previously installed by nvm remain
// readable through `installedNode`, but nvm is never used to install or update
// a version: new installs are owned by Lair on every platform.
func (e *Engine) InstallNode(ctx context.Context, request string) (*installer.Job, error) {
	return e.installNodeFromCatalog(ctx, request)
}

// installNodeWithNVM delegates the installation to nvm through the lair CLI and
// reports nvm's own output on the job log.
func (e *Engine) installNodeWithNVM(ctx context.Context, request string) (*installer.Job, error) {
	spec := strings.TrimSpace(request)
	switch {
	case spec == "", strings.EqualFold(spec, "lts"), strings.EqualFold(spec, "latest-lts"):
		spec = "--lts"
	case strings.EqualFold(spec, "latest"), strings.EqualFold(spec, "node"):
		spec = "node"
	}
	if err := nvm.ValidateArgs([]string{"install", spec}); err != nil {
		return nil, err
	}
	before := nvm.InstalledVersions()
	var job *installer.Job
	job, err := e.Installer.Run(ctx, installer.RunRequest{
		ID:      "node-" + strings.NewReplacer("/", "-", "*", "", " ", "-").Replace(spec),
		Package: "node", Version: spec,
		Run: func(ctx context.Context, log func(string)) error {
			out, err := nvm.Command(ctx, "install", spec)
			for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
				if line = strings.TrimSpace(line); line != "" {
					log(line)
				}
			}
			return err
		},
		OnSuccess: func() error {
			resolved := nvm.NewInstalled(before, nvm.InstalledVersions())
			if resolved == "" {
				resolved = spec
			}
			job.SetVersion(resolved)
			return e.Config.Update(func(c *config.Config) error {
				if c.Defaults["node"] == "" {
					c.Defaults["node"] = resolved
				}
				return nil
			})
		},
	})
	return job, err
}

// installNodeFromCatalog is the legacy path kept for Windows, where nvm is not
// available. It still installs through Lair's verified signed catalog.
func (e *Engine) installNodeFromCatalog(ctx context.Context, request string) (*installer.Job, error) {
	available, err := e.available("node")
	if err != nil {
		return nil, err
	}

	// An exact version typed by the user must not be silently replaced by the
	// newest release in its major line. If the signed catalog does not carry it,
	// resolve it against the checksums published by nodejs.org.
	if exact, normErr := nodepkg.NormalizeOfficialVersion(request); normErr == nil {
		found := false
		for _, v := range available {
			if v == exact || v == strings.TrimPrefix(exact, "v") {
				found = true
				break
			}
		}
		if !found {
			// An existing nvm installation is the preferred source for exact
			// versions it already manages: no network round trip and the same
			// tool the user chose. Catalog/major installs still use the signed
			// manifest path above.
			if nvm.Available() {
				return e.installNodeWithNVM(ctx, request)
			}
			art, err := nodepkg.OfficialArtifact(ctx, e.HTTP, exact, app.Platform())
			if err != nil {
				return nil, fmt.Errorf("node %s is not in the signed catalog and could not be verified from nodejs.org: %w", exact, err)
			}
			return e.installNodeArtifact(ctx, exact, art)
		}
	}

	picked := request
	if request == "" || strings.EqualFold(request, "lts") || strings.EqualFold(request, "latest-lts") {
		picked = e.LatestLTS("node")
		if picked == "" {
			if len(available) == 0 {
				return nil, fmt.Errorf("no Node versions available in the catalog")
			}
			picked, err = version.ExactOrLatest("", available)
			if err != nil {
				return nil, err
			}
		}
	} else {
		// 22, 22.11 and 22.11.0 all resolve to the newest published release
		// of that major: the catalog keeps one version per major line.
		picked, err = version.LatestInMajor(request, available)
		if err != nil {
			return nil, err
		}
	}
	art, err := e.artifact("node", picked)
	if err != nil {
		return nil, err
	}
	return e.installNodeArtifact(ctx, picked, art)
}

func (e *Engine) installNodeArtifact(ctx context.Context, picked string, art manifest.Artifact) (*installer.Job, error) {
	dest := filepath.Join(e.Paths.Node, picked)
	major, err := nodepkg.MajorOf(picked)
	if err != nil {
		return nil, err
	}
	job, err := e.Installer.Start(ctx, installer.Request{
		ID: "node-" + picked, Package: "node", Version: picked, Platform: app.Platform(),
		Artifact: art, Dest: dest,
		Expected:  nodepkg.Expected(),
		ConfigKey: "node/" + picked,
		OnSuccess: func(string) error {
			// Global packages live outside the version directory: they survive
			// a patch/minor upgrade inside the same major.
			if err := nodepkg.EnsureGlobal(e.Paths, major); err != nil {
				return err
			}
			if err := nodepkg.RemoveOlderInMajor(e.Paths, picked); err != nil {
				return err
			}
			return e.Config.Update(func(c *config.Config) error {
				if c.Defaults["node"] == "" {
					c.Defaults["node"] = picked
				}
				return nil
			})
		},
	})
	if err != nil {
		return nil, err
	}
	return job, nil
}

// NodeDefault sets the default Node version.
func (e *Engine) NodeDefault(ver string) error {
	installed := e.installedNode()
	found := ""
	for _, v := range installed {
		if v == ver {
			found = ver
		} else if p, err := version.Parse(v); err == nil && p.MajorLine() == ver {
			found = v
		}
	}
	if found == "" {
		return fmt.Errorf("Node %s is not installed", ver)
	}
	major, err := nodepkg.MajorOf(found)
	if err != nil {
		return err
	}
	// The stable `current` path follows the default major, so global CLIs from
	// that major are available in a fresh shell.
	if err := nodepkg.EnsureGlobal(e.Paths, major); err != nil {
		return err
	}
	return e.Config.Update(func(c *config.Config) error {
		c.Defaults["node"] = found
		return nil
	})
}

// NodeUninstall removes one major line. It never touches other majors or the
// user's global packages (which live under node/global/<major>).
func (e *Engine) NodeUninstall(major string) error {
	installed := e.installedNode()
	target := ""
	for _, v := range installed {
		m, err := nodepkg.MajorOf(v)
		if err == nil && m == major {
			target = v
			break
		}
	}
	if target == "" {
		return fmt.Errorf("Node %s is not installed", major)
	}
	c, err := e.Config.View()
	if err != nil {
		return err
	}
	if c.Defaults["node"] == target {
		return fmt.Errorf("Node %s is the default; choose another default before uninstalling", major)
	}
	// A directory under node/ is only removed when it looks like an installed
	// version (bin/node or node.exe), never when it is `global`.
	dir := filepath.Join(e.Paths.Node, target)
	if info, err := os.Stat(nodepkg.Binary(dir)); err != nil || info.IsDir() {
		return fmt.Errorf("%s does not look like a Node install", dir)
	}
	return os.RemoveAll(dir)
}

// NodeUpdate installs the newest published release of a Node major.
func (e *Engine) NodeUpdate(ctx context.Context, major string) (*installer.Job, error) {
	return e.InstallNode(ctx, major)
}

// Logs returns the contents of a log file by name without allowing traversal.
// Site logs are addressed as `site:<registered-name>:<file>` because they live
// in the project's storage/logs directory, not in LAIR_HOME/log.
func (e *Engine) Logs(name string) (string, error) {
	if strings.HasPrefix(name, "site:") {
		parts := strings.Split(name, ":")
		if len(parts) != 3 || parts[1] == "" {
			return "", fmt.Errorf("invalid log name")
		}
		file := parts[2]
		if file == "" || file == "." || file == ".." ||
			strings.ContainsAny(file, `/\\`) || filepath.Base(file) != file ||
			!strings.HasSuffix(file, ".log") {
			return "", fmt.Errorf("invalid log name")
		}
		c, err := e.Config.View()
		if err != nil {
			return "", err
		}
		site, ok := c.Sites[parts[1]]
		if !ok || site.Path == "" {
			return "", fmt.Errorf("unknown site %q", parts[1])
		}
		data, err := os.ReadFile(filepath.Join(site.Path, "storage", "logs", file))
		if err != nil {
			return "", err
		}
		return string(data), nil
	}
	if name == "" || strings.ContainsAny(name, `/\\`) || name == "." || name == ".." {
		return "", fmt.Errorf("invalid log name")
	}
	path := filepath.Join(e.Paths.Log, name)
	if !strings.HasSuffix(path, ".log") {
		path += ".log"
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// ListLogs returns the available log file names.
func (e *Engine) ListLogs() ([]string, error) {
	entries, err := os.ReadDir(e.Paths.Log)
	if err != nil {
		return []string{}, nil
	}
	out := []string{}
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".log") {
			out = append(out, entry.Name())
		}
	}
	sort.Strings(out)
	return out, nil
}

// WaitJob polls a job until it finishes or ctx expires.
func (e *Engine) WaitJob(ctx context.Context, job *installer.Job) (installer.Status, error) {
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()
	for {
		st := job.Status()
		if st.Done {
			return st, nil
		}
		select {
		case <-ctx.Done():
			return st, ctx.Err()
		case <-ticker.C:
		}
	}
}
