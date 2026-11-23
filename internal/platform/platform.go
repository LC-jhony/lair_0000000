// Package platform contains the OS-specific integration points that require
// privileges or talk to the host: the systemd unit + polkit rule, the
// systemd-resolved configuration for *.test, the trust store, and the
// privileged "apply once" plan.
//
// The abstraction has two parts:
//
//   - pure generators (SystemdUnit, PolkitRule, ResolvedDropIn, ...) that only
//     render files and are fully unit-testable;
//   - an Applier that performs privileged actions. It is never run inline by
//     the daemon: it is rendered as a single script the user runs once through
//     pkexec/sudo BEFORE Lair replaces anything on the system.
//
// Linux/systemd-resolved is the initial implementation. macOS and Windows
// provide clearly-marked stubs so the rest of the code compiles everywhere.
package platform

import (
	"fmt"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
)

// Kind identifies the host platform integration in use.
type Kind string

const (
	KindSystemd Kind = "systemd"
	KindDarwin  Kind = "darwin"
	KindWindows Kind = "windows"
)

// Current reports the platform abstraction for this OS.
func Current() Kind {
	switch runtime.GOOS {
	case "linux":
		return KindSystemd
	case "darwin":
		return KindDarwin
	case "windows":
		return KindWindows
	default:
		return Kind(runtime.GOOS)
	}
}

// Supported reports whether this platform has a real implementation.
// macOS and Windows have their own (non-systemd) privileged plans.
func (k Kind) Supported() bool {
	return k == KindSystemd || k == KindDarwin || k == KindWindows
}

// UnitFiles is the set of files generated for a system service.
type UnitFiles struct {
	UnitPath        string
	UnitContent     string
	DropInPath      string
	DropInContent   string
	PolkitPath      string
	PolkitContent   string
	ResolvedPath    string
	ResolvedContent string
	// DNS files implement the "safe dnsmasq" layout: dnsmasq only answers on
	// 127.0.0.1:53535 and resolved forwards just the local domain to it.
	DnsmasqConfPath    string
	DnsmasqConfContent string
	DNSUnitPath        string
	DNSUnitContent     string
	// FallbackResolvedContent is the pre-dnsmasq drop-in used when dnsmasq
	// cannot run. It keeps Internet resolution scoped to the local domain.
	FallbackResolvedContent string
	// macOS: /etc/resolver/<domain> points *.test at Lair's embedded DNS
	// server (already cross-platform, UDP 53535).
	ResolverPath    string
	ResolverContent string
	// Windows: hosts-file entries for the parked sites, rendered by BuildPlan
	// from the configured site names. The embedded DNS server is not usable
	// there without an elevated interface-level DNS change.
	HostsPath  string
	HostsBlock string
}

// UnitConfig carries the values needed to render the unit and its drop-in.
type UnitConfig struct {
	User       string // systemd instance user (%i)
	Home       string // user's home
	DataDir    string // LAIR data root
	ExecPath   string // absolute path to the lair binary
	Domain     string // e.g. "test"
	DNSAddress string // fallback resolved address, e.g. "127.0.0.1#53535"
	// DNSBinary is the executable given CAP_NET_BIND_SERVICE by the script.
	// Defaults to <DataDir>/bin/lair.
	DNSBinary string
	// Hosts lists the parked host names used by the Windows hosts-file plan.
	Hosts []string
	// CAPath is the local CA certificate installed by the privileged plan.
	CAPath string
}

// Generate renders every privileged file for the current platform.
func Generate(cfg UnitConfig) (UnitFiles, error) {
	switch Current() {
	case KindSystemd:
		return generateSystemd(cfg)
	case KindDarwin:
		return generateDarwin(cfg)
	case KindWindows:
		return generateWindows(cfg)
	default:
		return UnitFiles{}, fmt.Errorf("privileged setup is not implemented for %s yet", Current())
	}
}

// PrivilegedAction is one step in the one-time, pre-flight privileged plan.
type PrivilegedAction struct {
	Description string
	Command     []string
}

// PrivilegedPlan lists all privileged steps. The CLI prints them as a single
// script so elevation is requested once, before any file is replaced.
type PrivilegedPlan struct {
	Actions   []PrivilegedAction
	Files     UnitFiles
	DNSBinary string // setcap target (installed binary or engine executable)
	backupDir string // where the DNS setup keeps its pre-change copies
	Kind      Kind   // drives script rendering (POSIX vs PowerShell)
	CAPath    string // local CA certificate to install in the platform trust store
}

// BuildPlan computes the full plan (rendered files + commands) without
// performing any privileged action.
func BuildPlan(cfg UnitConfig) (*PrivilegedPlan, error) {
	files, err := Generate(cfg)
	if err != nil {
		return nil, err
	}
	binary := cfg.DNSBinary
	if binary == "" {
		binary = filepath.Join(cfg.DataDir, "bin", "lair")
	}
	if !filepath.IsAbs(binary) {
		binary = filepath.Join(cfg.DataDir, "bin", "lair")
	}
	plan := &PrivilegedPlan{
		Files: files, DNSBinary: binary, Kind: Current(), CAPath: cfg.CAPath,
		backupDir: filepath.Join(cfg.DataDir, "dns", "backup"),
	}
	switch Current() {
	case KindSystemd:
		plan.Actions = []PrivilegedAction{
			{Description: "install systemd unit and drop-in", Command: []string{"/bin/sh", "-c", "install -d -m 0755 /etc/systemd/system && cat > '" + files.UnitPath + "'"}},
			{Description: "install polkit rule for per-user daemon control", Command: []string{"/bin/sh", "-c", "install -d -m 0755 /etc/polkit-1/rules.d"}},
			{Description: "configure systemd-resolved for *." + cfg.Domain, Command: []string{"/bin/sh", "-c", "install -d -m 0755 /etc/systemd/resolved.conf.d"}},
			{Description: "reload systemd and restart resolved", Command: []string{"/bin/sh", "-c", "systemctl daemon-reload && systemctl restart systemd-resolved"}},
		}
	case KindDarwin:
		plan.Actions = []PrivilegedAction{
			{Description: "route *." + cfg.Domain + " to Lair's embedded DNS on 127.0.0.1:" + strconv.Itoa(DefaultDNSPort), Command: []string{"/bin/sh", "-c", "install -d -m 0755 /etc/resolver"}},
			{Description: "install the local CA in the macOS system trust store", Command: []string{"/usr/bin/security", "add-trusted-cert", "-d", "-r", "trustRoot", "-k", "/Library/Keychains/System.keychain", cfg.CAPath}},
		}
	case KindWindows:
		plan.Actions = []PrivilegedAction{
			{Description: "add parked hosts to the Windows hosts file", Command: []string{"powershell", "-NoProfile", "-File", "setup-privileged.ps1"}},
			{Description: "install the local CA in the Windows Root store", Command: []string{"certutil", "-addstore", "-f", "Root", cfg.CAPath}},
		}
	}
	return plan, nil
}

// DNS defaults of the safe dnsmasq setup: a high, loopback-only port, never
// the system-wide port 53.
const (
	DefaultDNSPort    = 53535
	LegacyDNSAddress  = "127.0.0.1#53535"
	DNSUnitName       = "lair-dns.service"
	DefaultDNSDomain  = "test"
	ResolvedDropInDir = "/etc/systemd/resolved.conf.d"
	ResolvedDropIn    = ResolvedDropInDir + "/50-lair.conf"
)

// Script renders the plan as a single POSIX shell script intended to be run
// once with elevation. Files are written via quoted heredocs. The DNS section
// is transactional: it backs up everything it touches, verifies resolution
// after applying the changes and restores the previous state on failure.
func (p *PrivilegedPlan) Script() string {
	if p.Kind == KindWindows {
		return p.powershellScript()
	}
	f := p.Files
	var s strings.Builder
	s.WriteString("#!/bin/sh\n# Generated by Lair. Review before running with elevated privileges.\nset -eu\n")
	write := func(path, content string) {
		fmt.Fprintf(&s, "install -d -m 0755 %s\n", shQuote(dirOf(path)))
		fmt.Fprintf(&s, "cat > %s <<'LAIR_EOF'\n%s\nLAIR_EOF\nchmod 0644 %s\n", shQuote(path), content, shQuote(path))
	}
	if f.UnitPath != "" {
		write(f.UnitPath, f.UnitContent)
	}
	if f.DropInPath != "" {
		write(f.DropInPath, f.DropInContent)
	}
	if f.PolkitPath != "" {
		write(f.PolkitPath, f.PolkitContent)
	}
	if f.DnsmasqConfPath != "" {
		write(f.DnsmasqConfPath, f.DnsmasqConfContent)
	}
	if f.DNSUnitPath != "" {
		write(f.DNSUnitPath, f.DNSUnitContent)
	}
	if f.ResolverPath != "" {
		// macOS resolver file: no systemd, no dnsmasq. Lair's own DNS server
		// answers on the same loopback port on every platform.
		write(f.ResolverPath, f.ResolverContent)
		return s.String()
	}
	// The resolved drop-in is written by the DNS section below, after the
	// previous configuration has been backed up.
	// The setcap target is generated from an absolute engine path, so it is
	// safe to render unquoted (and matches the documented command verbatim).
	s.WriteString(dnsScriptSection(shQuote(p.backupDir), p.DNSBinary,
		shQuote(f.ResolvedPath), f.ResolvedContent, f.FallbackResolvedContent))
	return s.String()
}

func dnsScriptSection(backupDirQ, binaryQ, resolvedPathQ, resolvedContent, fallbackContent string) string {
	fallbackDNS := LegacyDNSAddress
	if i := strings.Index(fallbackContent, "DNS="); i >= 0 {
		line := strings.TrimSpace(strings.SplitN(fallbackContent[i+len("DNS="):], "\n", 2)[0])
		if line != "" {
			fallbackDNS = line
		}
	}
	domain := DefaultDNSDomain
	if idx := strings.Index(resolvedContent, "Domains=~"); idx >= 0 {
		value := strings.TrimSpace(strings.SplitN(resolvedContent[idx+len("Domains=~"):], "\n", 2)[0])
		if value != "" {
			domain = value
		}
	}
	domainQ := shQuote(domain)
	return `
# --- DNS: dnsmasq on a high loopback port, scoped to the local domain ---
DNS_PORT=` + strconv.Itoa(DefaultDNSPort) + `
DOMAIN=` + domainQ + `
BACKUP_DIR=` + backupDirQ + `
STAMP=$(date +%Y%m%d%H%M%S)
backup() {
  src="$1"; name="$2"
  if [ -e "$src" ]; then
    mkdir -p "$BACKUP_DIR"
    cp -a "$src" "$BACKUP_DIR/$name.$STAMP" 2>/dev/null || true
  fi
}
restore() {
  target="$1"; name="$2"; saved="$BACKUP_DIR/$name.$STAMP"
  if [ -f "$saved" ]; then
    mkdir -p "$(dirname "$target")"
    cp -a "$saved" "$target"
  else
    rm -f "$target"
  fi
}
rollback() {
  echo "lair: la verificación de DNS falló; restaurando la configuración anterior" >&2
  systemctl stop ` + DNSUnitName + ` 2>/dev/null || true
  systemctl disable ` + DNSUnitName + ` 2>/dev/null || true
  restore /etc/resolv.conf resolv.conf
  restore /etc/NetworkManager/NetworkManager.conf NetworkManager.conf
  restore ` + shQuote(ResolvedDropIn) + ` 50-lair.conf
  restore /etc/dnsmasq.d/valet dnsmasq-valet
  restore /etc/NetworkManager/conf.d/valet.conf valet-nm.conf
  restore /etc/NetworkManager/dnsmasq.d/valet valet-dnsmasq
  systemctl daemon-reload 2>/dev/null || true
  systemctl restart systemd-resolved 2>/dev/null || true
  exit 1
}
# What public DNS answered before we touch anything. Empty means it did not
# resolve, in which case we must not require it afterwards.
BEFORE_EXAMPLE=$(getent ahostsv4 example.com 2>/dev/null | awk 'NR==1 {print $1}' || true)
backup /etc/resolv.conf resolv.conf
backup /etc/NetworkManager/NetworkManager.conf NetworkManager.conf
backup ` + shQuote(ResolvedDropIn) + ` 50-lair.conf
backup /etc/dnsmasq.d/valet dnsmasq-valet
backup /etc/NetworkManager/conf.d/valet.conf valet-nm.conf
backup /etc/NetworkManager/dnsmasq.d/valet valet-dnsmasq
# Remove classic Laravel Valet leftovers (copies were saved above).
rm -f /etc/dnsmasq.d/valet /etc/NetworkManager/conf.d/valet.conf /etc/NetworkManager/dnsmasq.d/valet
# Grant the low web ports to Lair's own binary. Non-fatal on systems without
# setcap: the user can still choose unprivileged ports in Valet configuration.
sudo setcap cap_net_bind_service=+ep ` + binaryQ + ` 2>/dev/null || true
port_busy() {
  if command -v ss >/dev/null 2>&1; then
    ss -lun 2>/dev/null | grep -q ":$DNS_PORT[[:space:]]" && return 0
    return 1
  fi
  if command -v python3 >/dev/null 2>&1; then
    python3 -c 'import socket; s=socket.socket(socket.AF_INET, socket.SOCK_DGRAM); s.bind(("127.0.0.1", 53535)); s.close()' >/dev/null 2>&1 || return 0
    return 1
  fi
  # Without a probe, prefer the safe fallback over breaking DNS.
  return 0
}
if ! command -v dnsmasq >/dev/null 2>&1 || [ ! -x /usr/sbin/dnsmasq ] || port_busy; then
  cat > ` + resolvedPathQ + ` <<'LAIR_FALLBACK_EOF'
` + fallbackContent + `LAIR_FALLBACK_EOF
  chmod 0644 ` + resolvedPathQ + `
  echo "lair: dnsmasq no disponible o el puerto 53535 está ocupado; se deja el drop-in anterior DNS=` + fallbackDNS + ` con Domains=~` + domain + ` (Internet sigue funcionando)." >&2
  systemctl daemon-reload
  systemctl restart systemd-resolved
  exit 0
fi
cat > ` + resolvedPathQ + ` <<'LAIR_EOF'
` + resolvedContent + `LAIR_EOF
chmod 0644 ` + resolvedPathQ + `
systemctl daemon-reload
systemctl enable --now ` + DNSUnitName + `
systemctl restart systemd-resolved
OK=1
getent hosts lair.` + domain + ` 2>/dev/null | grep -q '127.0.0.1' || OK=0
if [ -n "$BEFORE_EXAMPLE" ]; then
  AFTER_EXAMPLE=$(getent ahostsv4 example.com 2>/dev/null | awk 'NR==1 {print $1}' || true)
  [ -n "$AFTER_EXAMPLE" ] || OK=0
fi
if [ "$OK" != 1 ]; then
  rollback
fi
echo "lair: DNS listo: lair.` + domain + ` -> 127.0.0.1; el resto de dominios siguen resolviendo."
`
}

// shQuote quotes a value for POSIX shell single-quote semantics.
func shQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", `'\''`) + "'"
}

func dirOf(path string) string {
	for i := len(path) - 1; i >= 0; i-- {
		if path[i] == '/' {
			return path[:i]
		}
	}
	return "."
}
