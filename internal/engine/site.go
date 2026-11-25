package engine

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/lair-sh/lair/internal/config"
	"github.com/lair-sh/lair/internal/php"
	"github.com/lair-sh/lair/internal/project"
)

// Site errors are exported so callers can classify failures (%w) without
// matching translated message text.
var (
	ErrInvalidSiteName = errors.New("invalid site name")
	ErrSiteNotFound    = errors.New("site not found")
	ErrInvalidEnvKey   = errors.New("invalid env key")
	ErrUnsafeSitePath  = errors.New("unsafe site path")
)

// SiteRequest is the single payload of the `site` action. Only `op` and `name`
// are always required; the remaining fields are used by their operation.
type SiteRequest struct {
	Op          string   `json:"op"`
	Name        string   `json:"name"`
	Key         string   `json:"key"`
	Value       string   `json:"value"`
	Domains     []string `json:"domains"`
	DeleteFiles bool     `json:"delete_files"`
	// Reveal is an explicit, per-key opt-in for reading a masked .env value.
	// It is never implied by `env` and is only honoured for one key per call.
	Reveal bool `json:"reveal,omitempty"`
	// Enabled is a pointer so a missing field can be told apart from an
	// explicit false on the set-enabled operation.
	Enabled *bool `json:"enabled,omitempty"`
}

// SiteResult carries the answer of every `site` operation. Exactly one field is
// populated per operation.
type SiteResult struct {
	Detail  *SiteDetail    `json:"detail,omitempty"`
	Env     []SiteEnvEntry `json:"env,omitempty"`
	Domains []string       `json:"domains,omitempty"`
	Output  string         `json:"output,omitempty"`
	URL     string         `json:"url,omitempty"`
	Config  string         `json:"config,omitempty"`
	// Value is populated only by an explicit `env` + `reveal` request for one
	// key. It is never included in detail/status/logs.
	Value string `json:"value,omitempty"`
}

// SiteDetail is a read-only view of one parked site assembled from config,
// .env and the filesystem. Secret values are never included.
type SiteDetail struct {
	Name       string `json:"name"`
	Path       string `json:"path"`
	URL        string `json:"url"`
	Enabled    bool   `json:"enabled"`
	Type       string `json:"type"`
	Framework  string `json:"framework"`
	PHP        string `json:"php"`
	PHPVersion string `json:"php_version"`
	PHPSource  string `json:"php_source"`
	PHPFile    string `json:"php_file"`
	Node       string `json:"node"`
	NodeSource string `json:"node_source"`
	NodeFile   string `json:"node_file"`

	Database     string `json:"database"`
	DatabaseHost string `json:"database_host"`
	DatabasePort int    `json:"database_port"`
	DatabaseName string `json:"database_name"`

	Secure          bool     `json:"secure"`
	Root            string   `json:"root"`
	RootState       string   `json:"root_state"`
	RootSuggestions []string `json:"root_suggestions"`
	Domains         []string `json:"domains"`
	Problem         string   `json:"problem"`
	Notice          string   `json:"notice"`
}

// SiteEnvEntry is one variable read from the site's .env. Secret values are
// masked: Value is always empty when Secret is true, while Set tells whether
// the key exists in the file at all.
type SiteEnvEntry struct {
	Key    string `json:"key"`
	Value  string `json:"value"`
	Secret bool   `json:"secret"`
	Set    bool   `json:"set"`
}

var (
	siteNameRE  = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9.-]*[a-z0-9])?$`)
	domainLabel = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]*[a-z0-9])?$`)
	envKeyRE    = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
)

// SiteControl implements the single daemon action used by desktop and CLI
// for per-site detail and maintenance. It never returns secret values.
func (e *Engine) SiteControl(ctx context.Context, req SiteRequest) (SiteResult, error) {
	if err := ctx.Err(); err != nil {
		return SiteResult{}, err
	}
	name := normalizeSiteName(req.Name)
	if name == "" {
		return SiteResult{}, fmt.Errorf("%w: %q", ErrInvalidSiteName, req.Name)
	}
	op := strings.ToLower(strings.TrimSpace(req.Op))
	if op == "" {
		op = "detail"
	}
	c, err := e.Config.View()
	if err != nil {
		return SiteResult{}, fmt.Errorf("leer configuración: %w", err)
	}
	site, ok := c.Sites[name]
	if !ok {
		return SiteResult{}, fmt.Errorf("%w: %s", ErrSiteNotFound, name)
	}

	switch op {
	case "detail":
		detail := e.siteDetail(c, name, site)
		return SiteResult{Detail: detail}, nil
	case "open":
		return e.siteOpen(name, site)
	case "set-enabled":
		if req.Enabled == nil {
			return SiteResult{}, fmt.Errorf("set-enabled requiere el campo enabled")
		}
		enabled := *req.Enabled
		if err := e.Config.Update(func(c *config.Config) error {
			registered := c.Sites[name]
			registered.Enabled = enabled
			c.Sites[name] = registered
			return nil
		}); err != nil {
			return SiteResult{}, fmt.Errorf("guardar estado de %s: %w", name, err)
		}
		// Rebuild the routing table and the generated vhosts so a disabled
		// site stops being served immediately. syncWeb removes its generated
		// server block and drops it from the keep set.
		e.reloadSites()
		e.syncWeb()
		updated, err := e.Config.View()
		if err != nil {
			return SiteResult{}, fmt.Errorf("leer configuración: %w", err)
		}
		return SiteResult{Detail: e.siteDetail(updated, name, updated.Sites[name])}, nil
	case "config":
		// Read-only vhost preview. Disabled sites are rendered too so the UI
		// can show what would be served if the site were re-enabled.
		text, err := e.SiteVHost(name)
		if err != nil {
			return SiteResult{}, err
		}
		return SiteResult{Config: text}, nil
	case "env":
		if req.Reveal {
			// Expose exactly one key, only when the caller asks for it. The
			// default `env` operation keeps masking secrets as before.
			key := strings.TrimSpace(req.Key)
			if !envKeyRE.MatchString(key) {
				return SiteResult{}, fmt.Errorf("%w: %q", ErrInvalidEnvKey, req.Key)
			}
			values, _, err := readEnvFile(filepath.Join(site.Path, ".env"))
			if err != nil {
				return SiteResult{}, err
			}
			return SiteResult{Value: values[key]}, nil
		}
		entries, err := siteEnvEntries(site.Path)
		if err != nil {
			return SiteResult{}, err
		}
		return SiteResult{Env: entries}, nil
	case "env-set":
		key := strings.TrimSpace(req.Key)
		if !envKeyRE.MatchString(key) {
			return SiteResult{}, fmt.Errorf("%w: %q", ErrInvalidEnvKey, req.Key)
		}
		// A newline in the value could inject extra KEY= lines into .env.
		if strings.ContainsAny(req.Value, "\r\n\x00") {
			return SiteResult{}, fmt.Errorf("%w: value contains a line break", ErrInvalidEnvKey)
		}
		envPath := filepath.Join(site.Path, ".env")
		if err := project.PatchEnvFile(envPath, map[string]string{key: req.Value}); err != nil {
			return SiteResult{}, fmt.Errorf("escribir %s: %w", envPath, err)
		}
		// Report only the key; the value must never be logged. Returning the
		// refreshed entry list keeps the desktop env table in sync, with
		// secrets masked exactly like the `env` operation.
		result := SiteResult{Output: key + " actualizado"}
		if entries, err := siteEnvEntries(site.Path); err == nil {
			result.Env = entries
		}
		return result, nil
	case "domains":
		// An omitted `domains` field is a read (the desktop loads it alongside
		// detail/env); an explicit [] clears the aliases.
		if req.Domains == nil {
			return SiteResult{Domains: append([]string(nil), site.Domains...)}, nil
		}
		domains, err := normalizeDomains(req.Domains)
		if err != nil {
			return SiteResult{}, err
		}
		if err := e.Config.Update(func(c *config.Config) error {
			registered := c.Sites[name]
			registered.Domains = domains
			c.Sites[name] = registered
			return nil
		}); err != nil {
			return SiteResult{}, fmt.Errorf("guardar dominios: %w", err)
		}
		e.reloadSites()
		e.syncWeb()
		return SiteResult{Domains: domains}, nil
	case "delete", "unlink":
		if err := e.deleteSite(name, site, req.DeleteFiles); err != nil {
			return SiteResult{}, err
		}
		return SiteResult{Output: fmt.Sprintf("sitio %s eliminado", name)}, nil
	default:
		return SiteResult{}, fmt.Errorf("unknown site operation %q", req.Op)
	}
}

// SiteVHost renders the site's server block with the current web settings and
// filesystem state. It never writes to disk and includes disabled sites so the
// desktop vhost preview keeps working while a site is turned off.
func (e *Engine) SiteVHost(name string) (string, error) {
	name = normalizeSiteName(name)
	if name == "" {
		return "", fmt.Errorf("%w: %q", ErrInvalidSiteName, name)
	}
	c, err := e.Config.View()
	if err != nil {
		return "", fmt.Errorf("leer configuración: %w", err)
	}
	site, ok := c.Sites[name]
	if !ok {
		return "", fmt.Errorf("%w: %s", ErrSiteNotFound, name)
	}
	s := e.WebSettings()
	ns, err := e.siteVHost(name, site, s)
	if err != nil {
		return "", err
	}
	return renderVHost(s, ns)
}

func normalizeSiteName(name string) string {
	name = strings.ToLower(strings.TrimSuffix(strings.TrimSpace(name), "."))
	if name == "" || !strings.Contains(name, ".") || strings.ContainsAny(name, "/\\ \t\n\r") {
		return ""
	}
	return name
}

// normalizeDomains validates aliases before they reach a generated vhost. An
// empty list clears the aliases; duplicates and the canonical name are dropped.
func normalizeDomains(values []string) ([]string, error) {
	out := []string{}
	seen := map[string]bool{}
	for _, value := range values {
		domain := strings.ToLower(strings.TrimSuffix(strings.TrimSpace(value), "."))
		if domain == "" {
			continue
		}
		if len(domain) > 253 || strings.Contains(domain, "..") {
			return nil, fmt.Errorf("dominio no válido: %q", value)
		}
		for _, label := range strings.Split(domain, ".") {
			if !domainLabel.MatchString(label) {
				return nil, fmt.Errorf("dominio no válido: %q", value)
			}
		}
		if seen[domain] {
			continue
		}
		seen[domain] = true
		out = append(out, domain)
	}
	if len(out) == 0 {
		return nil, nil
	}
	return out, nil
}

// deleteSite unlinks a site and, only when explicitly requested, removes its
// project files. Paths are always taken from the registry: a payload cannot
// point the deletion at another directory or inside LAIR_HOME.
func (e *Engine) deleteSite(name string, site config.Site, deleteFiles bool) error {
	registered := filepath.Clean(site.Path)
	if registered == "" || registered == "." || !filepath.IsAbs(registered) {
		return fmt.Errorf("%w: ruta del sitio no absoluta: %q", ErrUnsafeSitePath, site.Path)
	}
	if insidePath(e.Paths.Root, registered) {
		return fmt.Errorf("%w: %s está dentro de LAIR_HOME", ErrUnsafeSitePath, registered)
	}

	if deleteFiles {
		// A site's SQLite database lives inside its folder: snapshot it first
		// so a mistaken unlink with --delete-files can be undone.
		if err := e.autoSnapshotSite(context.Background(), name); err != nil {
			return err
		}
		info, err := os.Lstat(registered)
		if err != nil {
			return fmt.Errorf("estado de %s: %w", registered, err)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("%w: %s es un enlace simbólico", ErrUnsafeSitePath, registered)
		}
		if !info.IsDir() {
			return fmt.Errorf("%w: %s no es un directorio", ErrUnsafeSitePath, registered)
		}
		resolved, err := filepath.EvalSymlinks(registered)
		if err != nil {
			return fmt.Errorf("resolver %s: %w", registered, err)
		}
		if filepath.Clean(resolved) != registered {
			return fmt.Errorf("%w: %s resuelve a %s", ErrUnsafeSitePath, registered, resolved)
		}
		if insidePath(e.Paths.Root, resolved) {
			return fmt.Errorf("%w: %s resuelve dentro de LAIR_HOME", ErrUnsafeSitePath, resolved)
		}
		if err := os.RemoveAll(registered); err != nil {
			return fmt.Errorf("eliminar archivos de %s: %w", registered, err)
		}
	}

	if err := e.Unlink(name); err != nil {
		return fmt.Errorf("desvincular %s: %w", name, err)
	}
	for _, suffix := range []string{".pem", "-key.pem"} {
		path := filepath.Join(e.Paths.Certs, name+suffix)
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("eliminar certificado %s: %w", path, err)
		}
	}
	return nil
}

// insidePath reports whether child is parent or a descendant of parent.
func insidePath(parent, child string) bool {
	parent, child = filepath.Clean(parent), filepath.Clean(child)
	rel, err := filepath.Rel(parent, child)
	if err != nil {
		return false
	}
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator)))
}

// siteEnvEntries reads the site's .env and masks every secret-looking value.
// Missing files simply produce an empty list.
func siteEnvEntries(sitePath string) ([]SiteEnvEntry, error) {
	values, order, err := readEnvFile(filepath.Join(sitePath, ".env"))
	if err != nil {
		return nil, fmt.Errorf("leer .env: %w", err)
	}
	entries := make([]SiteEnvEntry, 0, len(order))
	for _, key := range order {
		entry := SiteEnvEntry{Key: key, Value: values[key], Set: true}
		if isSecretKey(key) {
			entry.Secret = true
			entry.Value = ""
		}
		entries = append(entries, entry)
	}
	return entries, nil
}

func isSecretKey(key string) bool {
	upper := strings.ToUpper(key)
	for _, needle := range []string{"PASSWORD", "KEY", "TOKEN", "SECRET"} {
		if strings.Contains(upper, needle) {
			return true
		}
	}
	return false
}

// readEnvFile parses simple KEY=value lines. Quotes around a value are
// removed; comments and malformed lines are ignored.
func readEnvFile(path string) (map[string]string, []string, error) {
	values := map[string]string{}
	order := []string{}
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return values, order, nil
	}
	if err != nil {
		return nil, nil, err
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") || !strings.Contains(line, "=") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		key := strings.TrimSpace(parts[0])
		if key == "" {
			continue
		}
		value := strings.TrimSpace(parts[1])
		if len(value) >= 2 && (value[0] == '"' && value[len(value)-1] == '"' || value[0] == '\'' && value[len(value)-1] == '\'') {
			value = value[1 : len(value)-1]
		}
		if _, seen := values[key]; !seen {
			order = append(order, key)
		}
		values[key] = value
	}
	return values, order, nil
}

// siteDetail assembles the descriptive view of a site without exposing any
// secret. It is shared by the `site detail` action and the status snapshot.
func (e *Engine) siteDetail(c *config.Config, name string, site config.Site) *SiteDetail {
	env, _, err := readEnvFile(filepath.Join(site.Path, ".env"))
	if err != nil {
		env = map[string]string{}
	}
	detect := detectSiteRoot(site.Path)
	typ, framework := detectProjectType(site.Path)
	secure := e.siteIsSecure(name, site)
	phpValue, phpSource, phpFile := e.phpSelection(c, site)
	nodeValue, nodeSource, nodeFile := e.nodeSelection(c, site)

	detail := &SiteDetail{
		Name: name, Path: site.Path,
		URL:     e.siteURL(name, secure, env),
		Enabled: site.Enabled,
		Type:    typ, Framework: framework,
		PHP: phpValue, PHPVersion: e.installedPHPVersion(phpValue),
		PHPSource: phpSource, PHPFile: phpFile,
		Node: nodeValue, NodeSource: nodeSource, NodeFile: nodeFile,
		Database:     strings.TrimSpace(site.Database),
		DatabaseHost: strings.TrimSpace(env["DB_HOST"]),
		DatabaseName: strings.TrimSpace(env["DB_DATABASE"]),
		Secure:       secure,
		Root:         detect.root, RootState: detect.state,
		RootSuggestions: detect.suggestions,
		Domains:         append([]string(nil), site.Domains...),
		Problem:         detect.problem, Notice: detect.notice,
	}
	if detail.Database == "" {
		detail.Database = strings.TrimSpace(env["DB_CONNECTION"])
	}
	if port, err := strconv.Atoi(strings.TrimSpace(env["DB_PORT"])); err == nil {
		detail.DatabasePort = port
	}
	return detail
}

// siteOpen prepares the browser URL for one site. Like Valet Deck, it checks
// that the host resolves before handing the URL to the UI, so a missing DNS
// setup produces an actionable error instead of a dead browser tab. It also
// refreshes APP_URL in the project's .env with the real web settings.
func (e *Engine) siteOpen(name string, site config.Site) (SiteResult, error) {
	host := strings.TrimSuffix(name, ".")
	// HTTPS is only offered when it is enabled for the site AND a certificate
	// actually exists; otherwise the link would open a TLS handshake failure.
	secure := e.siteIsSecure(name, site) && regularFile(filepath.Join(e.Paths.Certs, name+".pem"))
	s := e.WebSettings()
	scheme, port := "http", s.HTTPPort
	if secure && s.HTTPS {
		scheme, port = "https", s.HTTPSPort
	}
	url := scheme + "://" + host
	if port != 80 && port != 443 {
		url = fmt.Sprintf("%s://%s:%d", scheme, host, port)
	}

	if site.Path != "" {
		for _, file := range []string{".env", ".env.example"} {
			_ = project.PatchEnvFile(filepath.Join(site.Path, file), map[string]string{"APP_URL": url})
		}
	}

	// Do not gate the link on a Go-side DNS lookup: the system resolver and the
	// browser can disagree (systemd-resolved stub timeouts, DoH, cached NXDOMAIN).
	// Return the URL and let the browser report a real resolution failure.
	return SiteResult{URL: url, Output: url}, nil
}

func (e *Engine) siteURL(name string, secure bool, env map[string]string) string {
	// APP_URL is informational and can be stale after an HTTPS toggle. Follow
	// it only when its scheme matches the site's authoritative secure flag.
	if value := strings.TrimSpace(env["APP_URL"]); value != "" {
		if strings.HasPrefix(value, "https://") == secure {
			return value
		}
	}
	s := e.WebSettings()
	scheme, port := "http", s.HTTPPort
	if secure && s.HTTPS {
		scheme, port = "https", s.HTTPSPort
	}
	host := strings.TrimSuffix(name, ".")
	if port == 80 || port == 443 {
		return scheme + "://" + host
	}
	return fmt.Sprintf("%s://%s:%d", scheme, host, port)
}

func (e *Engine) installedPHPVersion(value string) string {
	if value == "" {
		return ""
	}
	installed, err := php.Installed(e.Paths)
	if err != nil {
		return ""
	}
	for _, info := range installed {
		if info.Minor == value || info.Version == value || strings.HasPrefix(info.Version, value+".") {
			return info.Version
		}
	}
	return ""
}

// phpSelection resolves the effective PHP line and records where it came from.
func (e *Engine) phpSelection(c *config.Config, site config.Site) (value, source, file string) {
	if site.PHP != "" {
		return site.PHP, "site", "config.json"
	}
	for _, name := range []string{".lairrc", ".flockrc"} {
		if v := readPin(filepath.Join(site.Path, name), "php"); v != "" {
			return v, name, name
		}
	}
	return c.Defaults["php"], "default", "config.json"
}

// nodeSelection resolves the effective Node version. .nvmrc and package.json
// are project-owned pins; the config default is the last resort.
func (e *Engine) nodeSelection(c *config.Config, site config.Site) (value, source, file string) {
	if site.Node != "" {
		return site.Node, "site", "config.json"
	}
	if data, err := os.ReadFile(filepath.Join(site.Path, ".nvmrc")); err == nil {
		for _, line := range strings.Split(string(data), "\n") {
			if line = strings.TrimSpace(line); line != "" && !strings.HasPrefix(line, "#") {
				return line, ".nvmrc", ".nvmrc"
			}
		}
	}
	if v := nodeFromPackage(site.Path); v != "" {
		return v, "package.json", "package.json"
	}
	return c.Defaults["node"], "default", "config.json"
}

func readPin(path, key string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	prefix := key + "="
	for _, line := range strings.Split(string(data), "\n") {
		if line = strings.TrimSpace(line); strings.HasPrefix(line, prefix) {
			return strings.TrimSpace(strings.TrimPrefix(line, prefix))
		}
	}
	return ""
}

func nodeFromPackage(sitePath string) string {
	data, err := os.ReadFile(filepath.Join(sitePath, "package.json"))
	if err != nil {
		return ""
	}
	var doc struct {
		Engines struct {
			Node string `json:"node"`
		} `json:"engines"`
	}
	if json.Unmarshal(data, &doc) != nil {
		return ""
	}
	return strings.TrimSpace(doc.Engines.Node)
}

func detectProjectType(sitePath string) (typ, framework string) {
	if regularFile(filepath.Join(sitePath, "wp-config.php")) {
		return "php", "wordpress"
	}
	if regularFile(filepath.Join(sitePath, "configuration.php")) ||
		(directoryExists(filepath.Join(sitePath, "installation")) && directoryExists(filepath.Join(sitePath, "administrator"))) {
		return "php", "joomla"
	}
	if regularFile(filepath.Join(sitePath, "spark")) || regularFile(filepath.Join(sitePath, "app", "Config", "Database.php")) {
		return "php", "codeigniter"
	}
	if regularFile(filepath.Join(sitePath, "artisan")) {
		return "laravel", "laravel"
	}
	if regularFile(filepath.Join(sitePath, "composer.json")) {
		typ = "php"
		data, _ := os.ReadFile(filepath.Join(sitePath, "composer.json"))
		text := strings.ToLower(string(data))
		switch {
		case strings.Contains(text, `"laravel/framework"`):
			typ, framework = "laravel", "laravel"
		case strings.Contains(text, `"symfony/framework-bundle"`):
			framework = "symfony"
		}
	}
	if regularFile(filepath.Join(sitePath, "package.json")) {
		if typ == "" {
			typ = "node"
		}
		data, _ := os.ReadFile(filepath.Join(sitePath, "package.json"))
		text := strings.ToLower(string(data))
		for _, name := range []string{"sveltekit", "next", "nuxt", "react", "vue", "svelte"} {
			if strings.Contains(text, `"`+name+`"`) {
				framework = name
				break
			}
		}
	}
	return typ, framework
}

// siteProfile detects the document root and front controller for supported
// PHP projects. WordPress and Joomla serve from the project root; Symfony and
// CodeIgniter can use public/ or web/ depending on the generation.
func siteProfile(sitePath string) (framework, docRoot, front string) {
	docRoot = filepath.Join(sitePath, "public")
	front = "index.php"
	if regularFile(filepath.Join(sitePath, "artisan")) {
		return "laravel", docRoot, front
	}
	if regularFile(filepath.Join(sitePath, "wp-config.php")) {
		return "wordpress", sitePath, "index.php"
	}
	// Before installation Joomla has no configuration.php; its installer and
	// administrator directories are already present at the project root.
	if regularFile(filepath.Join(sitePath, "configuration.php")) ||
		(directoryExists(filepath.Join(sitePath, "installation")) && directoryExists(filepath.Join(sitePath, "administrator"))) {
		return "joomla", sitePath, "index.php"
	}
	if regularFile(filepath.Join(sitePath, "spark")) || regularFile(filepath.Join(sitePath, "app", "Config", "Database.php")) {
		if regularFile(filepath.Join(sitePath, "public", "index.php")) {
			return "codeigniter", docRoot, front
		}
		return "codeigniter", sitePath, "index.php"
	}
	if regularFile(filepath.Join(sitePath, "composer.json")) {
		data, _ := os.ReadFile(filepath.Join(sitePath, "composer.json"))
		if strings.Contains(strings.ToLower(string(data)), `"symfony/framework-bundle"`) {
			if regularFile(filepath.Join(sitePath, "public", "index.php")) {
				return "symfony", docRoot, front
			}
			candidate := filepath.Join(sitePath, "web", "app.php")
			if regularFile(candidate) {
				return "symfony", filepath.Join(sitePath, "web"), "app.php"
			}
			return "symfony", filepath.Join(sitePath, "web"), "index.php"
		}
	}
	return "", docRoot, front
}

type rootDetection struct {
	root        string
	state       string
	suggestions []string
	problem     string
	notice      string
}

// detectSiteRoot looks for the PHP front controller. Both the standard
// public/index.php and a project-root index.php are accepted; the latter gets
// an actionable suggestion because nginx roots at <site>/public.
func detectSiteRoot(sitePath string) rootDetection {
	public := filepath.Join(sitePath, "public")
	detected := rootDetection{root: public, state: "missing"}
	if sitePath == "" {
		detected.problem = "El sitio no tiene una ruta registrada"
		detected.suggestions = []string{"Registra la ruta con 'lair link'."}
		return detected
	}
	info, err := os.Lstat(sitePath)
	if err != nil || !info.IsDir() {
		detected.problem = "La carpeta del sitio no existe: " + sitePath
		detected.suggestions = []string{"Crea la carpeta o vuelve a enlazar el sitio."}
		return detected
	}
	if info.Mode()&os.ModeSymlink != 0 {
		detected.problem = "La ruta del sitio es un enlace simbólico"
		detected.suggestions = []string{"Enlaza la ruta real del proyecto."}
		return detected
	}
	if regularFile(filepath.Join(public, "index.php")) {
		detected.state = "ok"
		return detected
	}
	if regularFile(filepath.Join(sitePath, "index.php")) {
		detected.state = "ok"
		detected.root = sitePath
		// WordPress, Joomla and CodeIgniter serve legitimately from the root;
		// the public/ suggestion only applies to Laravel-style projects.
		framework, _, _ := siteProfile(sitePath)
		if framework == "" {
			detected.notice = "index.php está en la raíz; este proyecto espera public/index.php"
			detected.suggestions = []string{"Mueve index.php a public/ o ajusta la raíz del documento."}
		}
		return detected
	}
	entries, err := os.ReadDir(sitePath)
	if err == nil && len(entries) == 0 {
		detected.state = "empty"
		detected.notice = "La carpeta del sitio está vacía"
		detected.suggestions = []string{"Crea un proyecto Laravel aquí o añade public/index.php."}
		return detected
	}
	detected.problem = "No se encontró public/index.php ni index.php"
	detected.suggestions = []string{"Crea el front controller PHP en public/index.php."}
	return detected
}

func regularFile(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular()
}

func directoryExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}
