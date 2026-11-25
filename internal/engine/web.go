package engine

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/lair-sh/lair/internal/config"
	"github.com/lair-sh/lair/internal/php"
	"github.com/lair-sh/lair/internal/pki"
)

// WebView is the web layer snapshot exposed in the daemon status. The single
// embedded Go server is the web layer; there is no engine/mode selector.
type WebView struct {
	HTTPS        bool   `json:"https"`
	HTTPPort     int    `json:"http_port"`
	HTTPSPort    int    `json:"https_port"`
	Bind         string `json:"bind"`
	Domain       string `json:"domain"`
	RedirectHTTP bool   `json:"redirect_http"`
	Warning      string `json:"warning,omitempty"`
	Error        string `json:"error,omitempty"`
}

// WebRequest carries a web control operation. Pointer booleans distinguish
// "not supplied" from an explicit false, so a partial update is safe.
type WebRequest struct {
	Op           string `json:"op"` // status | configure | https-on | https-off
	HTTPPort     int    `json:"http_port,omitempty"`
	HTTPSPort    int    `json:"https_port,omitempty"`
	HTTPS        *bool  `json:"https,omitempty"`
	RedirectHTTP *bool  `json:"redirect_http,omitempty"`
	Bind         string `json:"bind,omitempty"`
	Domain       string `json:"domain,omitempty"`
}

// WebResult is returned by every web control operation.
type WebResult struct {
	Output string  `json:"output,omitempty"`
	Web    WebView `json:"web"`
}

// WebSettings returns the normalised, persisted web configuration.
func (e *Engine) WebSettings() config.WebSettings {
	c, err := e.Config.View()
	if err != nil {
		return config.DefaultWeb()
	}
	s, err := c.Web.Normalize()
	if err != nil {
		return config.DefaultWeb()
	}
	return s
}

// WebStatus returns the current web snapshot without touching the network.
func (e *Engine) WebStatus() WebView {
	s := e.WebSettings()
	e.webMu.Lock()
	warning, errText := e.webWarning, e.webError
	e.webMu.Unlock()
	view := WebView{
		HTTPS: s.HTTPS, HTTPPort: s.HTTPPort, HTTPSPort: s.HTTPSPort,
		Bind: s.Bind, Domain: s.Domain, RedirectHTTP: s.RedirectHTTP,
		Warning: warning, Error: errText,
	}
	// 0.0.0.0 is an explicit opt-in, but the user must be told that it exposes
	// every parked site to the local network even if there is another warning.
	if view.Bind == "0.0.0.0" {
		view.Warning = appendWarning(view.Warning, "bind 0.0.0.0 expone los sitios a la LAN; usa 127.0.0.1 si solo trabajas en local")
	}
	return view
}

func appendWarning(existing, next string) string {
	if existing == "" {
		return next
	}
	return existing + " · " + next
}

// setWebState records the last warning/error for this engine. State is
// per-engine (not package-global) so tests and multiple engines cannot race.
func (e *Engine) setWebState(warning, errText string) {
	e.webMu.Lock()
	e.webWarning, e.webError = warning, errText
	e.webMu.Unlock()
}

// siteVHosts builds one virtual host per parked site. With HTTPS enabled the
// local CA issues the site certificate on demand, so a newly parked site is
// immediately reachable over TLS.
func (e *Engine) siteVHosts(s config.WebSettings) ([]vhostSite, error) {
	c, err := e.Config.View()
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(c.Sites))
	for name := range c.Sites {
		names = append(names, name)
	}
	sort.Strings(names)
	sites := make([]vhostSite, 0, len(names))
	for _, name := range names {
		site := c.Sites[name]
		if site.Path == "" {
			continue
		}
		ns, err := e.siteVHost(name, site, s)
		if err != nil {
			return nil, err
		}
		// A site whose HTTPS was explicitly turned off must stay off: syncWeb
		// runs for every config change and must not re-issue its certificate.
		if s.HTTPS && e.siteIsSecure(name, site) {
			if err := e.issueSiteCertificate(name, site.Domains); err != nil {
				return nil, fmt.Errorf("certificado para %s: %w", name, err)
			}
		}
		sites = append(sites, ns)
	}
	return sites, nil
}

// siteVHost translates one registered site into a virtual host description.
// Enabled comes from config.json (legacy entries default to true on load) and
// RootState is re-detected from disk so a project without a front controller
// is served the built-in status page instead of a broken FastCGI vhost.
func (e *Engine) siteVHost(name string, site config.Site, s config.WebSettings) (vhostSite, error) {
	sock, err := e.Paths.SocketPath("fpm-" + name)
	if err != nil {
		return vhostSite{}, err
	}
	_, docRoot, _ := siteProfile(site.Path)
	ns := vhostSite{
		Name: name, Root: site.Path, DocRoot: docRoot, PHPSocket: sock,
		Domains:   append([]string(nil), site.Domains...),
		Enabled:   site.Enabled,
		RootState: detectSiteRoot(site.Path).state,
		PagesDir:  vhostPagesDir(e.Paths.Root),
		PHP:       site.PHP,
	}
	if ns.PHP == "" {
		if c, err := e.Config.View(); err == nil {
			ns.PHP = c.Defaults["php"]
		}
	}
	installed, _ := php.Installed(e.Paths)
	for _, info := range installed {
		if info.Minor == ns.PHP || info.Version == ns.PHP || info.Minor == strings.SplitN(ns.PHP, ".", 2)[0] {
			ns.PHPInstalled = true
			break
		}
	}
	if s.HTTPS {
		ns.Cert = filepath.Join(e.Paths.Certs, name+".pem")
		ns.Key = filepath.Join(e.Paths.Certs, name+"-key.pem")
	}
	return ns, nil
}

// siteSANs includes the configured aliases in the local certificate so an
// https:// alias does not fail with a hostname mismatch.
func (e *Engine) siteSANs(name string, domains []string) []string {
	sans := pki.SANsFor(name, e.WebSettings().Domain)
	if len(sans) == 0 {
		return nil
	}
	seen := map[string]bool{}
	out := []string{}
	for _, value := range append(sans, domains...) {
		value = strings.ToLower(strings.TrimSuffix(strings.TrimSpace(value), "."))
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	return out
}

func (e *Engine) issueSiteCertificate(name string, domains []string) error {
	sans := e.siteSANs(name, domains)
	if len(sans) == 0 {
		return fmt.Errorf("invalid site name %q for domain %q", name, e.WebSettings().Domain)
	}
	_, _, err := pki.IssueSite(e.Paths.Certs, name, sans...)
	return err
}

// syncVHosts regenerates the generated per-site files from config.json. No
// external process is involved: the embedded Go server owns routing directly.
func (e *Engine) syncVHosts(s config.WebSettings) error {
	sites, err := e.siteVHosts(s)
	if err != nil {
		return err
	}
	return writeVHosts(e.Paths.Root, s, sites)
}

// syncWeb is called after site changes. A failure is reported on the status
// but never rolls back the site operation itself.
func (e *Engine) syncWeb() {
	e.webControlMu.Lock()
	defer e.webControlMu.Unlock()
	if err := e.syncVHosts(e.WebSettings()); err != nil {
		e.setWebState("", err.Error())
		return
	}
	e.setWebState("", "")
}

// applyWebToServer mirrors persisted settings into the embedded Go server so
// its listeners use the same ports, domain and bind address as config.json.
func (e *Engine) applyWebToServer(s config.WebSettings) {
	e.Web.SetConfig(s.HTTPPort, s.HTTPSPort, s.Bind, s.Domain)
}

// rollbackWeb restores both the persisted settings and the in-memory server
// fields. A failed write must not leave e.Web pointing at ports that were
// never applied to the running listener.
func (e *Engine) rollbackWeb(previous config.WebSettings) {
	_ = e.Config.Update(func(c *config.Config) error { c.Web = previous; return nil })
	e.applyWebToServer(previous)
}

// webBinary is the executable a user must grant CAP_NET_BIND_SERVICE to. The
// engine's installed binary is preferred; the running executable is the
// fallback for development builds.
func (e *Engine) webBinary() string {
	installed := filepath.Join(e.Paths.Bin, "lair")
	if info, err := os.Stat(installed); err == nil && !info.IsDir() {
		return installed
	}
	if exe, err := os.Executable(); err == nil {
		return exe
	}
	return installed
}

// webBindAdvice turns a low-port bind failure into an actionable message. It
// never changes the requested ports: falling back silently to high ports would
// break *.test URLs and the privileged setup plan.
func webBindAdvice(err error, bin string) error {
	if errors.Is(err, syscall.EACCES) || errors.Is(err, syscall.EPERM) {
		return fmt.Errorf("%w; otorga la capacidad con: sudo setcap cap_net_bind_service=+ep %s "+
			"(o elige puertos altos en Configuración de Lair)", err, bin)
	}
	return err
}

// preflightWebPorts binds each configured port once before starting the
// servers. services.CheckPort reports EACCES as "port in use", which hides the
// real cause on unprivileged ports; this probe keeps the error honest.
func (e *Engine) preflightWebPorts(s config.WebSettings) error {
	for _, port := range []int{s.HTTPPort, s.HTTPSPort} {
		addr := net.JoinHostPort(s.Bind, strconv.Itoa(port))
		ln, err := net.Listen("tcp", addr)
		if err != nil {
			return fmt.Errorf("no se pudo enlazar %s: %w", addr, err)
		}
		_ = ln.Close()
	}
	return nil
}

// StartWeb configures the embedded Go server from persisted settings and
// starts it. It is the only web listener Lair runs. A bind failure is returned
// to the caller; the daemon treats it as non-fatal so DNS and the local API
// keep running.
func (e *Engine) StartWeb() error {
	e.webControlMu.Lock()
	defer e.webControlMu.Unlock()
	s := e.WebSettings()
	if e.Web.HasOverrides() {
		// CLI flags (--http-port, --https-port, --domain, ...) were applied by
		// daemon.Defaults before StartWeb. They win over persisted settings for
		// this run.
		httpPort, httpsPort, bind, domain := e.Web.Config()
		s.HTTPPort, s.HTTPSPort, s.Bind, s.Domain = httpPort, httpsPort, bind, domain
	}
	e.applyWebToServer(s)
	if err := e.preflightWebPorts(s); err != nil {
		advised := webBindAdvice(err, e.webBinary())
		e.setWebState("", advised.Error())
		return advised
	}
	if err := e.Web.Start(); err != nil {
		advised := webBindAdvice(err, e.webBinary())
		e.setWebState("", advised.Error())
		return advised
	}
	e.setWebState("", "")
	return nil
}

// WebControl implements the single daemon action used by desktop and CLI.
func (e *Engine) WebControl(ctx context.Context, req WebRequest) (WebResult, error) {
	op := strings.ToLower(strings.TrimSpace(req.Op))
	if op == "" {
		op = "status"
	}
	// Serialise the full configure -> regenerate sequence so two concurrent
	// callers cannot interleave and leave persisted settings out of sync.
	e.webControlMu.Lock()
	defer e.webControlMu.Unlock()
	if op == "status" {
		return WebResult{Web: e.WebStatus()}, nil
	}
	s := e.WebSettings()
	newSettings := s
	switch op {
	case "configure":
		if req.HTTPPort != 0 {
			newSettings.HTTPPort = req.HTTPPort
		}
		if req.HTTPSPort != 0 {
			newSettings.HTTPSPort = req.HTTPSPort
		}
		if req.HTTPS != nil {
			newSettings.HTTPS = *req.HTTPS
		}
		if req.RedirectHTTP != nil {
			newSettings.RedirectHTTP = *req.RedirectHTTP
		}
		if req.Bind != "" {
			newSettings.Bind = req.Bind
		}
		if req.Domain != "" {
			newSettings.Domain = req.Domain
		}
		var err error
		newSettings, err = newSettings.Normalize()
		if err != nil {
			return WebResult{}, err
		}
	case "https-on", "https-off":
		on := op == "https-on"
		newSettings.HTTPS = on
		if on {
			newSettings.RedirectHTTP = true
		}
	default:
		return WebResult{}, fmt.Errorf("unknown web operation %q", req.Op)
	}

	previous := s
	if err := e.Config.Update(func(c *config.Config) error {
		c.Web = newSettings
		return nil
	}); err != nil {
		return WebResult{}, err
	}
	s = newSettings
	if err := e.syncVHosts(s); err != nil {
		e.rollbackWeb(previous)
		e.setWebState("", err.Error())
		return WebResult{}, err
	}
	portsChanged := s.HTTPPort != previous.HTTPPort || s.HTTPSPort != previous.HTTPSPort || s.Bind != previous.Bind
	e.applyWebToServer(s)
	if portsChanged {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		err := e.Web.Reconfigure(ctx)
		cancel()
		if err != nil {
			// Persistence succeeded; a port conflict only means the running
			// listeners keep the old bind. Surface it as a warning instead of
			// rolling back the user's explicit configuration.
			e.setWebState("los cambios de puerto se aplicarán al reiniciar: "+err.Error(), "")
		}
	}
	e.setWebState("", "")
	return WebResult{Web: e.WebStatus()}, nil
}
