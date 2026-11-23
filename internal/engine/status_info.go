package engine

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/lair-sh/lair/internal/platform"
)

// DNSStatus is a read-only snapshot of local name resolution for the
// development domain. It uses the same probe as the privileged setup script.
type DNSStatus struct {
	Host     string   `json:"host"`
	OK       bool     `json:"ok"`
	Resolved []string `json:"resolved,omitempty"`
	Detail   string   `json:"detail,omitempty"`
}

// CAStatus is a read-only snapshot of the local certificate authority used to
// sign *.test certificates. Trust changes stay in the privileged scripts.
type CAStatus struct {
	Exists         bool   `json:"exists"`
	SystemTrusted  bool   `json:"system_trusted"`
	BrowserTrusted bool   `json:"browser_trusted"`
	SiteCerts      int    `json:"site_certs"`
	Path           string `json:"path,omitempty"`
}

func (e *Engine) statusDNS(domain string) DNSStatus {
	domain = strings.TrimPrefix(strings.TrimSpace(domain), ".")
	if domain == "" {
		domain = "test"
	}
	host := "lair." + domain
	s := DNSStatus{Host: host}
	ips, err := platform.ResolveHost(host)
	if err != nil {
		s.Detail = err.Error()
		return s
	}
	s.Resolved = ips
	for _, ip := range ips {
		if ip == "127.0.0.1" || ip == "::1" {
			s.OK = true
			break
		}
	}
	if !s.OK {
		s.Detail = "no resuelve a 127.0.0.1"
	}
	return s
}

func (e *Engine) statusCA() CAStatus {
	ca := filepath.Join(e.Paths.Certs, "ca.pem")
	s := CAStatus{Path: ca}
	if info, err := os.Stat(ca); err == nil && !info.IsDir() {
		s.Exists = true
	}
	for _, path := range []string{
		"/usr/local/share/ca-certificates/lair-local-ca.crt",
		"/etc/pki/ca-trust/source/anchors/lair-local-ca.crt",
	} {
		if _, err := os.Stat(path); err == nil {
			s.SystemTrusted = true
			break
		}
	}
	if _, err := exec.LookPath("certutil"); err == nil {
		if home, err := os.UserHomeDir(); err == nil {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			out, err := exec.CommandContext(ctx, "certutil", "-d", "sql:"+filepath.Join(home, ".pki", "nssdb"), "-L").Output()
			cancel()
			if err == nil && strings.Contains(string(out), "lair-local-ca") {
				s.BrowserTrusted = true
			}
		}
	}
	entries, err := os.ReadDir(e.Paths.Certs)
	if err == nil {
		for _, entry := range entries {
			name := entry.Name()
			if entry.IsDir() || !strings.HasSuffix(name, ".pem") || name == "ca.pem" || name == "ca-key.pem" {
				continue
			}
			s.SiteCerts++
		}
	}
	return s
}
