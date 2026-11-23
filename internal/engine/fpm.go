package engine

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"

	"github.com/lair-sh/lair/internal/app"
	"github.com/lair-sh/lair/internal/php"
)

// syncFPM ensures one supervised php-fpm pool per site. The PHP version comes
// from the site's isolation, then the global default, then the newest installed
// build. Pools are idempotent and stop when their site disappears.
func (e *Engine) syncFPM() {
	// Windows has no php-fpm: the web layer uses php-cgi worker pools on
	// loopback ports (see php.CGIPort) instead of unix sockets.
	if strings.HasPrefix(app.Platform(), "windows-") {
		return
	}
	c, err := e.Config.View()
	if err != nil {
		return
	}
	installed, _ := php.Installed(e.Paths)
	byMinor := map[string]php.Info{}
	for _, info := range installed {
		byMinor[info.Minor] = info
	}
	desired := map[string]bool{}
	writtenGlobal := map[string]bool{}
	for name, site := range c.Sites {
		if site.Path == "" {
			continue
		}
		minor := site.PHP
		if minor == "" {
			minor = c.Defaults["php"]
		}
		if minor == "" && len(installed) > 0 {
			minor = installed[0].Minor
		}
		info, ok := byMinor[minor]
		if !ok {
			continue
		}
		if !writtenGlobal[minor] {
			writtenGlobal[minor] = true
			if body, err := php.GlobalFPMConfig(info, e.Paths); err == nil {
				_ = os.WriteFile(filepath.Join(e.Paths.Run, "php-fpm-"+minor+".conf"), []byte(body), 0o600)
			}
		}
		bin := filepath.Join(info.Dir, "sbin", "php-fpm")
		if _, err := os.Stat(bin); err != nil {
			continue
		}
		body, err := php.FPMConfig(e.Paths, info, name, false)
		if err != nil {
			continue
		}
		confPath := filepath.Join(e.Paths.Run, "fpm-"+name+".conf")
		if err := os.WriteFile(confPath, []byte(body), 0o600); err != nil {
			continue
		}
		sock, err := e.Paths.SocketPath("fpm-" + name)
		if err != nil {
			continue
		}
		id := "fpm-" + name
		desired[id] = true
		key := bin + "|" + confPath + "|" + minor
		if err := e.FPM.Start(id, key, bin, confPath, site.Path, sock, php.Env(info, e.Paths)); err != nil {
			fmt.Fprintf(os.Stderr, "lair: php-fpm %s: %v\n", name, err)
		}
	}
	for _, id := range e.FPM.IDs() {
		if !desired[id] {
			e.FPM.Stop(id)
		}
	}
}

// fpmPool is one supervised php-fpm process serving a site's FastCGI socket.
type fpmPool struct {
	cancel context.CancelFunc
	done   chan struct{}
	key    string
}

// FPMManager owns the per-site php-fpm pools. They run as the current user and
// stop with the daemon.
type FPMManager struct {
	mu     sync.Mutex
	pools  map[string]*fpmPool
	logDir string
}

// NewFPMManager creates an empty pool manager.
func NewFPMManager(logDir string) *FPMManager {
	return &FPMManager{pools: map[string]*fpmPool{}, logDir: logDir}
}

// Start launches a pool unless an identical one is already running.
func (m *FPMManager) Start(id, key, bin, configPath, workDir, socket string, env []string) error {
	m.mu.Lock()
	if current, ok := m.pools[id]; ok {
		if current.key == key {
			m.mu.Unlock()
			return nil
		}
		m.mu.Unlock()
		m.Stop(id)
		return m.Start(id, key, bin, configPath, workDir, socket, env)
	}
	m.mu.Unlock()

	// A stale socket from a crashed pool must not block the new one.
	_ = os.Remove(socket)
	if err := os.MkdirAll(m.logDir, 0o755); err != nil {
		return err
	}
	logPath := filepath.Join(m.logDir, id+".log")
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithCancel(context.Background())
	// The static PHP build defaults to /var/log/php-fpm.log, which is not
	// writable by the user. Override it before FPM parses its config.
	cmd := exec.CommandContext(ctx, bin, "-F", "-O", "-y", configPath, "-d", "error_log="+logPath)
	cmd.Dir = workDir
	cmd.Env = env
	cmd.Stdout, cmd.Stderr = logFile, logFile
	if err := cmd.Start(); err != nil {
		cancel()
		logFile.Close()
		return err
	}
	done := make(chan struct{})
	go func() {
		_ = cmd.Wait()
		_ = logFile.Close()
		close(done)
	}()
	m.mu.Lock()
	m.pools[id] = &fpmPool{cancel: cancel, done: done, key: key}
	m.mu.Unlock()
	return nil
}

// Stop terminates one pool and removes its socket.
func (m *FPMManager) Stop(id string, socket ...string) {
	m.mu.Lock()
	pool, ok := m.pools[id]
	if ok {
		delete(m.pools, id)
	}
	m.mu.Unlock()
	if ok {
		pool.cancel()
		<-pool.done
	}
	if len(socket) > 0 && socket[0] != "" {
		_ = os.Remove(socket[0])
	}
}

// StopAll terminates every pool. Used on daemon shutdown.
func (m *FPMManager) StopAll() {
	m.mu.Lock()
	ids := make([]string, 0, len(m.pools))
	for id := range m.pools {
		ids = append(ids, id)
	}
	m.mu.Unlock()
	for _, id := range ids {
		m.Stop(id)
	}
}

// IDs returns the ids of all supervised pools.
func (m *FPMManager) IDs() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]string, 0, len(m.pools))
	for id := range m.pools {
		out = append(out, id)
	}
	return out
}

// Running reports whether a pool is currently supervised.
func (m *FPMManager) Running(id string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	_, ok := m.pools[id]
	return ok
}
