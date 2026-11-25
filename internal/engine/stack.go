package engine

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// StackInfo is a read-only description of the technologies a parked project
// actually uses. Passwords and other secret values are never returned.
type StackInfo struct {
	Site             string   `json:"site"`
	Framework        string   `json:"framework,omitempty"`
	FrameworkVersion string   `json:"framework_version,omitempty"`
	Database         string   `json:"database,omitempty"`
	DatabaseName     string   `json:"database_name,omitempty"`
	DatabaseHost     string   `json:"database_host,omitempty"`
	DatabasePort     int      `json:"database_port,omitempty"`
	Cache            string   `json:"cache,omitempty"`
	WebRoot          string   `json:"web_root,omitempty"`
	FrontController  string   `json:"front_controller,omitempty"`
	PHP              string   `json:"php,omitempty"`
	Node             string   `json:"node,omitempty"`
	Server           string   `json:"server,omitempty"`
	Components       []string `json:"components,omitempty"`
	Error            string   `json:"error,omitempty"`
}

var (
	wpNameRe    = regexp.MustCompile(`define\(\s*['"]DB_NAME['"]\s*,\s*['"]([^'"]+)['"]`)
	wpHostRe    = regexp.MustCompile(`define\(\s*['"]DB_HOST['"]\s*,\s*['"]([^'"]+)['"]`)
	wpVersionRe = regexp.MustCompile(`\$wp_version\s*=\s*['"]([^'"]+)['"]`)
	joomlaValRe = regexp.MustCompile(`(?:public|protected|private)?\s*\$%s\s*=\s*['"]([^'"]*)['"]`)
	composerRe  = regexp.MustCompile(`"(laravel/framework|symfony/framework-bundle)"\s*:\s*"([^"]+)"`)
)

// StackControl gathers the stack of one site from its config files and the
// active Lair selections. It only reads files and never exposes credentials.
func (e *Engine) StackControl(ctx context.Context, name string) StackInfo {
	name = strings.ToLower(strings.TrimSuffix(strings.TrimSpace(name), "."))
	stack := StackInfo{Site: name, Server: "Lair web · PHP-FPM"}
	c, err := e.Config.View()
	if err != nil {
		stack.Error = err.Error()
		return stack
	}
	site, ok := c.Sites[name]
	if !ok || site.Path == "" {
		stack.Error = fmt.Sprintf("sitio desconocido %q", name)
		return stack
	}

	framework, docRoot, front := siteProfile(site.Path)
	stack.Framework, stack.WebRoot, stack.FrontController = framework, docRoot, front
	stack.PHP, stack.Node = c.Defaults["php"], c.Defaults["node"]
	if stack.PHP == "" {
		stack.PHP = site.PHP
	}
	if stack.Node == "" {
		stack.Node = site.Node
	}

	env, _, _ := readEnvFile(filepath.Join(site.Path, ".env"))
	if stack.Database = strings.TrimSpace(env["DB_CONNECTION"]); stack.Database != "" {
		stack.DatabaseName = strings.TrimSpace(env["DB_DATABASE"])
		stack.DatabaseHost = strings.TrimSpace(env["DB_HOST"])
		stack.DatabasePort, _ = strconv.Atoi(strings.TrimSpace(env["DB_PORT"]))
		stack.Cache = strings.TrimSpace(env["CACHE_DRIVER"])
		if stack.Cache == "redis" && strings.TrimSpace(env["REDIS_HOST"]) == "" {
			stack.Cache = "redis (localhost)"
		}
	}

	switch framework {
	case "wordpress":
		stack.readWordPress(site.Path)
	case "joomla":
		stack.readJoomla(site.Path)
	case "symfony", "laravel":
		stack.readComposer(site.Path)
	}

	components := []string{"PHP-FPM", "HTTPS"}
	if stack.Node != "" {
		components = append(components, "Node "+stack.Node)
	}
	if stack.Cache != "" {
		components = append(components, "caché "+stack.Cache)
	}
	if stack.Database != "" {
		components = append(components, "BD "+stack.Database)
	}
	stack.Components = components
	return stack
}

func (s *StackInfo) readWordPress(root string) {
	data, _ := os.ReadFile(filepath.Join(root, "wp-config.php"))
	text := string(data)
	if m := wpNameRe.FindStringSubmatch(text); len(m) == 2 {
		s.DatabaseName = m[1]
	}
	if m := wpHostRe.FindStringSubmatch(text); len(m) == 2 {
		s.DatabaseHost = m[1]
	}
	// The SQLite drop-in replaces MySQL entirely; no password is read.
	if regularFile(filepath.Join(root, "wp-content", "db.php")) {
		s.Database, s.Cache = "sqlite (drop-in oficial)", "object cache WordPress"
	} else if s.Database == "" {
		s.Database = "mysql"
	}
	if version, _ := os.ReadFile(filepath.Join(root, "wp-includes", "version.php")); len(version) > 0 {
		if m := wpVersionRe.FindStringSubmatch(string(version)); len(m) == 2 {
			s.FrameworkVersion = m[1]
		}
	}
	if prefix := joomlaValue(filepath.Join(root, "wp-config.php"), "table_prefix"); prefix != "" {
		s.Components = append(s.Components, "prefijo "+prefix)
	}
}

func (s *StackInfo) readJoomla(root string) {
	config := filepath.Join(root, "configuration.php")
	if !regularFile(config) {
		s.Database = "sin configurar (instalador)"
		return
	}
	s.Database = joomlaValue(config, "dbtype")
	if strings.Contains(s.Database, "mysqli") {
		s.Database = "mysql (MySQLi)"
	} else if s.Database != "" {
		s.Database += " (PDO)"
	}
	s.DatabaseName = joomlaValue(config, "db")
	// Joomla 6 renamed dbhost/dbuser to host/user. Only the host is exposed.
	s.DatabaseHost = joomlaValue(config, "host")
	if s.DatabaseHost == "" {
		s.DatabaseHost = joomlaValue(config, "dbhost")
	}
	if prefix := joomlaValue(config, "dbprefix"); prefix != "" {
		s.Components = append(s.Components, "prefijo "+prefix)
	}
	s.Cache = "Joomla cache"
	if data, _ := os.ReadFile(filepath.Join(root, "libraries", "src", "Version.php")); len(data) > 0 {
		text := string(data)
		if m := regexp.MustCompile(`RELEASE\s*=\s*['"]([^'"]+)['"]`).FindStringSubmatch(text); len(m) == 2 {
			s.FrameworkVersion = m[1]
		} else {
			major := joomlaVersionPart(text, "MAJOR_VERSION")
			minor := joomlaVersionPart(text, "MINOR_VERSION")
			patch := joomlaVersionPart(text, "PATCH_VERSION")
			if major != "" && minor != "" {
				s.FrameworkVersion = major + "." + minor
				if patch != "" {
					s.FrameworkVersion += "." + patch
				}
			}
		}
	}
}

func joomlaValue(path, key string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	re := regexp.MustCompile(fmt.Sprintf(joomlaValRe.String(), regexp.QuoteMeta(key)))
	if m := re.FindStringSubmatch(string(data)); len(m) == 2 {
		return m[1]
	}
	return ""
}

func joomlaVersionPart(text, key string) string {
	re := regexp.MustCompile(`(?:public\s+)?const\s+` + key + `\s*=\s*(\d+)`)
	if m := re.FindStringSubmatch(text); len(m) == 2 {
		return m[1]
	}
	return ""
}

func (s *StackInfo) readComposer(root string) {
	data, _ := os.ReadFile(filepath.Join(root, "composer.json"))
	if m := composerRe.FindStringSubmatch(string(data)); len(m) == 3 {
		s.FrameworkVersion = strings.TrimPrefix(m[2], "^")
		s.FrameworkVersion = strings.TrimPrefix(s.FrameworkVersion, "~")
	}
	if s.Database == "" {
		s.Database = "sin configurar"
	}
}
