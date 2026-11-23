// Package daemon's Runtime wires together the local API, the DNS server, the
// desktop API bridge and the service supervisors. It is the long-running
// process started by `lair daemon --foreground` (or the systemd unit).
package daemon

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"github.com/lair-sh/lair/internal/app"
	"github.com/lair-sh/lair/internal/desktopapi"
	"github.com/lair-sh/lair/internal/dns"
	"github.com/lair-sh/lair/internal/engine"
	"github.com/lair-sh/lair/internal/manifest"
	"github.com/lair-sh/lair/internal/pki"
	"github.com/lair-sh/lair/internal/services"
	"github.com/lair-sh/lair/internal/web"
)

// RuntimeConfig configures a daemon Runtime.
type RuntimeConfig struct {
	Paths         app.Paths
	Catalog       *manifest.Catalog
	Domain        string
	DNSPort       int
	HTTPPort      int
	HTTPSPort     int
	DesktopPort   int
	EnableDNS     bool
	EnableWeb     bool
	EnableAPI     bool
	EnableDesktop bool
}

// Runtime owns all long-lived daemon components.
type Runtime struct {
	cfg          RuntimeConfig
	Engine       *engine.Engine
	api          *Server
	dns          *dns.Server
	web          *web.Server
	desktop      *desktopapi.Server
	token        string
	desktopToken string
	desktopURL   string
	sock         string

	stopOnce sync.Once
	stop     context.CancelFunc
}

// Defaults returns a RuntimeConfig with production defaults.
func Defaults(paths app.Paths, catalog *manifest.Catalog) RuntimeConfig {
	return RuntimeConfig{
		Paths: paths, Catalog: catalog, Domain: "test",
		DNSPort: dns.DefaultPort, HTTPPort: 80, HTTPSPort: 443,
		DesktopPort: 9797,
		EnableDNS:   true, EnableWeb: true, EnableAPI: true, EnableDesktop: true,
	}
}

// New builds the runtime. No sockets are opened until Run is called.
func New(cfg RuntimeConfig) (*Runtime, error) {
	if cfg.Domain == "" {
		cfg.Domain = "test"
	}
	if cfg.DNSPort == 0 {
		cfg.DNSPort = dns.DefaultPort
	}
	if cfg.HTTPPort == 0 {
		cfg.HTTPPort = 80
	}
	if cfg.HTTPSPort == 0 {
		cfg.HTTPSPort = 443
	}
	if cfg.DesktopPort == 0 {
		cfg.DesktopPort = 9797
	}
	if err := cfg.Paths.Ensure(); err != nil {
		return nil, err
	}
	eng, err := engine.New(cfg.Paths, cfg.Catalog)
	if err != nil {
		return nil, err
	}
	// Project discovery mutates config and the filesystem. Do it once at daemon
	// startup instead of from every read-only Status() call (including shims).
	_ = eng.SyncProjectDirs()

	// Explicit --http-port/--https-port flags override the persisted settings
	// for this daemon run; with no flags config.json remains authoritative.
	settings := eng.WebSettings()
	if cfg.HTTPPort != 80 {
		settings.HTTPPort = cfg.HTTPPort
	}
	if cfg.HTTPSPort != 443 {
		settings.HTTPSPort = cfg.HTTPSPort
	}
	eng.Web.SetConfig(settings.HTTPPort, settings.HTTPSPort, settings.Bind, settings.Domain)
	r := &Runtime{cfg: cfg, Engine: eng}
	// The engine owns the router and the web settings from config.json. Using
	// its server directly keeps site registration and the listener in sync.
	r.web = eng.Web
	r.dns = dns.NewServer(cfg.Domain, cfg.DNSPort)
	r.sock, err = cfg.Paths.SocketPath("api")
	if err != nil {
		return nil, err
	}
	return r, nil
}

// Socket and Token return the API connection details for the CLI.
func (r *Runtime) Socket() string { return r.sock }
func (r *Runtime) Token() string  { return r.token }

// DesktopURL returns the loopback URL of the desktop API bridge. It is
// available once Run has started the server. The token is kept separate.
func (r *Runtime) DesktopURL() string { return r.desktopURL }

// DesktopToken returns the token used by the desktop API bridge.
func (r *Runtime) DesktopToken() string { return r.desktopToken }

// Run starts every enabled component and blocks until ctx is cancelled, a
// shutdown signal arrives, or a fatal component error occurs.
func (r *Runtime) Run(ctx context.Context) error {
	runCtx, cancel := context.WithCancel(ctx)
	r.stop = cancel
	defer cancel()
	defer r.Engine.FPM.StopAll()
	defer r.Engine.CGI.StopAll()

	// The catalog may have been started by the caller; starting it again is
	// safe (LoadFromCache is idempotent and the refresh loop honours ctx).
	if r.cfg.Catalog != nil {
		// A corrected build automatically replaces an installed yanked one.
		r.cfg.Catalog.Subscribe(func(*manifest.Manifest) {
			if err := r.Engine.ReplaceYanked(runCtx); err != nil {
				fmt.Fprintf(os.Stderr, "lair: yanked-build replacement: %v\n", err)
			}
		})
		if err := r.cfg.Catalog.Start(runCtx); err != nil {
			return fmt.Errorf("start catalog: %w", err)
		}
	}

	var wg sync.WaitGroup
	errCh := make(chan error, 4)

	// Keep the outgoing CA bundle (system roots + local CA) current so PHP,
	// Composer and other tools can reach public HTTPS after an upgrade.
	if _, _, err := pki.EnsureCA(r.Engine.Paths.Certs); err != nil {
		fmt.Fprintf(os.Stderr, "lair: warning: CA bundle: %v\n", err)
	}

	// The PHP dumps agent is regenerated on every start so its embedded paths
	// stay correct after an upgrade. auto_prepend_file is written only to the
	// CLI scan dirs of installed PHP versions, never to FPM config.
	if err := r.Engine.EnsureDumpsAgent(); err != nil {
		fmt.Fprintf(os.Stderr, "lair: warning: dumps agent: %v\n", err)
	}

	// All local front-ends (unix API and desktop bridge) share one dispatcher
	// so the native app and the CLI execute exactly the same set of actions.
	dispatcher := NewDispatcher(r.Engine, r.stop)
	dispatcher.Set(ActionRestart, func(context.Context, json.RawMessage) (any, error) {
		r.restartServices()
		return map[string]bool{"restarted": true}, nil
	})

	// DNS (unprivileged UDP on loopback).
	if r.cfg.EnableDNS {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := r.dns.ListenAndServe(runCtx); err != nil {
				errCh <- fmt.Errorf("dns: %w", err)
			}
		}()
	}

	// Web layer. The embedded Go server is the only web listener; a binding
	// failure is non-fatal for DNS/API/desktop.
	if r.cfg.EnableWeb {
		if err := r.Engine.StartWeb(); err != nil {
			fmt.Fprintf(os.Stderr, "lair: warning: web server: %v\n", err)
		} else {
			fmt.Fprintf(os.Stderr, "lair: web layer: go\n")
			go func() {
				<-runCtx.Done()
				shutdownCtx, sCancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer sCancel()
				_ = r.web.Shutdown(shutdownCtx)
			}()
		}
		if view := r.Engine.WebStatus(); view.Warning != "" {
			fmt.Fprintf(os.Stderr, "lair: warning: %s\n", view.Warning)
		}
	}

	// Local API over a token-protected unix socket.
	if r.cfg.EnableAPI {
		tok, err := NewToken()
		if err != nil {
			return err
		}
		r.token = tok
		if err := WriteToken(filepath.Join(r.cfg.Paths.Run, "api.token"), tok); err != nil {
			return fmt.Errorf("write api token: %w", err)
		}
		r.api = NewServer(r.sock, tok)
		Register(r.api, dispatcher)
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := r.api.Serve(runCtx); err != nil {
				errCh <- fmt.Errorf("api: %w", err)
			}
		}()
	}

	// HTTP bridge for the native desktop app on loopback. A dedicated token
	// is written to run/desktop.token (0600) and the base URL to
	// run/desktop.url (0600). There is no web interface to serve.
	if r.cfg.EnableDesktop {
		tok, err := NewToken()
		if err != nil {
			return err
		}
		r.desktopToken = tok
		srv, err := desktopapi.New(dispatcher, tok)
		if err != nil {
			return fmt.Errorf("desktop api: %w", err)
		}
		r.desktop = srv
		tokenPath := filepath.Join(r.cfg.Paths.Run, "desktop.token")
		urlPath := filepath.Join(r.cfg.Paths.Run, "desktop.url")
		if err := WriteToken(tokenPath, tok); err != nil {
			return fmt.Errorf("write desktop token: %w", err)
		}
		r.desktopURL = fmt.Sprintf("http://127.0.0.1:%d", r.cfg.DesktopPort)
		if err := os.WriteFile(urlPath, []byte(r.desktopURL+"\n"), 0o600); err != nil {
			return fmt.Errorf("write desktop url: %w", err)
		}
		_ = os.Chmod(urlPath, 0o600)
		wg.Add(1)
		go func() {
			defer wg.Done()
			addr := fmt.Sprintf("127.0.0.1:%d", r.cfg.DesktopPort)
			if err := r.desktop.ListenAndServe(runCtx, addr); err != nil {
				fmt.Fprintf(os.Stderr, "lair: warning: desktop api: %v\n", err)
			}
		}()
	}

	// Bring up all configured services under the supervisor.
	r.startConfiguredServices()

	// OS signals stop the daemon cleanly.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)
	defer signal.Stop(sigCh)

	var runErr error
	select {
	case <-runCtx.Done():
	case sig := <-sigCh:
		fmt.Fprintf(os.Stderr, "lair: received %s, shutting down\n", sig)
		cancel()
	case runErr = <-errCh:
		cancel()
	}

	// Stop supervised services before releasing other resources.
	r.Engine.Supervisors.StopAll()
	_, _ = r.Engine.MailControl(context.Background(), MailRequest{Op: "stop"})
	cancel()
	wg.Wait()
	_ = os.Remove(filepath.Join(r.cfg.Paths.Run, "api.token"))
	_ = os.Remove(filepath.Join(r.cfg.Paths.Run, "desktop.token"))
	_ = os.Remove(filepath.Join(r.cfg.Paths.Run, "desktop.url"))
	return runErr
}

// startConfiguredServices starts supervisors for services that have an active
// version and an installed build. Failures are non-fatal: a bad service should
// not prevent the environment from coming up.
func (r *Runtime) startConfiguredServices() {
	c, err := r.Engine.Config.View()
	if err != nil {
		return
	}
	for _, id := range services.IDs() {
		ver := c.Defaults[id]
		if ver == "" {
			continue
		}
		if err := r.Engine.StartService(id); err != nil {
			fmt.Fprintf(os.Stderr, "lair: skipping service %s: %v\n", id, err)
		}
	}
}

// restartServices reloads specs and restarts every configured supervisor.
func (r *Runtime) restartServices() {
	c, err := r.Engine.Config.View()
	if err != nil {
		return
	}
	for _, id := range services.IDs() {
		if c.Defaults[id] == "" {
			continue
		}
		if err := r.Engine.RestartService(id); err != nil {
			fmt.Fprintf(os.Stderr, "lair: restart %s: %v\n", id, err)
		}
	}
}

// Shutdown requests a clean stop from outside Run (used by tests).
func (r *Runtime) Shutdown() {
	if r.stop != nil {
		r.stopOnce.Do(r.stop)
	}
}
