package project

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// ValetSite is one symlink (or folder) found under ~/.valet/Sites. Existing
// says whether Lair already has a site with that name: the import is read-only
// and never registers, enables or disables a site by itself.
type ValetSite struct {
	Name     string `json:"name"`
	Host     string `json:"host"`
	Path     string `json:"path"`
	Existing bool   `json:"existing"`
}

// ValetImport is the read-only snapshot of a Laravel Valet installation.
type ValetImport struct {
	Domain string      `json:"domain,omitempty"`
	Paths  []string    `json:"paths,omitempty"`
	Sites  []ValetSite `json:"sites,omitempty"`
}

// valetConfig is the subset of ~/.valet/config.json Lair needs. Unknown keys
// are ignored so newer Valet releases stay compatible.
type valetConfig struct {
	Domain string   `json:"domain"`
	Paths  []string `json:"paths"`
}

var valetDomainRE = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]*[a-z0-9])?(?:\.[a-z0-9](?:[a-z0-9-]*[a-z0-9])?)*$`)

// ValetDir returns the directory of a Valet installation for the given home.
// An empty home resolves to the current user's home.
func ValetDir(home string) string {
	if home == "" {
		home, _ = os.UserHomeDir()
	}
	return filepath.Join(home, ".valet")
}

// ReadValet reads ~/.valet/config.json and ~/.valet/Sites without writing,
// executing `valet` or changing anything under ~/.valet. The optional
// registered map is consulted only to flag names Lair already knows.
func ReadValet(home string, registered map[string]bool) (ValetImport, error) {
	var out ValetImport
	root := ValetDir(home)
	data, err := os.ReadFile(filepath.Join(root, "config.json"))
	if err != nil {
		if os.IsNotExist(err) {
			return out, fmt.Errorf("no se encontró %s", filepath.Join(root, "config.json"))
		}
		return out, fmt.Errorf("leer config.json de Valet: %w", err)
	}
	var cfg valetConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return out, fmt.Errorf("parsear config.json de Valet: %w", err)
	}
	domain := strings.ToLower(strings.TrimSuffix(strings.TrimSpace(cfg.Domain), "."))
	if domain != "" && !valetDomainRE.MatchString(domain) {
		return out, fmt.Errorf("dominio de Valet no válido: %q", cfg.Domain)
	}
	out.Domain = domain
	seen := map[string]bool{}
	for _, p := range cfg.Paths {
		p = strings.TrimSpace(p)
		if p == "" || seen[p] {
			continue
		}
		if strings.HasPrefix(p, "~") {
			h := home
			if h == "" {
				h, _ = os.UserHomeDir()
			}
			p = filepath.Join(h, strings.TrimPrefix(p, "~"))
		}
		seen[p] = true
		out.Paths = append(out.Paths, p)
	}

	sitesRoot := filepath.Join(root, "Sites")
	entries, err := os.ReadDir(sitesRoot)
	if err != nil && !os.IsNotExist(err) {
		return out, fmt.Errorf("leer %s: %w", sitesRoot, err)
	}
	for _, entry := range entries {
		name := strings.TrimSpace(entry.Name())
		if name == "" || strings.HasPrefix(name, ".") {
			continue
		}
		// Valet links may be named `blog` or `blog.test`.
		host := strings.TrimSuffix(strings.ToLower(name), "."+domain)
		target := filepath.Join(sitesRoot, name)
		if resolved, err := filepath.EvalSymlinks(target); err == nil {
			target = resolved
		} else if info, statErr := os.Stat(target); statErr != nil || !info.IsDir() {
			continue // broken link or unrelated file: nothing to detect
		}
		out.Sites = append(out.Sites, ValetSite{
			Name: host, Host: host + "." + domain, Path: target,
			Existing: registered[host+"."+domain] || registered[host],
		})
	}
	return out, nil
}

// ValetOutput renders a human-readable summary of a read-only import.
func ValetOutput(v ValetImport, addedPaths []string, domainChanged bool) string {
	var b strings.Builder
	if domainChanged {
		fmt.Fprintf(&b, "Dominio importado: %s. ", v.Domain)
	}
	fmt.Fprintf(&b, "Carpetas importadas: %d. ", len(addedPaths))
	if len(v.Sites) == 0 {
		b.WriteString("No se encontraron sitios en ~/.valet/Sites.")
	} else {
		b.WriteString("Sitios detectados: ")
		for i, s := range v.Sites {
			if i > 0 {
				b.WriteString(", ")
			}
			status := "nuevo"
			if s.Existing {
				status = "ya registrado"
			}
			fmt.Fprintf(&b, "%s (%s)", s.Host, status)
		}
		b.WriteString(".")
	}
	return b.String()
}
