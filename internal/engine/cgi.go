package engine

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"

	"github.com/lair-sh/lair/internal/config"
	"github.com/lair-sh/lair/internal/php"
	"github.com/lair-sh/lair/internal/version"
)

// cgiWorker is one supervised php-cgi.exe process bound to a loopback port.
type cgiWorker struct {
	cancel context.CancelFunc
	done   chan struct{}
	key    string
}

// CGIManager owns the Windows php-cgi pools. It is a no-op manager on Unix,
// where FPMManager is used instead.
type CGIManager struct {
	mu      sync.Mutex
	workers map[string]*cgiWorker
}

// NewCGIManager creates an empty worker manager.
func NewCGIManager() *CGIManager { return &CGIManager{workers: map[string]*cgiWorker{}} }

// Start launches a worker unless an identical one is already running.
func (m *CGIManager) Start(id, key, bin, addr, workDir string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if current, ok := m.workers[id]; ok {
		if current.key == key {
			return nil
		}
		delete(m.workers, id)
		current.cancel()
		select {
		case <-current.done:
		case <-time.After(3 * time.Second):
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cmd := exec.CommandContext(ctx, bin, "-b", addr)
	cmd.Dir = workDir
	cmd.Env = os.Environ()
	// Never write raw FPM/CGI stderr to the terminal: it may contain paths or
	// credentials. Logs are handled by the daemon.
	cmd.Stdout, cmd.Stderr = io.Discard, io.Discard
	if err := cmd.Start(); err != nil {
		cancel()
		return err
	}
	done := make(chan struct{})
	go func() {
		_ = cmd.Wait()
		close(done)
	}()
	m.workers[id] = &cgiWorker{cancel: cancel, done: done, key: key}
	return nil
}

// Stop terminates one worker.
func (m *CGIManager) Stop(id string) {
	m.mu.Lock()
	worker, ok := m.workers[id]
	if ok {
		delete(m.workers, id)
	}
	m.mu.Unlock()
	if ok {
		worker.cancel()
		select {
		case <-worker.done:
		case <-time.After(3 * time.Second):
		}
	}
}

// StopAll terminates every worker.
func (m *CGIManager) StopAll() {
	m.mu.Lock()
	ids := make([]string, 0, len(m.workers))
	for id := range m.workers {
		ids = append(ids, id)
	}
	m.mu.Unlock()
	for _, id := range ids {
		m.Stop(id)
	}
}

// IDs returns the ids of all supervised workers.
func (m *CGIManager) IDs() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]string, 0, len(m.workers))
	for id := range m.workers {
		out = append(out, id)
	}
	return out
}

// syncWindowsCGI starts one php-cgi worker per installed PHP minor used by an
// enabled site, bound to the loopback port derived from that minor.
func (e *Engine) syncWindowsCGI(c *config.Config) {
	installed, _ := php.Installed(e.Paths)
	byMinor := map[string]php.Info{}
	for _, info := range installed {
		byMinor[info.Minor] = info
	}
	desired := map[string]bool{}
	for _, site := range c.Sites {
		if site.Path == "" || !site.Enabled {
			continue
		}
		minor := site.PHP
		if minor == "" {
			minor = c.Defaults["php"]
		}
		if parsed, err := version.Parse(minor); err == nil {
			minor = parsed.MinorLine()
		}
		info, ok := byMinor[minor]
		if !ok {
			continue
		}
		port, err := php.CGIPort(minor, false)
		if err != nil {
			continue
		}
		bin := filepath.Join(info.Dir, "bin", "php-cgi.exe")
		if _, err := os.Stat(bin); err != nil {
			continue
		}
		addr := fmt.Sprintf("127.0.0.1:%d", port)
		id := "cgi-" + minor
		desired[id] = true
		key := bin + "|" + addr
		if err := e.CGI.Start(id, key, bin, addr, site.Path); err != nil {
			fmt.Fprintf(os.Stderr, "lair: php-cgi %s: %v\n", minor, err)
		}
	}
	for _, id := range e.CGI.IDs() {
		if !desired[id] {
			e.CGI.Stop(id)
		}
	}
}
