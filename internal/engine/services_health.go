package engine

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/lair-sh/lair/internal/services"
)

// ServiceHealth is the result of one live probe against an installed service.
type ServiceHealth struct {
	ID      string `json:"id"`
	Healthy bool   `json:"healthy"`
	Detail  string `json:"detail,omitempty"`
}

// ServicesHealth probes every supervised service that has an active version.
// It is read-only: no restarts, no writes to config or data.
func (e *Engine) ServicesHealth(ctx context.Context) []ServiceHealth {
	c, err := e.Config.View()
	if err != nil {
		return nil
	}
	out := []ServiceHealth{}
	for _, id := range servicesList() {
		version := c.Defaults[id]
		if version == "" || id == SQLiteToolID {
			continue
		}
		port := c.Ports[id]
		if port == 0 {
			port = services.DefaultPort(id)
		}
		healthy, detail := e.probeService(ctx, id, port)
		out = append(out, ServiceHealth{ID: id, Healthy: healthy, Detail: detail})
	}
	return out
}

func (e *Engine) probeService(ctx context.Context, id string, port int) (bool, string) {
	switch id {
	case "mariadb", "mysql", "postgres":
		return e.probeDatabase(ctx, id, port)
	case "redis", "valkey":
		client := "redis-cli"
		if id == "valkey" {
			client = "valkey-cli"
		}
		out, err := probeCommand(ctx, filepath.Join(e.Paths.Bin, client), "-p", strconv.Itoa(port), "PING")
		return err == nil && strings.Contains(strings.ToUpper(out), "PONG"), strings.TrimSpace(out)
	case "mongodb":
		spec, err := e.ServiceSpec("mongodb")
		if err != nil {
			return false, err.Error()
		}
		out, err := probeCommand(ctx, filepath.Join(spec.Dir, "bin", "mongosh"),
			"--quiet", "--eval", "db.adminCommand({ping:1}).ok")
		return err == nil && strings.Contains(lastLine(out), "1"), lastLine(out)
	case "meilisearch", "typesense", "rustfs":
		path := "/health"
		if id == "rustfs" {
			path = "/"
		}
		return probeHTTP(ctx, fmt.Sprintf("http://127.0.0.1:%d%s", port, path))
	default:
		return false, "sin sonda"
	}
}

func (e *Engine) probeDatabase(ctx context.Context, id string, port int) (bool, string) {
	spec, err := e.ServiceSpec(id)
	if err != nil {
		return false, err.Error()
	}
	if id == "postgres" {
		out, err := probeCommand(ctx, filepath.Join(spec.Dir, "bin", "psql"),
			"-h", "127.0.0.1", "-p", strconv.Itoa(port), "-U", "postgres", "-d", "postgres", "-tAc", "SELECT 1")
		return err == nil && strings.TrimSpace(out) == "1", strings.TrimSpace(out)
	}
	client := "mariadb"
	if _, err := os.Stat(filepath.Join(spec.Dir, "bin", "mariadb")); err != nil {
		client = "mysql"
	}
	out, err := probeCommand(ctx, filepath.Join(spec.Dir, "bin", client),
		"-h", "127.0.0.1", "-P", strconv.Itoa(port), "-u", "root", "-N", "-e", "SELECT 1")
	// The MariaDB client prints an SSL warning on stdout before the result.
	return err == nil && lastLine(out) == "1", lastLine(out)
}

func lastLine(out string) string {
	lines := strings.Split(strings.TrimSpace(out), "\n")
	return strings.TrimSpace(lines[len(lines)-1])
}

func probeCommand(ctx context.Context, bin string, args ...string) (string, error) {
	runCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	out, err := exec.CommandContext(runCtx, bin, args...).CombinedOutput()
	return string(out), err
}

func probeHTTP(ctx context.Context, url string) (bool, string) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return false, err.Error()
	}
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return false, err.Error()
	}
	defer resp.Body.Close()
	return resp.StatusCode < 500, resp.Status
}
