package engine

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

// TableInfo is a row count for one well-known Laravel table. No row values are
// returned: this is a read-only size overview for the site detail pane.
type TableInfo struct {
	Name  string `json:"name"`
	Rows  int    `json:"rows,omitempty"`
	Error string `json:"error,omitempty"`
}

// SiteData is an aggregate, secret-free data overview of one parked site.
type SiteData struct {
	Site      string      `json:"site"`
	Tables    []TableInfo `json:"tables,omitempty"`
	CacheKeys int         `json:"cache_keys,omitempty"`
	Mail      int         `json:"mail,omitempty"`
	Error     string      `json:"error,omitempty"`
}

var dataTables = []string{"users", "sessions", "jobs", "failed_jobs", "migrations", "cache"}

// SiteData reads only counts from the site's configured database and cache.
func (e *Engine) SiteData(ctx context.Context, name string) SiteData {
	name = strings.ToLower(strings.TrimSuffix(strings.TrimSpace(name), "."))
	data := SiteData{Site: name}
	c, err := e.Config.View()
	if err != nil {
		data.Error = err.Error()
		return data
	}
	site, ok := c.Sites[name]
	if !ok || site.Path == "" {
		data.Error = fmt.Sprintf("sitio desconocido %q", name)
		return data
	}
	env, _, _ := readEnvFile(filepath.Join(site.Path, ".env"))
	driver := strings.TrimSpace(env["DB_CONNECTION"])
	if driver == "" {
		driver = site.Database
	}
	if driver == "" {
		driver = "mariadb"
	}
	db := strings.TrimSpace(env["DB_DATABASE"])
	user := strings.TrimSpace(env["DB_USERNAME"])
	password := env["DB_PASSWORD"]
	id := e.databaseServiceID(driver)
	spec, err := e.ServiceSpec(id)
	if err != nil {
		data.Error = err.Error()
		return data
	}
	port := c.Ports[id]
	if raw := strings.TrimSpace(env["DB_PORT"]); raw != "" {
		if parsed, convErr := strconv.Atoi(raw); convErr == nil && parsed > 0 {
			port = parsed
		}
	}
	if port == 0 {
		port = spec.Port
	}
	for _, table := range dataTables {
		info := TableInfo{Name: table}
		var out []byte
		if id == "postgres" || driver == "pgsql" {
			if user == "" {
				user = "postgres"
			}
			args := []string{"-h", "127.0.0.1", "-p", strconv.Itoa(port), "-U", user, "-d", db, "-tAc", "SELECT count(*) FROM " + table}
			out, err = dataCommand(ctx, filepath.Join(spec.Dir, "bin", "psql"), args, "PGPASSWORD="+password)
		} else {
			if user == "" {
				user = "root"
			}
			client := "mariadb"
			if _, statErr := os.Stat(filepath.Join(spec.Dir, "bin", "mariadb")); statErr != nil {
				client = "mysql"
			}
			args := []string{"-h", "127.0.0.1", "-P", strconv.Itoa(port), "-u", user, "-D", db, "-N", "-e", "SELECT COUNT(*) FROM " + table}
			out, err = dataCommand(ctx, filepath.Join(spec.Dir, "bin", client), args, "MYSQL_PWD="+password)
		}
		if err != nil {
			info.Error = "—" // table may not exist in this project
		} else if rows, convErr := strconv.Atoi(strings.TrimSpace(string(out))); convErr == nil {
			info.Rows = rows
		}
		data.Tables = append(data.Tables, info)
	}
	cachePort := strings.TrimSpace(env["REDIS_PORT"])
	if cachePort == "" {
		cachePort = strconv.Itoa(c.Ports["redis"])
	}
	if cachePort != "" && cachePort != "0" {
		out, err := dataCommand(ctx, filepath.Join(e.Paths.Bin, "redis-cli"),
			[]string{"-h", "127.0.0.1", "-p", cachePort, "-n", "1", "DBSIZE"}, "")
		if err == nil {
			data.CacheKeys, _ = strconv.Atoi(strings.TrimSpace(string(out)))
		}
	}
	if status, mailErr := e.MailControl(ctx, MailRequest{Op: "status"}); mailErr == nil {
		data.Mail = status.Count
	}
	return data
}

func dataCommand(ctx context.Context, bin string, args []string, secret string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, bin, args...)
	if secret != "" {
		cmd.Env = append(cmd.Environ(), secret)
	}
	return cmd.Output()
}
