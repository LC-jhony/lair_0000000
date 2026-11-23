package web

import (
	"context"
	"crypto/tls"
	"fmt"
	"html"
	"io"
	"net"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/lair-sh/lair/internal/fastcgi"
	"github.com/lair-sh/lair/internal/pki"
	"github.com/lair-sh/lair/internal/services"
)

// Server is the embedded web server. It serves plain HTTP on :80 and HTTPS on
// :443, both bound to 127.0.0.1. Sites are resolved by Host header and proxied
// to their PHP-FPM unix socket.
type Server struct {
	Router    *Router
	CertsDir  string
	Domain    string // e.g. "test"
	HTTPPort  int
	HTTPSPort int
	Bind      string // 127.0.0.1 (default) or 0.0.0.0
	Timeout   time.Duration

	// FPMDial is injectable for tests; defaults to net.DialTimeout.
	FPMDial func(network, addr string, timeout time.Duration) (net.Conn, error)

	httpServer  *http.Server
	httpsServer *http.Server

	// cfgMu protects the listener settings, which may change at runtime via
	// WebControl. Start and getCertificate always read a snapshot.
	// overridden is set when SetConfig is called before Start, e.g. by CLI
	// flags. StartWeb must honour those values instead of re-reading config.json.
	overridden bool

	cfgMu sync.RWMutex

	certMu sync.Mutex
	certs  map[string]*tls.Certificate // canonical host -> certificate
}

// NewServer creates a web server with default ports 80/443.
func NewServer(certsDir, domain string) *Server {
	if domain == "" {
		domain = "test"
	}
	return &Server{
		Router: NewRouter(), CertsDir: certsDir, Domain: domain,
		HTTPPort: 80, HTTPSPort: 443, Bind: "127.0.0.1", Timeout: 10 * time.Second,
		certs: map[string]*tls.Certificate{},
	}
}

// Config returns a consistent snapshot of the listener settings.
func (s *Server) Config() (httpPort, httpsPort int, bind, domain string) {
	s.cfgMu.RLock()
	defer s.cfgMu.RUnlock()
	return s.HTTPPort, s.HTTPSPort, s.Bind, s.Domain
}

// SetConfig replaces the listener settings. It does not touch running
// listeners; call Reconfigure to apply changes to ports or bind address.
func (s *Server) SetConfig(httpPort, httpsPort int, bind, domain string) {
	s.cfgMu.Lock()
	defer s.cfgMu.Unlock()
	s.HTTPPort, s.HTTPSPort, s.Bind, s.Domain = httpPort, httpsPort, bind, domain
	s.overridden = true
}

// HasOverrides reports whether listener settings were injected before start.
func (s *Server) HasOverrides() bool {
	s.cfgMu.RLock()
	defer s.cfgMu.RUnlock()
	return s.overridden
}

// ClearCertCache drops cached certificates after site aliases change, so the
// next handshake issues a certificate containing the new SANs.
func (s *Server) ClearCertCache() {
	s.certMu.Lock()
	defer s.certMu.Unlock()
	s.certs = map[string]*tls.Certificate{}
}

// Reconfigure stops the current listeners and starts them with new settings.
// It is the implementation behind the daemon's "hot reload" of web settings.
func (s *Server) Reconfigure(ctx context.Context) error {
	if err := s.Shutdown(ctx); err != nil {
		return err
	}
	return s.Start()
}

// Handler implements http.Handler.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	site, serverName, ok := s.Router.Match(r.Host)
	if !ok {
		http.Error(w, "lair: unknown site "+r.Host, http.StatusNotFound)
		return
	}
	// A site whose front controller is missing or misplaced is served a small
	// status page instead of being proxied to a broken FPM pool.
	if site.RootState != "" && site.RootState != "ok" {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusNotFound)
		_, _ = io.WriteString(w, notConfiguredPage(site))
		return
	}

	// Static files in public/ are served directly. Only dynamic routes reach
	// PHP-FPM. Requests are rejected if the cleaned path escapes public/.
	docRoot := filepath.Join(site.Root, "public")
	if r.Method == http.MethodGet || r.Method == http.MethodHead {
		if rel := strings.TrimPrefix(path.Clean("/"+r.URL.Path), "/"); rel != "" {
			// Every .php file is a dynamic route, not a static asset. Serving
			// public/foo.php as text would leak its source. index.php and any
			// other front controller must reach PHP-FPM.
			if !strings.EqualFold(filepath.Ext(rel), ".php") {
				if file, ok := safeStaticFile(docRoot, rel); ok {
					http.ServeFile(w, r, file)
					return
				}
			}
		}
	}

	dial := s.FPMDial
	if dial == nil {
		dial = net.DialTimeout
	}
	network, addr := "unix", site.PHPSocket
	if site.PHPAddr != "" {
		network, addr = "tcp", site.PHPAddr
	}
	conn, err := dial(network, addr, s.timeout())
	if err != nil {
		http.Error(w, "lair: PHP-FPM unavailable: "+err.Error(), http.StatusBadGateway)
		return
	}
	defer conn.Close()

	client := &fastcgi.Client{Conn: conn, Timeout: s.timeout()}
	params := FastCGIParams(site, serverName, r)
	body, _ := io.ReadAll(io.LimitReader(r.Body, 64<<20))
	resp, err := client.Do(fastcgi.Request{Params: params, Body: body})
	if err != nil {
		http.Error(w, "lair: FastCGI error: "+err.Error(), http.StatusBadGateway)
		return
	}
	for k, values := range resp.Header {
		for _, v := range values {
			w.Header().Add(k, v)
		}
	}
	if w.Header().Get("Content-Type") == "" {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
	}
	w.WriteHeader(resp.Status)
	_, _ = w.Write(resp.Body)
	// PHP-FPM stderr is deliberately not written to the HTTP response: it can
	// contain credentials, absolute paths and stack traces.
}

// notConfiguredPage is the built-in status page for a parked site that has no
// front controller yet. It only shows the host and the expected project path:
// no LAIR_HOME paths, tokens or environment values.
func notConfiguredPage(site Site) string {
	host := html.EscapeString(site.Name)
	expected := html.EscapeString(filepath.Join(site.Root, "public", "index.php"))
	return `<!doctype html><html lang="es"><head><meta charset="utf-8">` +
		`<meta name="viewport" content="width=device-width,initial-scale=1">` +
		`<title>Sitio no configurado</title></head><body>` +
		`<h1>Sitio no configurado</h1>` +
		`<p>El host <strong>` + host + `</strong> está registrado en Lair, pero todavía no tiene una aplicación PHP accesible.</p>` +
		`<p>Ruta esperada: <code>` + expected + `</code></p>` +
		`</body></html>`
}

// safeStaticFile resolves a requested relative path under docRoot. Both the
// document root and the target are symlink-resolved, so a link inside a site
// may point to another file of the same site but never outside it (where it
// could expose a sibling project or Lair's private CA key).
func safeStaticFile(docRoot, rel string) (string, bool) {
	candidate := filepath.Join(docRoot, filepath.FromSlash(rel))
	if !within(docRoot, candidate) {
		return "", false
	}
	resolvedRoot, err := filepath.EvalSymlinks(docRoot)
	if err != nil {
		return "", false
	}
	info, err := os.Lstat(candidate)
	if err != nil || info.IsDir() {
		return "", false
	}
	resolved, err := filepath.EvalSymlinks(candidate)
	if err != nil {
		return "", false
	}
	if !within(resolvedRoot, resolved) {
		return "", false
	}
	info, err = os.Stat(resolved)
	if err != nil || !info.Mode().IsRegular() {
		return "", false
	}
	return resolved, true
}

// within reports whether file is inside base after cleaning (no traversal).
func within(base, file string) bool {
	rel, err := filepath.Rel(base, file)
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator))
}

func (s *Server) timeout() time.Duration {
	if s.Timeout > 0 {
		return s.Timeout
	}
	return 10 * time.Second
}

// getCertificate issues (on demand) and loads a certificate for a parked site.
// The SNI host is only used to find the site; the certificate is always issued
// for the canonical name from the router, so an arbitrary subdomain cannot
// obtain a certificate naming a host Lair does not own. Certificates are
// cached per canonical host to avoid re-reading/re-signing on every handshake.
func (s *Server) getCertificate(hello *tls.ClientHelloInfo) (*tls.Certificate, error) {
	host := pki.NormalizeHost(stripPort(hello.ServerName))
	_, _, _, configuredDomain := s.Config()
	domain := pki.NormalizeHost(configuredDomain)
	if host == "" || domain == "" || (host != domain && !strings.HasSuffix(host, "."+domain)) {
		return nil, fmt.Errorf("refusing to issue certificate for %q", host)
	}

	// Re-check the router on every handshake, even for a cached host: an
	// unlinked site must stop being served over TLS immediately.
	site, _, ok := s.Router.Match(host)
	if !ok {
		return nil, fmt.Errorf("refusing to issue certificate for unparked host %q", host)
	}
	canonical := pki.NormalizeHost(site.Name)

	s.certMu.Lock()
	defer s.certMu.Unlock()
	if s.certs == nil {
		s.certs = map[string]*tls.Certificate{}
	}
	// Only the canonical host is cached: caching arbitrary SNI values lets any
	// subdomain grow the map without bound.
	if cert := s.certs[canonical]; cert != nil {
		return cert, nil
	}
	sans := pki.SANsFor(canonical, domain)
	if len(sans) == 0 {
		return nil, fmt.Errorf("invalid parked site %q for domain %q", site.Name, domain)
	}
	seen := make(map[string]bool, len(sans)+len(site.Domains))
	for _, name := range sans {
		seen[pki.NormalizeHost(name)] = true
	}
	// Aliases registered on the site must be part of the certificate, otherwise
	// HTTPS on a configured domain fails with a hostname mismatch.
	for _, alias := range site.Domains {
		if alias = pki.NormalizeHost(alias); alias != "" && !seen[alias] {
			sans = append(sans, alias)
			seen[alias] = true
		}
	}
	certPath, keyPath, err := pki.IssueSite(s.CertsDir, canonical, sans...)
	if err != nil {
		return nil, err
	}
	cert, err := tls.LoadX509KeyPair(certPath, keyPath)
	if err != nil {
		return nil, err
	}
	s.certs[canonical] = &cert
	return &cert, nil
}

// Start begins serving on both ports. It returns after the listeners are
// created; use Shutdown to stop. The bind address comes from config (loopback
// by default); 0.0.0.0 is an explicit opt-in that exposes sites to the LAN.
func (s *Server) Start() error {
	httpPort, httpsPort, configuredBind, _ := s.Config()
	if c := services.CheckPort(httpPort); c != nil {
		return fmt.Errorf("cannot start HTTP: %s", c.String())
	}
	if c := services.CheckPort(httpsPort); c != nil {
		return fmt.Errorf("cannot start HTTPS: %s", c.String())
	}
	bind := strings.TrimSpace(configuredBind)
	if bind == "" {
		bind = "127.0.0.1"
	}
	if bind != "127.0.0.1" && bind != "0.0.0.0" && bind != "localhost" {
		return fmt.Errorf("invalid bind address %q", bind)
	}
	s.httpServer = &http.Server{
		Addr:              net.JoinHostPort(bind, strconv.Itoa(httpPort)),
		Handler:           s,
		ReadHeaderTimeout: 10 * time.Second,
	}
	s.httpsServer = &http.Server{
		Addr:              net.JoinHostPort(bind, strconv.Itoa(httpsPort)),
		Handler:           s,
		ReadHeaderTimeout: 10 * time.Second,
		TLSConfig: &tls.Config{
			MinVersion:     tls.VersionTLS12,
			GetCertificate: s.getCertificate,
		},
	}
	errCh := make(chan error, 2)
	if httpPort > 0 {
		go func() {
			if err := s.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				errCh <- err
			}
		}()
	}
	if httpsPort > 0 {
		go func() {
			if err := s.httpsServer.ListenAndServeTLS("", ""); err != nil && err != http.ErrServerClosed {
				errCh <- err
			}
		}()
	}
	select {
	case err := <-errCh:
		return err
	default:
		return nil
	}
}

// Shutdown gracefully stops both servers.
func (s *Server) Shutdown(ctx context.Context) error {
	var first error
	if s.httpServer != nil {
		if err := s.httpServer.Shutdown(ctx); err != nil {
			first = err
		}
	}
	if s.httpsServer != nil {
		if err := s.httpsServer.Shutdown(ctx); err != nil && first == nil {
			first = err
		}
	}
	return first
}

// PortsInUse reports conflicts on 80/443 before any replacement happens.
func PortsInUse() ([]*services.PortConflict, error) {
	out := []*services.PortConflict{}
	for _, p := range []int{80, 443} {
		if c := services.CheckPort(p); c != nil {
			out = append(out, c)
		}
	}
	return out, nil
}
