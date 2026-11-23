// Package web is Lair's embedded HTTP server: it routes *.test hosts to the
// PHP-FPM unix socket for the site's PHP version, serving Laravel through
// FastCGI and terminating HTTPS with certificates from the local CA. It is the
// small, purpose-built web layer so no external proxy binary is needed.
package web

import (
	"fmt"
	"net/http"
	"path"
	"path/filepath"
	"strings"
	"sync"
)

// Site describes one parked development site. The first fields are the stable
// routing contract; the rest are tolerant descriptive fields filled by the
// engine from config, .env and the filesystem. Clients may ignore them.
type Site struct {
	Name      string `json:"name"`       // canonical host, e.g. "acme.test"
	Root      string `json:"root"`       // project root (contains public/)
	PHPSocket string `json:"php_socket"` // unix socket of this site's FPM pool
	// PHPAddr is the TCP address of a php-cgi worker pool on Windows. When set,
	// the server dials TCP instead of a unix socket.
	PHPAddr  string `json:"php_addr,omitempty"`
	Secure   bool   `json:"secure"`  // a local CA certificate has been issued for this host
	Enabled  bool   `json:"enabled"` // false keeps the site registered but removes its vhost
	PHP      string `json:"php,omitempty"`
	Node     string `json:"node,omitempty"`
	Database string `json:"database,omitempty"`

	Type            string   `json:"type,omitempty"`
	Framework       string   `json:"framework,omitempty"`
	PHPVersion      string   `json:"php_version,omitempty"`
	PHPSource       string   `json:"php_source,omitempty"`
	PHPFile         string   `json:"php_file,omitempty"`
	NodeSource      string   `json:"node_source,omitempty"`
	NodeFile        string   `json:"node_file,omitempty"`
	RootState       string   `json:"root_state,omitempty"`
	RootSuggestions []string `json:"root_suggestions,omitempty"`
	Domains         []string `json:"domains,omitempty"`
	Problem         string   `json:"problem,omitempty"`
	Notice          string   `json:"notice,omitempty"`
	URL             string   `json:"url,omitempty"`
}

// Router maps request hosts to sites in a concurrency-safe way. `sites` is
// keyed by canonical name; `hosts` maps every registered host (canonical plus
// aliases) back to that name so aliases really route and can be removed again.
type Router struct {
	mu    sync.RWMutex
	sites map[string]Site
	hosts map[string]string
}

// NewRouter creates an empty router.
func NewRouter() *Router {
	return &Router{sites: map[string]Site{}, hosts: map[string]string{}}
}

func normalizeHost(host string) string {
	return strings.ToLower(strings.TrimSuffix(strings.TrimSpace(host), "."))
}

// Add registers or replaces a site and its aliases. A disabled site stays in
// the table (so the UI still sees it) but Match never routes to it.
func (r *Router) Add(site Site) error {
	if site.Name == "" || site.Root == "" || site.PHPSocket == "" {
		return fmt.Errorf("site name, root and php socket are required")
	}
	name := normalizeHost(site.Name)
	r.mu.Lock()
	defer r.mu.Unlock()
	for host, canonical := range r.hosts {
		if canonical == name {
			delete(r.hosts, host)
		}
	}
	r.sites[name] = site
	r.hosts[name] = name
	for _, alias := range site.Domains {
		if alias = normalizeHost(alias); alias != "" && alias != name {
			r.hosts[alias] = name
		}
	}
	return nil
}

// SetSecure flips only the TLS flag of an existing site, keeping its root and
// FPM socket untouched.
func (r *Router) SetSecure(name string, secure bool) {
	name = normalizeHost(name)
	r.mu.Lock()
	defer r.mu.Unlock()
	if site, ok := r.sites[name]; ok {
		site.Secure = secure
		r.sites[name] = site
	}
}

// Remove deletes a site.
func (r *Router) Remove(name string) {
	name = normalizeHost(name)
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.sites, name)
	for host, canonical := range r.hosts {
		if canonical == name || host == name {
			delete(r.hosts, host)
		}
	}
}

// List returns all sites sorted by name.
func (r *Router) List() []Site {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]Site, 0, len(r.sites))
	for _, s := range r.sites {
		out = append(out, s)
	}
	return out
}

// Match resolves a Host header (possibly with a port and subdomain) to a
// site. A subdomain like "www.acme.test" matches the parked "acme.test" and
// carries the subdomain in the SERVER_NAME param.
func (r *Router) Match(host string) (Site, string, bool) {
	host = normalizeHost(stripPort(host))
	r.mu.RLock()
	defer r.mu.RUnlock()
	name, found := r.hosts[host]
	if !found {
		// Walk up parent domains to find a parked site (www.<site>.test).
		labels := strings.Split(host, ".")
		for i := 1; i < len(labels); i++ {
			candidate := strings.Join(labels[i:], ".")
			if canonical, ok := r.hosts[candidate]; ok {
				name, found = canonical, true
				break
			}
		}
	}
	if !found {
		return Site{}, "", false
	}
	site, ok := r.sites[name]
	if !ok || !site.Enabled {
		// Disabled sites stay registered for the UI but are never published.
		return Site{}, "", false
	}
	return site, host, true
}

func stripPort(host string) string {
	if i := strings.LastIndex(host, ":"); i >= 0 {
		// Only strip when the suffix is all digits; IPv6 literals are not used
		// for *.test hosts.
		suffix := host[i+1:]
		digits := suffix != ""
		for _, c := range suffix {
			if c < '0' || c > '9' {
				digits = false
				break
			}
		}
		if digits {
			return host[:i]
		}
	}
	return host
}

// FastCGIParams builds the CGI environment for a request. It maps the URL path
// to <root>/public/<path>, falling back to index.php for directories and
// pretty URLs (Laravel front-controller pattern).
func FastCGIParams(site Site, serverName string, req *http.Request) map[string]string {
	docRoot := filepath.Join(site.Root, "public")
	urlPath := req.URL.Path
	if urlPath == "" {
		urlPath = "/"
	}
	clean := path.Clean("/" + urlPath)
	script := filepath.Join(docRoot, filepath.FromSlash(strings.TrimPrefix(clean, "/")))
	// Extension-less routes and directories use the Laravel front controller.
	// Files with an extension (e.g. style.css) keep their real path.
	base := path.Base(clean)
	if !strings.HasSuffix(clean, ".php") && !strings.Contains(base, ".") {
		script = filepath.Join(docRoot, "index.php")
	}
	method := req.Method
	if method == "" {
		method = http.MethodGet
	}
	params := map[string]string{
		"SCRIPT_FILENAME":   script,
		"SCRIPT_NAME":       strings.TrimPrefix(clean, "/"),
		"DOCUMENT_ROOT":     docRoot,
		"REQUEST_METHOD":    method,
		"REQUEST_URI":       req.URL.RequestURI(),
		"QUERY_STRING":      req.URL.RawQuery,
		"SERVER_NAME":       serverName,
		"SERVER_PROTOCOL":   req.Proto,
		"GATEWAY_INTERFACE": "CGI/1.1",
		"REDIRECT_STATUS":   "200",
	}
	if req.TLS != nil {
		params["HTTPS"] = "on"
		params["SERVER_PORT"] = "443"
	} else {
		params["SERVER_PORT"] = "80"
	}
	return params
}
