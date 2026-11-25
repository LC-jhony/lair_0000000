package engine

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/lair-sh/lair/internal/services"
)

// QueueInfo is a read-only snapshot of a Laravel site's queue backend. Lair
// does not supervise queue workers; the hint shows how to process one job.
type QueueInfo struct {
	Site    string `json:"site"`
	Driver  string `json:"driver"`
	Pending int    `json:"pending"`
	Failed  int    `json:"failed"`
	Worker  string `json:"worker,omitempty"`
	Hint    string `json:"hint,omitempty"`
	Error   string `json:"error,omitempty"`
}

// QueueControl reads QUEUE_CONNECTION from the site's .env and counts pending
// and failed jobs from the configured backend (database or Redis). It never
// mutates the project and never exposes credentials.
func (e *Engine) QueueControl(ctx context.Context, name string) QueueInfo {
	name = strings.ToLower(strings.TrimSuffix(strings.TrimSpace(name), "."))
	q := QueueInfo{Site: name, Worker: "not supervised"}
	c, err := e.Config.View()
	if err != nil {
		q.Error = err.Error()
		return q
	}
	site, ok := c.Sites[name]
	if !ok || site.Path == "" {
		q.Error = fmt.Sprintf("sitio desconocido %q", name)
		return q
	}
	env, _, _ := readEnvFile(filepath.Join(site.Path, ".env"))
	q.Driver = strings.TrimSpace(env["QUEUE_CONNECTION"])
	if q.Driver == "" {
		q.Driver = "database"
	}
	q.Hint = fmt.Sprintf("lair exec php artisan queue:work %s --once", q.Driver)
	switch q.Driver {
	case "database":
		dbDriver := strings.TrimSpace(env["DB_CONNECTION"])
		if dbDriver == "" {
			dbDriver = site.Database
		}
		if dbDriver == "" {
			dbDriver = "mariadb"
		}
		q.Pending, q.Failed, err = e.countDBQueue(ctx, dbDriver, env)
		if err != nil {
			q.Error = err.Error()
		}
	case "redis":
		q.Pending, err = e.redisQueueLen(ctx, env)
		if err != nil {
			q.Error = err.Error()
		}
	default:
		q.Error = fmt.Sprintf("driver de cola %q no soportado por Lair", q.Driver)
	}
	return q
}

func (e *Engine) countDBQueue(ctx context.Context, driver string, env map[string]string) (pending, failed int, err error) {
	id := e.databaseServiceID(driver)
	spec, err := e.ServiceSpec(id)
	if err != nil {
		return 0, 0, err
	}
	c, err := e.Config.View()
	if err != nil {
		return 0, 0, err
	}
	port := c.Ports[id]
	if raw := strings.TrimSpace(env["DB_PORT"]); raw != "" {
		if parsed, convErr := strconv.Atoi(raw); convErr == nil && parsed > 0 {
			port = parsed
		}
	}
	if port == 0 {
		port = services.DefaultPort(driver)
	}
	database := strings.TrimSpace(env["DB_DATABASE"])
	user := strings.TrimSpace(env["DB_USERNAME"])
	password := env["DB_PASSWORD"]

	if id == "postgres" || driver == "pgsql" {
		if user == "" {
			user = "postgres"
		}
		bin := filepath.Join(spec.Dir, "bin", "psql")
		base := []string{"-h", "127.0.0.1", "-p", strconv.Itoa(port), "-U", user, "-d", database, "-tAc"}
		pending = queryCount(ctx, bin, append(append([]string{}, base...), "SELECT count(*) FROM jobs"), "PGPASSWORD="+password)
		failed = queryCount(ctx, bin, append(append([]string{}, base...), "SELECT count(*) FROM failed_jobs"), "PGPASSWORD="+password)
		return pending, failed, nil
	}
	if user == "" {
		user = "root"
	}
	client := "mariadb"
	if _, err := os.Stat(filepath.Join(spec.Dir, "bin", "mariadb")); err != nil {
		client = "mysql"
	}
	bin := filepath.Join(spec.Dir, "bin", client)
	base := []string{"-h", "127.0.0.1", "-P", strconv.Itoa(port), "-u", user, "-D", database, "-N", "-e"}
	pending = queryCount(ctx, bin, append(append([]string{}, base...), "SELECT COUNT(*) FROM jobs"), "MYSQL_PWD="+password)
	failed = queryCount(ctx, bin, append(append([]string{}, base...), "SELECT COUNT(*) FROM failed_jobs"), "MYSQL_PWD="+password)
	return pending, failed, nil
}

func queryCount(ctx context.Context, bin string, args []string, secret string) int {
	runCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	cmd := exec.CommandContext(runCtx, bin, args...)
	cmd.Env = append(os.Environ(), secret)
	out, err := cmd.Output()
	if err != nil {
		return 0 // table may not exist yet
	}
	value, err := strconv.Atoi(strings.TrimSpace(string(out)))
	if err != nil || value < 0 {
		return 0
	}
	return value
}

func (e *Engine) redisQueueLen(ctx context.Context, env map[string]string) (int, error) {
	port := strings.TrimSpace(env["REDIS_PORT"])
	if port == "" {
		port = strconv.Itoa(services.DefaultPort("redis"))
	}
	host := strings.TrimSpace(env["REDIS_HOST"])
	if host == "" {
		host = "127.0.0.1"
	}
	bin := filepath.Join(e.Paths.Bin, "redis-cli")
	runCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	cmd := exec.CommandContext(runCtx, bin, "-h", host, "-p", port, "LLEN", "queues:default")
	out, err := cmd.Output()
	if err != nil {
		return 0, err
	}
	value, err := strconv.Atoi(strings.TrimSpace(string(out)))
	if err != nil || value < 0 {
		return 0, fmt.Errorf("respuesta Redis no válida: %q", strings.TrimSpace(string(out)))
	}
	return value, nil
}
