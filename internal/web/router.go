// Package web is Lair's embedded HTTP server: it routes *.test hosts to the
// PHP-FPM unix socket for the site's PHP version, serving Laravel through
// FastCGI and terminating HTTPS with certificates from the local CA. It is the
// small, purpose-built web layer so no external proxy binary is needed.
package web

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"sort"
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
	DocRoot         string   `json:"doc_root,omitempty"`
	FrontController string   `json:"front_controller,omitempty"`
}

// DocumentRoot returns the directory PHP-FPM and static files must use. Older
// callers that never set DocRoot keep the historical Laravel layout.
func (s Site) DocumentRoot() string {
	if s.DocRoot != "" {
		return s.DocRoot
	}
	return filepath.Join(s.Root, "public")
}

// FrontFile returns the PHP entrypoint used for pretty URLs.
func (s Site) FrontFile() string {
	if s.FrontController != "" {
		return s.FrontController
	}
	return "index.php"
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

// List returns all sites sorted by name. Map iteration order is random, so an
// explicit sort keeps the Sites pane stable across reloads.
func (r *Router) List() []Site {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]Site, 0, len(r.sites))
	for _, s := range r.sites {
		out = append(out, s)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
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
	docRoot := site.DocumentRoot()
	urlPath := req.URL.Path
	if urlPath == "" {
		urlPath = "/"
	}
	clean := path.Clean("/" + urlPath)
	script := filepath.Join(docRoot, filepath.FromSlash(strings.TrimPrefix(clean, "/")))
	// Extension-less routes and directories use the framework front controller.
	// Files with an extension (e.g. style.css) keep their real path.
	base := path.Base(clean)
	scriptName, pathInfo := clean, ""
	if !strings.HasSuffix(clean, ".php") && !strings.Contains(base, ".") {
		// A project can have several front controllers: Joomla's installer
		// lives in installation/index.php, not in the root index.php. Prefer
		// the index.php inside the requested directory when it exists.
		dir := strings.TrimSuffix(clean, "/")
		nested := filepath.Join(docRoot, filepath.FromSlash(strings.TrimPrefix(dir, "/")), site.FrontFile())
		if info, err := os.Stat(nested); err == nil && !info.IsDir() {
			script = nested
			scriptName = path.Join("/", strings.TrimPrefix(dir, "/"), site.FrontFile())
		} else {
			script = filepath.Join(docRoot, site.FrontFile())
			// FastCGI requires an absolute SCRIPT_NAME. Frameworks derive
			// their base URL from it: a relative name makes the router resolve
			// paths against the wrong root and return a generic 500. The
			// pretty route belongs in PATH_INFO.
			scriptName, pathInfo = "/"+site.FrontFile(), clean
		}
	}
	method := req.Method
	if method == "" {
		method = http.MethodGet
	}
	params := map[string]string{
		"SCRIPT_FILENAME":   script,
		"SCRIPT_NAME":       scriptName,
		"DOCUMENT_ROOT":     docRoot,
		"REQUEST_METHOD":    method,
		"REQUEST_URI":       req.URL.RequestURI(),
		"QUERY_STRING":      req.URL.RawQuery,
		"SERVER_NAME":       serverName,
		"SERVER_PROTOCOL":   req.Proto,
		"GATEWAY_INTERFACE": "CGI/1.1",
		"REDIRECT_STATUS":   "200",
		// Laravel derives asset()/url() roots from the FastCGI request, not
		// from APP_URL. Without the real Host/port, built assets point to :443
		// while Lair serves HTTPS on 8443 and the page stays blank loading.
		"HTTP_HOST": req.Host,
	}
	// One id per HTTP request lets the dumps agent group queries into a real
	// request timeline. FPM workers are long-lived, so a per-process id would
	// merge unrelated requests; the web layer is the only place that sees the
	// start and end of each one.
	requestID := req.Header.Get("X-Lair-Request-Id")
	if requestID == "" {
		var b [8]byte
		if _, err := rand.Read(b[:]); err == nil {
			requestID = hex.EncodeToString(b[:])
		}
	}
	if requestID != "" {
		params["LAIR_DUMPS_REQUEST_ID"] = requestID
	}
	// PHP only populates $_POST for requests advertising their type and size.
	// Without these, install forms and any other POST reach the app with an
	// empty superglobal even though the stdin record carried the body.
	if contentType := req.Header.Get("Content-Type"); contentType != "" {
		params["CONTENT_TYPE"] = contentType
	}
	if req.ContentLength >= 0 {
		params["CONTENT_LENGTH"] = fmt.Sprintf("%d", req.ContentLength)
	}
	if pathInfo != "" {
		params["PATH_INFO"] = pathInfo
	}
	port := "80"
	if req.TLS != nil {
		port = "443"
		params["HTTPS"] = "on"
		params["REQUEST_SCHEME"] = "https"
	} else {
		params["REQUEST_SCHEME"] = "http"
	}
	if _, chosen, err := net.SplitHostPort(req.Host); err == nil {
		port = chosen
	}
	params["SERVER_PORT"] = port
	return params
}
