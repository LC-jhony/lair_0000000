package engine

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
)

func TestWebControlConfigureHTTPSAndPorts(t *testing.T) {
	e, _ := testEnv(t)
	project := t.TempDir()
	// A ready front controller keeps this test focused on port/HTTPS routing;
	// sites without one serve the built-in status page instead.
	writeTestFile(t, filepath.Join(project, "public", "index.php"), "<?php\n")
	if err := e.Link("acme.test", project); err != nil {
		t.Fatal(err)
	}

	off := false
	res, err := e.WebControl(context.Background(), WebRequest{
		Op: "configure", HTTPPort: 8081, HTTPSPort: 8444, HTTPS: &off,
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Web.HTTPS || res.Web.HTTPPort != 8081 || res.Web.HTTPSPort != 8444 {
		t.Fatalf("web status = %+v", res.Web)
	}
	preview, err := e.SiteVHost("acme.test")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(preview, "ssl_certificate") {
		t.Fatalf("HTTPS block rendered while disabled:\n%s", preview)
	}
	if !strings.Contains(preview, "listen 127.0.0.1:8081;") {
		t.Fatalf("HTTP port missing:\n%s", preview)
	}

	res, err = e.WebControl(context.Background(), WebRequest{Op: "https-on"})
	if err != nil {
		t.Fatal(err)
	}
	if !res.Web.HTTPS {
		t.Fatal("HTTPS was not enabled")
	}
	preview, err = e.SiteVHost("acme.test")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"listen 127.0.0.1:8444 ssl;", "ssl_certificate", "return 301 https://$host:8444$request_uri;"} {
		if !strings.Contains(preview, want) {
			t.Fatalf("vhost missing %q:\n%s", want, preview)
		}
	}
	if _, err := os.Stat(filepath.Join(e.Paths.Certs, "acme.test.pem")); err != nil {
		t.Fatalf("site certificate missing: %v", err)
	}

	c, err := e.Config.View()
	if err != nil {
		t.Fatal(err)
	}
	if c.Web.HTTPPort != 8081 || c.Web.HTTPSPort != 8444 || !c.Web.HTTPS {
		t.Fatalf("persisted web settings = %+v", c.Web)
	}
	// The embedded server always mirrors the persisted settings.
	if e.Web.HTTPPort != c.Web.HTTPPort || e.Web.HTTPSPort != c.Web.HTTPSPort ||
		e.Web.Domain != c.Web.Domain || e.Web.Bind != c.Web.Bind {
		t.Fatalf("server and config out of sync: %+v vs %+v", e.Web, c.Web)
	}
}

func TestWebStatusReflectsSettings(t *testing.T) {
	e, _ := testEnv(t)
	res, err := e.WebControl(context.Background(), WebRequest{Op: "status"})
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	if !res.Web.HTTPS || res.Web.HTTPPort != 80 || res.Web.HTTPSPort != 443 ||
		res.Web.Bind != "127.0.0.1" || res.Web.Domain != "test" || !res.Web.RedirectHTTP {
		t.Fatalf("status = %+v", res.Web)
	}
	if res.Web.Warning != "" || res.Web.Error != "" {
		t.Fatalf("unexpected status warning/error: %+v", res.Web)
	}
}

func TestWebControlInvalidPortIsRejected(t *testing.T) {
	e, _ := testEnv(t)
	if _, err := e.WebControl(context.Background(), WebRequest{Op: "configure", HTTPPort: 80, HTTPSPort: 80}); err == nil {
		t.Fatal("same HTTP/HTTPS port accepted")
	}
	c, err := e.Config.View()
	if err != nil {
		t.Fatal(err)
	}
	if c.Web.HTTPPort != 80 || c.Web.HTTPSPort != 443 {
		t.Fatalf("config was not rolled back: %+v", c.Web)
	}
	if e.Web.HTTPPort != 80 || e.Web.HTTPSPort != 443 {
		t.Fatalf("in-memory server was changed: %+v", e.Web)
	}
}

func TestWebControlAppliesPrivilegedPorts(t *testing.T) {
	e, _ := testEnv(t)
	// Start from different ports, then apply the privileged pair explicitly.
	if _, err := e.WebControl(context.Background(), WebRequest{
		Op: "configure", HTTPPort: 8080, HTTPSPort: 8443,
	}); err != nil {
		t.Fatal(err)
	}
	res, err := e.WebControl(context.Background(), WebRequest{
		Op: "configure", HTTPPort: 80, HTTPSPort: 443,
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Web.HTTPPort != 80 || res.Web.HTTPSPort != 443 {
		t.Fatalf("web status = %+v", res.Web)
	}
	c, err := e.Config.View()
	if err != nil {
		t.Fatal(err)
	}
	if c.Web.HTTPPort != 80 || c.Web.HTTPSPort != 443 {
		t.Fatalf("persisted web settings = %+v", c.Web)
	}
	if e.Web.HTTPPort != 80 || e.Web.HTTPSPort != 443 {
		t.Fatalf("server ports out of sync: %+v", e.Web)
	}
}

func TestWebBindPermissionAdvice(t *testing.T) {
	err := webBindAdvice(fmt.Errorf("listen tcp 127.0.0.1:80: %w", syscall.EACCES), "/home/u/.local/share/lair/bin/lair")
	if err == nil {
		t.Fatal("permission error accepted without advice")
	}
	for _, want := range []string{"sudo setcap cap_net_bind_service=+ep", "/home/u/.local/share/lair/bin/lair"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("advice missing %q: %s", want, err)
		}
	}
	// A busy port must not be misreported as a missing capability.
	busy := webBindAdvice(syscall.EADDRINUSE, "/bin/lair")
	if strings.Contains(busy.Error(), "setcap") {
		t.Fatalf("busy port reported as permission error: %s", busy)
	}
}

func TestWebControlUnknownOperationIsRejected(t *testing.T) {
	e, _ := testEnv(t)
	for _, op := range []string{"test", "regenerate", "start", "stop", "restart", "config", "bogus"} {
		if _, err := e.WebControl(context.Background(), WebRequest{Op: op}); err == nil {
			t.Fatalf("operation %q was accepted", op)
		}
	}
}

func TestWebControlHTTPSOnOffAndBind(t *testing.T) {
	e, _ := testEnv(t)
	if _, err := e.WebControl(context.Background(), WebRequest{Op: "https-off"}); err != nil {
		t.Fatal(err)
	}
	if c, _ := e.Config.View(); c.Web.HTTPS {
		t.Fatalf("https-off did not persist: %+v", c.Web)
	}
	res, err := e.WebControl(context.Background(), WebRequest{Op: "configure", Bind: "0.0.0.0", Domain: "lair.test"})
	if err != nil {
		t.Fatal(err)
	}
	if res.Web.Bind != "0.0.0.0" || res.Web.Domain != "lair.test" || !strings.Contains(res.Web.Warning, "LAN") {
		t.Fatalf("bind/domain status = %+v", res.Web)
	}
	if e.Web.Bind != "0.0.0.0" || e.Web.Domain != "lair.test" {
		t.Fatalf("server not updated: bind=%q domain=%q", e.Web.Bind, e.Web.Domain)
	}
}
