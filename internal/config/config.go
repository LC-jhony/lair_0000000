// Package config owns config.json. Only the daemon writes it; the CLI reads
// through the local API. Writes are atomic and the file is 0600.
package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

const CurrentVersion = 1

// Defaults of the three new local-development surfaces. They live in config
// so an older config.json (which has no keys for them) keeps working.
const (
	DefaultMailPort     = 2525
	DefaultDebuggerPort = 9003
)

// Config is the persistent state of an installation.
type Config struct {
	Version   int               `json:"version"`
	Installed map[string]string `json:"installed"` // "<pkg>/<version>" -> build
	Defaults  map[string]string `json:"defaults"`  // "php", "node", "<service>" -> version
	Ports     map[string]int    `json:"ports"`     // "<service>" -> active port
	Sites     map[string]Site   `json:"sites"`     // site name -> pinned versions
	// ProjectDirs is the list of folders scanned for projects. The first one
	// is the default parent for new projects and is mirrored in ProjectsDir.
	ProjectDirs []string `json:"project_dirs,omitempty"`
	// ProjectsDir is the derived, backwards-compatible view of ProjectDirs[0].
	ProjectsDir string           `json:"projects_dir,omitempty"`
	Web         WebSettings      `json:"web"`
	Mail        MailSettings     `json:"mail"`
	Dumps       DumpsSettings    `json:"dumps"`
	Debugger    DebuggerSettings `json:"debugger"`
}

// MailSettings configures the built-in loopback SMTP catcher.
type MailSettings struct {
	Port int `json:"port"`
}

// DefaultMail returns a fresh mail configuration. Port 2525 is unprivileged
// and the conventional choice for local mail catchers.
func DefaultMail() MailSettings { return MailSettings{Port: DefaultMailPort} }

// DefaultProjectsDir is the folder for new sites. It is called "Lair" (with
// an uppercase L) and lives under the user's home, never under Lair's own
// data root. Older releases used ~/lair and ~/Projects; those are migrated
// on load without deleting any project files.
func DefaultProjectsDir(home string) string {
	if home == "" {
		home, _ = os.UserHomeDir()
	}
	if home == "" {
		return "Lair"
	}
	return filepath.Join(home, "Lair")
}

// LegacyProjectsDirName is the lowercase folder used before ~/Lair.
const LegacyProjectsDirName = "lair"

// LegacyProjectDirs are the project folders used before ~/Lair became the
// default. Load replaces them with DefaultProjectsDir("") and keeps the
// projects on disk untouched.
var LegacyProjectDirs = []string{LegacyProjectsDirName, "Projects"}

// isLegacyProjectDir reports whether dir is one of the old default folders
// (compared against the user's home so an unrelated ~/Projects from another
// user is never mistaken for ours).
func isLegacyProjectDir(dir string, home string) bool {
	if strings.HasPrefix(dir, "~") && home != "" {
		dir = filepath.Join(home, strings.TrimPrefix(dir, "~"))
	}
	clean := filepath.Clean(dir)
	if home == "" {
		home, _ = os.UserHomeDir()
	}
	for _, name := range LegacyProjectDirs {
		if clean == filepath.Join(home, name) || clean == filepath.Join("~", name) || clean == name {
			return true
		}
	}
	return false
}

// DumpsSettings persists the capture switches of the PHP dumps agent.
// Configured is an internal marker: older configs without a "dumps" key get
// the defaults, while an explicitly saved all-false config stays all-false.
type DumpsSettings struct {
	Configured bool `json:"configured,omitempty"`
	Capturing  bool `json:"capturing"`
	Queries    bool `json:"queries"`
	Logs       bool `json:"logs"`
	Views      bool `json:"views"`
	Jobs       bool `json:"jobs"`
	Requests   bool `json:"requests"`
	ShowInPage bool `json:"show_in_page"`
}

// DefaultDumps captures the common types by default but leaves page injection
// off until the user opts in.
func DefaultDumps() DumpsSettings {
	return DumpsSettings{
		Configured: true,
		Queries:    true, Logs: true, Views: true, Jobs: true, Requests: true,
	}
}

// DebuggerSettings persists the two Lair-side Xdebug switches. The port is
// fixed at 9003 and the pool configuration itself comes from internal/php.
type DebuggerSettings struct {
	Detect      bool   `json:"detect"`
	Breakpoints bool   `json:"breakpoints"`
	Mode        string `json:"mode"`
}

// DefaultDebugger returns the default debugger configuration: detection off,
// no breakpoints and Xdebug in "debug" mode.
func DefaultDebugger() DebuggerSettings { return DebuggerSettings{Mode: "debug"} }

// WebSettings controls how *.test sites are served. HTTPS can be disabled
// without losing the sites; ports are configurable to work around local
// network conflicts. Defaults are applied on load for older config.json files.
type WebSettings struct {
	HTTPPort     int    `json:"http_port"`
	HTTPSPort    int    `json:"https_port"`
	HTTPS        bool   `json:"https"`
	RedirectHTTP bool   `json:"redirect_http"`
	Bind         string `json:"bind"`
	Domain       string `json:"domain"`
}

// DefaultWeb returns the default web layer: the embedded Go server on the
// conventional HTTP/HTTPS ports with HTTPS enabled using Lair's local CA.
// Privileged ports need CAP_NET_BIND_SERVICE (or setcap on the binary); any
// port can still be changed through the desktop's Lair configuration and Normalize.
func DefaultWeb() WebSettings {
	return WebSettings{
		HTTPPort:     80,
		HTTPSPort:    443,
		HTTPS:        true,
		RedirectHTTP: true,
		Bind:         "127.0.0.1",
		Domain:       "test",
	}
}

// Normalize fills zero values with defaults and validates the result.
func (w WebSettings) Normalize() (WebSettings, error) {
	d := DefaultWeb()
	if w.HTTPPort == 0 {
		w.HTTPPort = d.HTTPPort
	}
	if w.HTTPSPort == 0 {
		w.HTTPSPort = d.HTTPSPort
	}
	if w.HTTPPort < 1 || w.HTTPPort > 65535 || w.HTTPSPort < 1 || w.HTTPSPort > 65535 {
		return w, fmt.Errorf("web ports must be between 1 and 65535")
	}
	if w.HTTPPort == w.HTTPSPort {
		return w, fmt.Errorf("HTTP and HTTPS ports must differ")
	}
	if w.Bind == "" {
		w.Bind = d.Bind
	}
	if w.Bind != "127.0.0.1" && w.Bind != "0.0.0.0" && w.Bind != "localhost" {
		return w, fmt.Errorf("invalid bind address %q (use 127.0.0.1 or 0.0.0.0)", w.Bind)
	}
	if w.Domain == "" {
		w.Domain = d.Domain
	}
	return w, nil
}

// Site holds per-site version pins and aliases. Domains are extra host names
// rendered as server_name aliases in the generated virtual hosts; the map key
// remains the canonical name and is never duplicated here.
type Site struct {
	Path     string   `json:"path"`
	PHP      string   `json:"php,omitempty"`
	Node     string   `json:"node,omitempty"`
	Database string   `json:"database,omitempty"`
	Domains  []string `json:"domains,omitempty"`
	Enabled  bool     `json:"enabled,omitempty"`
}

// UnmarshalJSON defaults Enabled to true when the key is absent, so a
// config.json written before enable/disable existed keeps its sites active.
// A pointer is used to distinguish an explicit `false` from a missing key
// (which the omitempty tag alone cannot do for a bool).
func (s *Site) UnmarshalJSON(data []byte) error {
	type plain Site
	aux := struct {
		*plain
		Enabled *bool `json:"enabled"`
	}{plain: (*plain)(s)}
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}
	if aux.Enabled == nil {
		s.Enabled = true
	} else {
		s.Enabled = *aux.Enabled
	}
	return nil
}

// MarshalJSON always writes the enabled flag. Without this, an explicitly
// disabled site would be serialised without the key (because of omitempty) and
// Load would silently re-enable it on the next read.
func (s Site) MarshalJSON() ([]byte, error) {
	type plain Site
	return json.Marshal(struct {
		plain
		Enabled bool `json:"enabled"`
	}{plain: plain(s), Enabled: s.Enabled})
}

// New returns an initialised empty config.
func New() *Config {
	c := &Config{
		Version:   CurrentVersion,
		Installed: map[string]string{},
		Defaults:  map[string]string{},
		Ports:     map[string]int{},
		Sites:     map[string]Site{},
		Web:       DefaultWeb(),
		Mail:      DefaultMail(),
		Dumps:     DefaultDumps(),
		Debugger:  DefaultDebugger(),
	}
	c.Normalize()
	return c
}

// Normalize fills missing values and keeps the two project-dir fields in
// sync. ProjectDirs is authoritative: ProjectsDir is always derived from its
// first element, so older clients reading `projects_dir` keep working.
// Legacy ~/lair and ~/Projects entries are replaced by ~/Lair (the folders
// themselves are never deleted).
func (c *Config) Normalize() {
	if c.Installed == nil {
		c.Installed = map[string]string{}
	}
	if c.Defaults == nil {
		c.Defaults = map[string]string{}
	}
	if c.Ports == nil {
		c.Ports = map[string]int{}
	}
	if c.Sites == nil {
		c.Sites = map[string]Site{}
	}
	home, _ := os.UserHomeDir()
	def := DefaultProjectsDir(home)
	seen := map[string]bool{}
	dirs := make([]string, 0, len(c.ProjectDirs)+1)
	for _, dir := range c.ProjectDirs {
		dir = strings.TrimSpace(dir)
		if dir == "" || seen[dir] {
			continue
		}
		if strings.HasPrefix(dir, "~") && home != "" {
			dir = filepath.Join(home, strings.TrimPrefix(dir, "~"))
		}
		if isLegacyProjectDir(dir, home) {
			dir = def
		}
		if seen[dir] {
			continue
		}
		seen[dir] = true
		dirs = append(dirs, dir)
	}
	// A config written before project_dirs existed only has projects_dir.
	if old := strings.TrimSpace(c.ProjectsDir); old != "" && len(dirs) == 0 {
		if strings.HasPrefix(old, "~") && home != "" {
			old = filepath.Join(home, strings.TrimPrefix(old, "~"))
		}
		if !isLegacyProjectDir(old, home) {
			dirs = append(dirs, old)
		}
	}
	if len(dirs) == 0 {
		dirs = append(dirs, def)
	}
	c.ProjectDirs = dirs
	c.ProjectsDir = dirs[0]
}

// Load reads config.json, returning a fresh config when it does not exist.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return New(), nil
	}
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}
	var c Config
	if err := json.Unmarshal(data, &c); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	c.Normalize()
	// Per-site defaults (notably Enabled=true for legacy entries) are applied
	// by Site.UnmarshalJSON while decoding, so an explicit false survives.
	// Older config.json files have no web section at all; fill it in.
	if c.Web.HTTPPort == 0 {
		c.Web = DefaultWeb()
	}
	// Retro-compatible defaults: config.json files written before these
	// surfaces existed have zero values for all three structs.
	if c.Mail.Port == 0 {
		c.Mail = DefaultMail()
	}
	if !c.Dumps.Configured {
		c.Dumps = DefaultDumps()
	}
	if c.Debugger.Mode == "" {
		c.Debugger = DefaultDebugger()
	}
	return &c, nil
}

// Store serialises writes so concurrent jobs cannot interleave config.json.
type Store struct {
	path string
	mu   sync.Mutex
}

func NewStore(path string) *Store { return &Store{path: path} }

// Update applies fn to a fresh copy of the config under lock and saves it.
func (s *Store) Update(fn func(*Config) error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	c, err := Load(s.path)
	if err != nil {
		return err
	}
	if err := fn(c); err != nil {
		return err
	}
	c.Normalize()
	return s.save(c)
}

// View returns a read-only snapshot.
func (s *Store) View() (*Config, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return Load(s.path)
}

func (s *Store) save(c *Config) error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}
	buf, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(s.path), ".config-*")
	if err != nil {
		return err
	}
	name := tmp.Name()
	defer os.Remove(name)
	if _, err := tmp.Write(buf); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(name, s.path)
}
