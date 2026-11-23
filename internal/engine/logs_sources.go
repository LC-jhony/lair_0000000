package engine

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/lair-sh/lair/internal/services"
)

// LogSource is one discoverable group of log files. `path` is the directory
// and `files` are names inside it, so a client can either show the list or
// read a file directly. Site files live in the project's storage/logs dir;
// service files live in LAIR_HOME/log and work with the existing `logs` action.
type LogSource struct {
	Kind  string   `json:"kind"` // "site" | "service"
	ID    string   `json:"id"`
	Name  string   `json:"name"`
	Path  string   `json:"path"`
	Files []string `json:"files"`
}

// ListLogSources discovers logs of parked sites and of Lair's own services.
// It never fails because a site or a service has no log yet.
func (e *Engine) ListLogSources() ([]LogSource, error) {
	c, err := e.Config.View()
	if err != nil {
		return nil, err
	}
	sources := []LogSource{}

	names := make([]string, 0, len(c.Sites))
	for name := range c.Sites {
		names = append(names, name)
	}
	sort.Strings(names)

	phpLogs := []string{}
	for _, name := range names {
		site := c.Sites[name]
		if site.Path == "" {
			continue
		}
		dir := filepath.Join(site.Path, "storage", "logs")
		files := logFiles(dir)
		if len(files) > 0 {
			sources = append(sources, LogSource{
				Kind: "site", ID: name, Name: name, Path: dir, Files: files,
			})
		}
		phpLogs = append(phpLogs, name+"-php.log")
	}

	// Service logs use the existing `logs` action, which reads from Paths.Log.
	if files := logFiles(e.Paths.Log, "lair.log", "daemon.log", "*-install.log"); len(files) > 0 {
		sources = append(sources, LogSource{
			Kind: "service", ID: "daemon", Name: "Lair daemon", Path: e.Paths.Log, Files: files,
		})
	}
	if files := logFiles(e.Paths.Log, "nginx-error.log", "nginx-access.log"); len(files) > 0 {
		sources = append(sources, LogSource{
			Kind: "service", ID: "nginx", Name: "nginx", Path: e.Paths.Log, Files: files,
		})
	}
	if files := append(logFiles(e.Paths.Log, "php-fpm.log"), logFiles(e.Paths.Log, phpLogs...)...); len(files) > 0 {
		sort.Strings(files)
		sources = append(sources, LogSource{
			Kind: "service", ID: "php-fpm", Name: "PHP-FPM (sitios)", Path: e.Paths.Log, Files: dedupe(files),
		})
	}
	// The supervisor writes <id>.log for every service it runs. Expose those
	// files so database/cache logs are reachable from the Logs pane.
	for _, id := range services.IDs() {
		if files := logFiles(e.Paths.Log, id+".log"); len(files) > 0 {
			sources = append(sources, LogSource{
				Kind: "service", ID: id, Name: serviceLabel(id), Path: e.Paths.Log, Files: files,
			})
		}
	}
	return sources, nil
}

func serviceLabel(id string) string {
	switch id {
	case "postgres":
		return "PostgreSQL"
	case "mariadb":
		return "MariaDB"
	case "mongodb":
		return "MongoDB"
	case "mysql":
		return "MySQL"
	case "redis":
		return "Redis"
	case "valkey":
		return "Valkey"
	case "meilisearch":
		return "Meilisearch"
	case "typesense":
		return "Typesense"
	case "rustfs":
		return "RustFS"
	}
	return id
}

// logFiles returns the requested names that exist as regular .log files. An
// empty pattern list means "all .log files in the directory".
func logFiles(dir string, patterns ...string) []string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return []string{}
	}
	out := []string{}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".log") {
			continue
		}
		if len(patterns) == 0 {
			out = append(out, entry.Name())
			continue
		}
		for _, pattern := range patterns {
			if pattern == "" {
				continue
			}
			if strings.Contains(pattern, "*") {
				if ok, _ := filepath.Match(pattern, entry.Name()); ok {
					out = append(out, entry.Name())
					break
				}
			} else if pattern == entry.Name() {
				out = append(out, entry.Name())
				break
			}
		}
	}
	sort.Strings(out)
	return dedupe(out)
}

func dedupe(in []string) []string {
	out := make([]string, 0, len(in))
	seen := map[string]bool{}
	for _, v := range in {
		if !seen[v] {
			seen[v] = true
			out = append(out, v)
		}
	}
	return out
}
